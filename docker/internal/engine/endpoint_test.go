package engine

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestEndpointPrecedence(t *testing.T) {
	home := t.TempDir()
	writeContext(t, home, "desktop-linux", `{"Name":"desktop-linux","Endpoints":{"docker":{"Host":"unix:///ctx/docker.sock","SkipTLSVerify":false}}}`)
	writeFile(t, filepath.Join(home, ".docker", "config.json"), `{"currentContext":"desktop-linux"}`)

	ep, err := Resolve(Options{Home: home, Getenv: envOf(nil)})
	if err != nil || ep.Addr != "/ctx/docker.sock" || ep.Source != "context desktop-linux" {
		t.Fatalf("context: %+v %v", ep, err)
	}
	ep, _ = Resolve(Options{Home: home, Getenv: envOf(map[string]string{"DOCKER_CONTEXT": "default"})})
	if ep.Source != "default" {
		t.Fatalf("DOCKER_CONTEXT=default must skip the context: %+v", ep)
	}
	ep, _ = Resolve(Options{Home: home, Getenv: envOf(map[string]string{"DOCKER_HOST": "unix:///env.sock"})})
	if ep.Addr != "/env.sock" || ep.Source != "DOCKER_HOST" {
		t.Fatalf("env beats context: %+v", ep)
	}
	ep, _ = Resolve(Options{Home: home, Host: "unix:///setting.sock", Getenv: envOf(map[string]string{"DOCKER_HOST": "unix:///env.sock"})})
	if ep.Addr != "/setting.sock" || ep.Source != "setting" {
		t.Fatalf("setting beats env: %+v", ep)
	}
	ep, _ = Resolve(Options{Home: home, Host: "/bare/path.sock", Getenv: envOf(nil)})
	if ep.Addr != "/bare/path.sock" {
		t.Fatalf("bare path: %+v", ep)
	}
}

func TestDefaultSocketsAreProbed(t *testing.T) {
	home, err := os.MkdirTemp("", "h")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(home)
	sock := filepath.Join(home, ".colima", "default", "docker.sock")
	_ = os.MkdirAll(filepath.Dir(sock), 0o755)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ep, _ := Resolve(Options{Home: home, Getenv: envOf(nil)})
	// /var/run/docker.sock may exist on the machine running the tests; Colima's must win otherwise.
	if _, statErr := os.Stat("/var/run/docker.sock"); statErr != nil && ep.Addr != sock {
		t.Fatalf("expected Colima socket, got %+v", ep)
	}
}

func TestUnknownContextFallsBack(t *testing.T) {
	home := t.TempDir()
	ep, err := Resolve(Options{Home: home, Getenv: envOf(map[string]string{"DOCKER_CONTEXT": "gone"})})
	if err != nil || ep.Source != "default" {
		t.Fatalf("%+v %v", ep, err)
	}
}

func TestPlainTCPWarnsAndSSHIsRefused(t *testing.T) {
	ep, err := Resolve(Options{Host: "tcp://10.0.0.5:2375", Getenv: envOf(nil)})
	if err != nil || ep.Network != "tcp" || ep.Addr != "10.0.0.5:2375" || ep.TLS != nil || !strings.Contains(ep.Warning, "plain tcp://") {
		t.Fatalf("%+v %v", ep, err)
	}
	ep, _ = Resolve(Options{Getenv: envOf(map[string]string{"DOCKER_HOST": "tcp://box"})})
	if ep.Addr != "box:2375" || ep.Warning == "" {
		t.Fatalf("default port: %+v", ep)
	}
	if ep, _ := Resolve(Options{Host: "unix:///x.sock", Getenv: envOf(nil)}); ep.Warning != "" {
		t.Errorf("a unix socket is not a warning: %+v", ep)
	}
	for _, bad := range []string{"ssh://me@host", "npipe:////./pipe/docker_engine", "ftp://x", "tcp://", "nonsense"} {
		if _, err := Resolve(Options{Host: bad, Getenv: envOf(nil)}); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestTLSFromEnvironment(t *testing.T) {
	certs := t.TempDir()
	writeCerts(t, certs)
	env := map[string]string{"DOCKER_HOST": "tcp://engine.example:2376", "DOCKER_TLS_VERIFY": "1", "DOCKER_CERT_PATH": certs}
	ep, err := Resolve(Options{Getenv: envOf(env)})
	if err != nil || ep.TLS == nil || ep.Warning != "" || ep.TLS.ServerName != "engine.example" || ep.TLS.InsecureSkipVerify || len(ep.TLS.Certificates) != 1 || ep.TLS.RootCAs == nil {
		t.Fatalf("%+v %v", ep, err)
	}
	// Missing certificates must not silently degrade to an unverified connection.
	env["DOCKER_CERT_PATH"] = t.TempDir()
	if _, err := Resolve(Options{Getenv: envOf(env)}); err == nil || !strings.Contains(err.Error(), "ca.pem") {
		t.Fatalf("err = %v", err)
	}
}

func TestContextWithTLSMaterial(t *testing.T) {
	home := t.TempDir()
	writeContext(t, home, "remote", `{"Name":"remote","Endpoints":{"docker":{"Host":"tcp://remote.example:2376","SkipTLSVerify":false}}}`)
	writeCerts(t, filepath.Join(home, ".docker", "contexts", "tls", contextDigest("remote"), "docker"))
	ep, err := Resolve(Options{Home: home, Getenv: envOf(map[string]string{"DOCKER_CONTEXT": "remote"})})
	if err != nil || ep.TLS == nil || ep.Warning != "" || ep.Source != "context remote" {
		t.Fatalf("%+v %v", ep, err)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	f := newFake(t)
	d := f.client()
	if d.http.CheckRedirect == nil {
		t.Fatal("redirect policy missing")
	}
	if err := d.http.CheckRedirect(nil, nil); err == nil {
		t.Fatal("redirects must not be followed")
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func writeContext(t *testing.T, home, name, meta string) {
	t.Helper()
	writeFile(t, filepath.Join(home, ".docker", "contexts", "meta", contextDigest(name), "meta.json"), meta)
}

// writeCerts writes a self-signed ca.pem, cert.pem and key.pem.
func writeCerts(t *testing.T, dir string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "flint-test"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(key)
	crt := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	writeFile(t, filepath.Join(dir, "ca.pem"), crt)
	writeFile(t, filepath.Join(dir, "cert.pem"), crt)
	writeFile(t, filepath.Join(dir, "key.pem"), string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})))
}

func TestCLIEnvOnlyForTheSetting(t *testing.T) {
	for host, want := range map[string]string{
		"":                   "",
		"unix:///tmp/d.sock": "DOCKER_HOST=unix:///tmp/d.sock",
		"tcp://h:2375":       "DOCKER_HOST=tcp://h:2375",
		"https://h:2376":     "DOCKER_HOST=tcp://h:2376 DOCKER_TLS_VERIFY=1",
	} {
		ep, err := Resolve(Options{Host: host, Getenv: envOf(nil), Home: t.TempDir()})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, k := range []string{"DOCKER_HOST", "DOCKER_TLS", "DOCKER_TLS_VERIFY"} {
			if v, ok := ep.CLIEnv()[k]; ok {
				got = append(got, k+"="+v)
			}
		}
		if strings.Join(got, " ") != want {
			t.Errorf("%q: env %q, want %q", host, got, want)
		}
	}
}
