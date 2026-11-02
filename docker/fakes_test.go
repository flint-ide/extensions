package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/flint-ide/extensions/docker/internal/engine"
)

// sent is a message the extension sent to Flint, with its params decoded.
type sent struct {
	Method string
	Params map[string]any
}

// fakeFlint records what the extension sends and answers its calls.
type fakeFlint struct {
	mu      sync.Mutex
	notes   []sent
	calls   []sent
	answers map[string]any   // method → result
	errs    map[string]error // method → error
}

func newFakeFlint() *fakeFlint {
	return &fakeFlint{answers: map[string]any{}, errs: map[string]error{}}
}

func decodeParams(v any) map[string]any {
	b, _ := json.Marshal(v)
	m := map[string]any{}
	_ = json.Unmarshal(b, &m)
	return m
}

func (f *fakeFlint) notify(method string, params any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.notes = append(f.notes, sent{method, decodeParams(params)})
}

func (f *fakeFlint) call(method string, params, result any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, sent{method, decodeParams(params)})
	if err := f.errs[method]; err != nil {
		return err
	}
	if a, ok := f.answers[method]; ok && result != nil {
		b, _ := json.Marshal(a)
		return json.Unmarshal(b, result)
	}
	return nil
}

func (f *fakeFlint) notified(method string) []sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []sent
	for _, n := range f.notes {
		if n.Method == method {
			out = append(out, n)
		}
	}
	return out
}

func (f *fakeFlint) called(method string) []sent {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []sent
	for _, c := range f.calls {
		if c.Method == method {
			out = append(out, c)
		}
	}
	return out
}

// fakeEngine is the Docker Engine as the extension sees it.
type fakeEngine struct {
	mu         sync.Mutex
	containers []engine.Container
	listErr    error
	actErr     error
	acts       []string // "start <id>"
	composes   []string // "up <project name or dir>"
	lists      int
	closed     int
}

func (f *fakeEngine) List(context.Context) ([]engine.Container, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists++
	if f.listErr != nil {
		return nil, f.listErr
	}
	return append([]engine.Container(nil), f.containers...), nil
}

func (f *fakeEngine) Act(_ context.Context, id, action string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.acts = append(f.acts, action+" "+id)
	return f.actErr
}

func (f *fakeEngine) Compose(_ context.Context, p engine.ComposeProject, action string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.composes = append(f.composes, action+" "+projectName(p))
	return f.actErr
}

func (f *fakeEngine) Close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
}

func (f *fakeEngine) snapshot() (acts, composes []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.acts...), append([]string(nil), f.composes...)
}

// harness is an extension wired to fakes.
type harness struct {
	e      *ext
	fl     *fakeFlint
	eng    *fakeEngine
	found  []engine.ComposeProject
	noCLI  bool
	dials  []string // host settings a client was made for
	dialMu sync.Mutex
	clock  time.Time
}

// Containers of the fixtures: a Compose project "shop" with a running and a
// stopped service, a running standalone container and a stopped one.
func fixture() []engine.Container {
	shop := func(id, name, service, state, status string) engine.Container {
		return engine.Container{
			ID: id, Name: name, Image: "img/" + service, State: state, Status: status,
			Project: "shop", Service: service, WorkingDir: "/p/shop", ConfigFiles: "/p/shop/compose.yaml,/p/shop/compose.override.yaml",
		}
	}
	return []engine.Container{
		shop("web1", "shop-web-1", "web", "exited", "Exited (0) 5 minutes ago"),
		shop("db1", "shop-db-1", "db", "running", "Up 2 hours (healthy)"),
		{ID: "lonely1", Name: "lonely", Image: "alpine", State: "exited", Status: "Exited (1) 1 hour ago"},
		{ID: "api1", Name: "api", Image: "node:22", State: "running", Status: "Up 3 minutes",
			Ports: []engine.Port{{Host: "0.0.0.0", HostPort: 8080, Container: "80/tcp"}}},
	}
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{fl: newFakeFlint(), eng: &fakeEngine{containers: fixture()}, clock: time.Unix(1_800_000_000, 0)}
	home := t.TempDir()
	opts := func(host string) engine.Options {
		return engine.Options{Host: host, Home: home, Getenv: func(string) string { return "" }}
	}
	d := deps{
		resolve: func(host string) (engine.Endpoint, error) { return engine.Resolve(opts(host)) },
		dial: func(host string) (engineClient, error) {
			h.dialMu.Lock()
			defer h.dialMu.Unlock()
			h.dials = append(h.dials, host)
			return h.eng, nil
		},
		composeAvailable: func() bool { return !h.noCLI },
		findCompose:      func([]string) []engine.ComposeProject { return h.found },
		now: func() time.Time {
			h.dialMu.Lock()
			defer h.dialMu.Unlock()
			h.clock = h.clock.Add(2 * snapTTL) // every listing is fresh unless a test says otherwise
			return h.clock
		},
	}
	h.e = newExt(h.fl, d)
	t.Cleanup(h.e.stop)
	return h
}

// request sends a request the way Flint does, through the JSON params.
func (h *harness) request(t *testing.T, method string, params any) (any, error) {
	t.Helper()
	b, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	return h.e.handle(method, b)
}

func (h *harness) notify(t *testing.T, method string, params any) {
	t.Helper()
	b, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	h.e.notified(method, b)
}

// resolve returns the panel's view model for parent, decoded as Flint would.
func (h *harness) resolve(t *testing.T, parent string) viewModel {
	t.Helper()
	v, err := h.request(t, "view/resolve", map[string]string{"view": panelID, "parent": parent})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	var m viewModel
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// act sends view/action and waits for the background work.
func (h *harness) act(t *testing.T, item, action string) error {
	t.Helper()
	_, err := h.request(t, "view/action", map[string]string{"view": panelID, "item": item, "action": action})
	h.e.wg.Wait()
	return err
}

type viewModel struct {
	Kind    string   `json:"kind"`
	Text    string   `json:"text"`
	Actions []action `json:"actions"`
	Items   []struct {
		ID          string   `json:"id"`
		Label       string   `json:"label"`
		Description string   `json:"description"`
		Tooltip     string   `json:"tooltip"`
		Icon        string   `json:"icon"`
		HasChildren bool     `json:"hasChildren"`
		Actions     []action `json:"actions"`
	} `json:"items"`
}

func (v viewModel) ids() []string {
	var out []string
	for _, it := range v.Items {
		out = append(out, it.ID)
	}
	return out
}

func actionIDs(list []action) []string {
	var out []string
	for _, a := range list {
		out = append(out, a.ID)
	}
	return out
}

func notRunning() error { return fmt.Errorf("%w (unix:///var/run/docker.sock)", engine.ErrNotRunning) }

var errBoom = errors.New("boom")
