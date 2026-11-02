package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Limits. Security limits stay constants; the log tail is a setting.
const (
	// cmdTimeout bounds one short API call.
	cmdTimeout = 30 * time.Second
	// actionTimeout bounds start/stop/restart (stop waits for the container's grace period).
	actionTimeout = 2 * time.Minute
	// maxJSON caps a decoded response body.
	maxJSON = 32 << 20
	// maxErrBody caps an error message read from the engine.
	maxErrBody = 64 << 10
)

// API versions this client speaks: it uses the newest one the engine knows,
// within this range.
const (
	clientMaxVersion = "1.47"
	clientMinVersion = "1.24"
)

// ErrNotRunning is returned when the engine can't be reached.
var ErrNotRunning = errors.New("Docker isn't running — start Docker Desktop, Colima or OrbStack")

// Docker talks to the Docker Engine API.
type Docker struct {
	ep   Endpoint
	http *http.Client
	base string

	mu      sync.Mutex
	version string // negotiated, "1.45"
}

// New prepares a client; nothing is sent until the first call.
func New(o Options) (*Docker, error) {
	ep, err := Resolve(o)
	if err != nil {
		return nil, err
	}
	return newWithEndpoint(ep), nil
}

func newWithEndpoint(ep Endpoint) *Docker {
	d := &Docker{ep: ep}
	tr := &http.Transport{
		DialContext:         func(ctx context.Context, _, _ string) (net.Conn, error) { return ep.dial(ctx) },
		DisableCompression:  true,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     30 * time.Second,
	}
	d.http = &http.Client{
		Transport: tr,
		// A redirect would send our request (and TLS identity) elsewhere.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	d.base = "http://docker"
	return d
}

// Endpoint reports where the client connects.
func (d *Docker) Endpoint() Endpoint { return d.ep }

// Close drops idle connections.
func (d *Docker) Close() { d.http.CloseIdleConnections() }

type version struct{ major, minor int }

func parseVersion(s string) (version, bool) {
	a, b, ok := strings.Cut(strings.TrimSpace(s), ".")
	if !ok {
		return version{}, false
	}
	ma, e1 := strconv.Atoi(a)
	mi, e2 := strconv.Atoi(b)
	return version{ma, mi}, e1 == nil && e2 == nil
}

func (v version) less(o version) bool {
	return v.major < o.major || (v.major == o.major && v.minor < o.minor)
}

func (v version) String() string { return fmt.Sprintf("%d.%d", v.major, v.minor) }

// apiVersion negotiates once: the engine's newest version (from /_ping, or
// /version when the header is missing), capped at what this client knows.
func (d *Docker) apiVersion(ctx context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.version != "" {
		return d.version, nil
	}
	newest := ""
	resp, err := d.send(ctx, http.MethodGet, "/_ping", nil, nil)
	if err != nil {
		return "", err
	}
	newest = resp.Header.Get("Api-Version")
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<10))
	_ = resp.Body.Close()
	if newest == "" {
		var info struct {
			APIVersion string `json:"ApiVersion"`
		}
		if err := d.getJSON(ctx, "/version", nil, &info, false); err != nil {
			return "", err
		}
		newest = info.APIVersion
	}
	dv, ok := parseVersion(newest)
	if !ok {
		return "", fmt.Errorf("the Docker engine reported an unknown API version %q", newest)
	}
	maxV, _ := parseVersion(clientMaxVersion)
	minV, _ := parseVersion(clientMinVersion)
	if dv.less(minV) {
		return "", fmt.Errorf("Docker Engine API %s is too old (Flint needs %s or newer): update Docker", dv, minV)
	}
	if maxV.less(dv) {
		dv = maxV
	}
	d.version = dv.String()
	return d.version, nil
}

// send performs a request without version prefix handling or status checks.
func (d *Docker) send(ctx context.Context, method, path string, q url.Values, body io.Reader) (*http.Response, error) {
	u := d.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("User-Agent", "flint")
	resp, err := d.http.Do(req)
	if err != nil {
		return nil, d.connErr(err)
	}
	return resp, nil
}

// connErr turns a failed dial into the "Docker isn't running" hint.
func (d *Docker) connErr(err error) error {
	var op *net.OpError
	if errors.As(err, &op) && op.Op == "dial" {
		return fmt.Errorf("%w (%s)", ErrNotRunning, d.ep.Host)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("docker (%s): %w", d.ep.Host, err)
}

// request sends a versioned request; a non-2xx status becomes the engine's message.
func (d *Docker) request(ctx context.Context, method, path string, q url.Values, body any) (*http.Response, error) {
	v, err := d.apiVersion(ctx)
	if err != nil {
		return nil, err
	}
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		rd = bytes.NewReader(b)
	}
	resp, err := d.send(ctx, method, "/v"+v+path, q, rd)
	if err != nil {
		return nil, err
	}
	if isOK(resp.StatusCode) || resp.StatusCode == http.StatusNotModified {
		return resp, nil
	}
	defer resp.Body.Close()
	return nil, statusError(resp)
}

func isOK(code int) bool { return code >= 200 && code < 300 }

func statusError(resp *http.Response) error {
	b, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrBody))
	var e struct {
		Message string `json:"message"`
	}
	msg := strings.TrimSpace(string(b))
	if json.Unmarshal(b, &e) == nil && e.Message != "" {
		msg = e.Message
	}
	if msg == "" {
		msg = resp.Status
	}
	return fmt.Errorf("docker: %s", msg)
}

// getJSON decodes a GET; versioned selects the /vX prefix.
func (d *Docker) getJSON(ctx context.Context, path string, q url.Values, out any, versioned bool) error {
	var resp *http.Response
	var err error
	if versioned {
		resp, err = d.request(ctx, http.MethodGet, path, q, nil)
	} else {
		resp, err = d.send(ctx, http.MethodGet, path, q, nil)
		if err == nil && !isOK(resp.StatusCode) {
			defer resp.Body.Close()
			return statusError(resp)
		}
	}
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return decodeCapped(resp.Body, out)
}

// decodeCapped decodes JSON, refusing bodies over maxJSON.
func decodeCapped(r io.Reader, out any) error {
	lr := &io.LimitedReader{R: r, N: maxJSON + 1}
	if err := json.NewDecoder(lr).Decode(out); err != nil {
		if lr.N <= 0 {
			return fmt.Errorf("docker: response larger than %d MiB", maxJSON>>20)
		}
		return fmt.Errorf("docker: unexpected response: %w", err)
	}
	if lr.N <= 0 {
		return fmt.Errorf("docker: response larger than %d MiB", maxJSON>>20)
	}
	return nil
}

// Ping checks that the engine answers and returns the negotiated API version.
func (d *Docker) Ping(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	return d.apiVersion(ctx)
}
