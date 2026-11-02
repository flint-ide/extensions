package engine

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Endpoint is where the Docker Engine listens and how to reach it.
type Endpoint struct {
	Network string // "unix" or "tcp"
	Addr    string // socket path or host:port
	// TLS is set for tcp endpoints that use TLS.
	TLS *tls.Config
	// Host is the endpoint as the docker CLI writes it (unix:///…, tcp://…).
	Host string
	// Source says where it came from: setting, DOCKER_HOST, context <name> or default.
	Source string
	// Warning is set when the connection is not protected (plain tcp://).
	Warning string
}

// Options say how to find the engine. The zero value reads the process
// environment and the user's home like the docker CLI does.
type Options struct {
	// Host overrides everything else (Settings ▸ Docker): unix://, tcp://, https://.
	Host string
	// Getenv and Home are replaced in tests.
	Getenv func(string) string
	Home   string
}

func (o Options) env(k string) string {
	if o.Getenv != nil {
		return o.Getenv(k)
	}
	return os.Getenv(k)
}

func (o Options) home() string {
	if o.Home != "" {
		return o.Home
	}
	h, _ := os.UserHomeDir()
	return h
}

// configDir is the docker CLI's config directory.
func (o Options) configDir() string {
	if d := o.env("DOCKER_CONFIG"); d != "" {
		return d
	}
	return filepath.Join(o.home(), ".docker")
}

// socketCandidates are the sockets of Docker Desktop, Colima, OrbStack,
// Rancher Desktop and rootless Docker, tried after /var/run/docker.sock.
func (o Options) socketCandidates() []string {
	h := o.home()
	list := []string{
		"/var/run/docker.sock",
		filepath.Join(h, ".docker", "run", "docker.sock"),
		filepath.Join(h, ".colima", "default", "docker.sock"),
		filepath.Join(h, ".orbstack", "run", "docker.sock"),
		filepath.Join(h, ".rd", "docker.sock"),
	}
	if x := o.env("XDG_RUNTIME_DIR"); x != "" {
		list = append(list, filepath.Join(x, "docker.sock"))
	}
	return list
}

// Resolve finds the engine: the Host option, DOCKER_HOST, the current docker
// context, then the platform's default sockets.
func Resolve(o Options) (Endpoint, error) {
	host, source := strings.TrimSpace(o.Host), "setting"
	if host == "" {
		host, source = o.env("DOCKER_HOST"), "DOCKER_HOST"
	}
	if host != "" {
		cfg, err := o.tlsFromEnv()
		if err != nil {
			return Endpoint{}, err
		}
		return parseHost(host, cfg, source)
	}
	if name := o.contextName(); name != "" && name != "default" {
		if ep, ok, err := o.contextEndpoint(name); err != nil {
			return Endpoint{}, err
		} else if ok {
			return ep, nil
		}
	}
	for _, p := range o.socketCandidates() {
		if fi, err := os.Stat(p); err == nil && fi.Mode()&os.ModeSocket != 0 {
			return Endpoint{Network: "unix", Addr: p, Host: "unix://" + p, Source: "default"}, nil
		}
	}
	p := "/var/run/docker.sock"
	return Endpoint{Network: "unix", Addr: p, Host: "unix://" + p, Source: "default"}, nil
}

func (o Options) contextName() string {
	if c := o.env("DOCKER_CONTEXT"); c != "" {
		return c
	}
	b, err := os.ReadFile(filepath.Join(o.configDir(), "config.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		CurrentContext string `json:"currentContext"`
	}
	_ = json.Unmarshal(b, &cfg)
	return cfg.CurrentContext
}

func contextDigest(name string) string {
	sum := sha256.Sum256([]byte(name))
	return hex.EncodeToString(sum[:])
}

// contextEndpoint reads ~/.docker/contexts/meta/<sha256(name)>/meta.json.
func (o Options) contextEndpoint(name string) (Endpoint, bool, error) {
	dir := filepath.Join(o.configDir(), "contexts")
	b, err := os.ReadFile(filepath.Join(dir, "meta", contextDigest(name), "meta.json"))
	if err != nil {
		return Endpoint{}, false, nil // unknown context: fall back to the defaults
	}
	var meta struct {
		Endpoints map[string]struct {
			Host          string `json:"Host"`
			SkipTLSVerify bool   `json:"SkipTLSVerify"`
		} `json:"Endpoints"`
	}
	if err := json.Unmarshal(b, &meta); err != nil {
		return Endpoint{}, false, fmt.Errorf("docker context %q: %w", name, err)
	}
	e, ok := meta.Endpoints["docker"]
	if !ok || e.Host == "" {
		return Endpoint{}, false, nil
	}
	certs := filepath.Join(dir, "tls", contextDigest(name), "docker")
	var cfg *tls.Config
	if fi, err := os.Stat(filepath.Join(certs, "ca.pem")); err == nil && !fi.IsDir() || e.SkipTLSVerify {
		cfg, err = loadTLS(certs, !e.SkipTLSVerify)
		if err != nil {
			return Endpoint{}, false, fmt.Errorf("docker context %q: %w", name, err)
		}
	}
	ep, err := parseHost(e.Host, cfg, "context "+name)
	return ep, err == nil, err
}

// tlsFromEnv builds the TLS settings DOCKER_TLS_VERIFY / DOCKER_CERT_PATH ask
// for; nil when TLS isn't requested. Missing certificates are an error, never a silent downgrade.
func (o Options) tlsFromEnv() (*tls.Config, error) {
	if o.env("DOCKER_TLS_VERIFY") == "" {
		return nil, nil
	}
	dir := o.env("DOCKER_CERT_PATH")
	if dir == "" {
		dir = o.configDir()
	}
	return loadTLS(dir, true)
}

// loadTLS reads ca.pem, cert.pem and key.pem from dir.
func loadTLS(dir string, verify bool) (*tls.Config, error) {
	cfg := &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: !verify} //nolint:gosec // the context asked to skip verification
	if ca, err := os.ReadFile(filepath.Join(dir, "ca.pem")); err == nil {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(ca) {
			return nil, fmt.Errorf("%s/ca.pem holds no certificate", dir)
		}
		cfg.RootCAs = pool
	} else if verify {
		return nil, fmt.Errorf("DOCKER_TLS_VERIFY is set but %s/ca.pem is missing", dir)
	}
	if _, err := os.Stat(filepath.Join(dir, "cert.pem")); err == nil {
		cert, err := tls.LoadX509KeyPair(filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem"))
		if err != nil {
			return nil, err
		}
		cfg.Certificates = []tls.Certificate{cert}
	}
	return cfg, nil
}

// parseHost reads a DOCKER_HOST style address. tlsCfg applies to tcp/https.
func parseHost(host string, tlsCfg *tls.Config, source string) (Endpoint, error) {
	if strings.HasPrefix(host, "/") {
		host = "unix://" + host
	}
	u, err := url.Parse(host)
	if err != nil || u.Scheme == "" {
		return Endpoint{}, fmt.Errorf("invalid Docker host %q (use unix:///path or tcp://host:port)", host)
	}
	switch u.Scheme {
	case "unix":
		p := u.Path
		if p == "" {
			p = u.Host
		}
		if p == "" {
			return Endpoint{}, fmt.Errorf("invalid Docker host %q: no socket path", host)
		}
		return Endpoint{Network: "unix", Addr: p, Host: "unix://" + p, Source: source}, nil
	case "tcp", "http", "https":
		addr := u.Host
		if addr == "" {
			return Endpoint{}, fmt.Errorf("invalid Docker host %q: no address", host)
		}
		if _, _, err := net.SplitHostPort(addr); err != nil {
			port := "2375"
			if tlsCfg != nil || u.Scheme == "https" {
				port = "2376"
			}
			addr = net.JoinHostPort(strings.Trim(addr, "[]"), port)
		}
		ep := Endpoint{Network: "tcp", Addr: addr, Source: source}
		if u.Scheme == "https" && tlsCfg == nil {
			tlsCfg = &tls.Config{MinVersion: tls.VersionTLS12}
		}
		if tlsCfg != nil {
			cfg := tlsCfg.Clone()
			h, _, _ := net.SplitHostPort(addr)
			cfg.ServerName = h
			ep.TLS = cfg
			ep.Host = "tcp://" + addr
		} else {
			ep.Host = "tcp://" + addr
			ep.Warning = fmt.Sprintf("Docker at %s is reached over plain tcp:// without TLS: anyone on the network can control your containers. Use TLS (DOCKER_TLS_VERIFY) or a unix socket.", addr)
		}
		return ep, nil
	case "ssh":
		return Endpoint{}, errors.New("ssh:// Docker hosts need the docker CLI; use a unix socket or tcp:// with TLS")
	default:
		return Endpoint{}, fmt.Errorf("unsupported Docker host scheme %q", u.Scheme)
	}
}

// CLIEnv is the environment the docker CLI (compose, a terminal running
// docker logs or docker exec) needs to reach the same engine: DOCKER_HOST
// when the endpoint came from the setting, with DOCKER_TLS_VERIFY (or
// DOCKER_TLS without verification) when it uses TLS. Endpoints found from a
// docker context or the default sockets need nothing: the CLI finds them the
// same way.
func (e Endpoint) CLIEnv() map[string]string {
	if e.Source != "setting" {
		return nil
	}
	env := map[string]string{"DOCKER_HOST": e.Host}
	if e.TLS != nil {
		if e.TLS.InsecureSkipVerify {
			env["DOCKER_TLS"] = "1"
		} else {
			env["DOCKER_TLS_VERIFY"] = "1"
		}
	}
	return env
}

// Key identifies the endpoint, so a cached client is dropped when it changes.
func (e Endpoint) Key() string { return e.Network + "|" + e.Addr + "|" + e.Source }

// dial opens a connection to the engine (TLS handshake included).
func (e Endpoint) dial(ctx context.Context) (net.Conn, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, e.Network, e.Addr)
	if err != nil {
		return nil, err
	}
	if e.TLS == nil {
		return conn, nil
	}
	tc := tls.Client(conn, e.TLS)
	if err := tc.HandshakeContext(ctx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return tc, nil
}
