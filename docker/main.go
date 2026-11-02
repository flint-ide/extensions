// Command docker-ext is Flint's Docker extension: a panel with the
// containers of the Docker Engine grouped by Compose project, their actions
// (start, stop, restart, remove, compose up / down / pull) and terminal tabs
// for their logs and a shell. It talks to the engine through its API
// (internal/engine, standard library only) and to Flint over stdin and stdout.
//
// Build it next to its manifest and install the folder from Flint
// (Install Extension from Folder…):
//
//	go build -o docker-ext .
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"github.com/flint-ide/extensions/docker/internal/engine"
)

// Ids from flint-extension.json.
const (
	extID      = "flint.docker"
	panelID    = "flint.docker.containers"
	statusID   = "flint.docker.running"
	cmdRefresh = "flint.docker.refresh"
)

// pollInterval is how often the panel is refreshed while it is on screen.
const pollInterval = 3 * time.Second

// flint is the other side of the connection: *conn, or a fake in tests.
type flint interface {
	notify(method string, params any)
	call(method string, params, result any) error
}

// engineClient is what the extension needs of the Docker Engine
// (*engine.Docker, or a fake in tests).
type engineClient interface {
	List(ctx context.Context) ([]engine.Container, error)
	Act(ctx context.Context, id, action string) error
	Compose(ctx context.Context, project engine.ComposeProject, action string) error
	Close()
}

// deps are the extension's ways into the system, replaced in tests.
type deps struct {
	// resolve finds the engine for the host setting.
	resolve func(host string) (engine.Endpoint, error)
	// dial prepares a client for the host setting.
	dial func(host string) (engineClient, error)
	// composeAvailable reports whether the docker CLI is installed.
	composeAvailable func() bool
	// findCompose lists compose files in the project folders.
	findCompose func(roots []string) []engine.ComposeProject
	now         func() time.Time
}

func systemDeps() deps {
	return deps{
		resolve: func(host string) (engine.Endpoint, error) { return engine.Resolve(engine.Options{Host: host}) },
		dial: func(host string) (engineClient, error) {
			d, err := engine.New(engine.Options{Host: host})
			if err != nil {
				return nil, err
			}
			return d, nil
		},
		composeAvailable: engine.ComposeAvailable,
		findCompose:      engine.FindComposeFiles,
		now:              time.Now,
	}
}

type ext struct {
	fl       flint
	deps     deps
	interval time.Duration

	ctx    context.Context // ends at shutdown
	cancel context.CancelFunc
	wg     sync.WaitGroup // actions running in the background

	mu        sync.Mutex
	settings  settings
	roots     []string
	client    engineClient
	clientKey string
	snap      *snapshot         // the last listing; nil after a change
	busy      map[string]string // item id → what runs on it ("Stopping…")
	status    string            // the status item's last text
	stopPoll  chan struct{}     // closed to stop polling; nil while the panel is hidden
}

func newExt(fl flint, d deps) *ext {
	ctx, cancel := context.WithCancel(context.Background())
	return &ext{fl: fl, deps: d, interval: pollInterval, ctx: ctx, cancel: cancel, settings: defaultSettings(), busy: map[string]string{}}
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("docker: ") // stderr is Flint's log of the extension
	c := newConn(os.Stdout)
	e := newExt(c, systemDeps())
	if err := c.serve(os.Stdin, e.handle, e.notified); err != nil {
		log.Fatal(err)
	}
	e.stop()
}

// handle answers Flint's requests.
func (e *ext) handle(method string, params json.RawMessage) (any, error) {
	switch method {
	case "initialize":
		var p struct {
			Roots    []root   `json:"roots"`
			Settings settings `json:"settings"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{Code: -32602, Message: err.Error()}
		}
		e.mu.Lock()
		e.settings = p.Settings.normalize()
		e.roots = rootPaths(p.Roots)
		e.mu.Unlock()
		log.Printf("started for %d folder(s)", len(p.Roots))
		return map[string]any{"contract": 1, "capabilities": map[string]any{}}, nil
	case "shutdown":
		e.stop()
		return nil, nil
	case "view/resolve":
		var p struct{ View, Parent string }
		_ = json.Unmarshal(params, &p)
		if p.View != panelID {
			return nil, &rpcError{Code: -32602, Message: "unknown view " + p.View}
		}
		return e.view(p.Parent), nil
	case "view/action":
		var p struct{ View, Item, Action string }
		_ = json.Unmarshal(params, &p)
		if p.View != panelID {
			return nil, &rpcError{Code: -32602, Message: "unknown view " + p.View}
		}
		return nil, e.action(p.Item, p.Action)
	case "command/execute":
		var p struct{ ID string }
		_ = json.Unmarshal(params, &p)
		if p.ID == cmdRefresh {
			return nil, e.refreshNow()
		}
		return nil, &rpcError{Code: -32602, Message: "unknown command " + p.ID}
	}
	return nil, &rpcError{Code: -32601, Message: "method not found: " + method}
}

// notified handles Flint's notifications.
func (e *ext) notified(method string, params json.RawMessage) {
	switch method {
	case "settings/changed":
		var p struct{ Settings settings }
		if json.Unmarshal(params, &p) == nil {
			e.mu.Lock()
			e.settings, e.snap = p.Settings.normalize(), nil
			e.mu.Unlock()
			e.refreshView()
		}
	case "workspace/rootsChanged":
		var p struct{ Roots []root }
		if json.Unmarshal(params, &p) == nil {
			e.mu.Lock()
			e.roots, e.snap = rootPaths(p.Roots), nil
			e.mu.Unlock()
			e.refreshView()
		}
	case "view/visible":
		var p struct {
			View    string
			Visible bool
		}
		if json.Unmarshal(params, &p) == nil && p.View == panelID {
			e.setPolling(p.Visible)
		}
	}
}

type root struct {
	Path    string `json:"path"`
	Trusted bool   `json:"trusted"`
}

func rootPaths(rs []root) []string {
	out := make([]string, 0, len(rs))
	for _, r := range rs {
		out = append(out, r.Path)
	}
	return out
}

// stop ends polling and background work (shutdown, or stdin closed).
func (e *ext) stop() {
	e.setPolling(false)
	e.cancel()
	e.mu.Lock()
	if e.client != nil {
		e.client.Close()
		e.client, e.clientKey = nil, ""
	}
	e.mu.Unlock()
}

// setPolling starts or stops refreshing the panel on a timer: only while it
// is on screen, as the protocol asks.
func (e *ext) setPolling(on bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !on {
		if e.stopPoll != nil {
			close(e.stopPoll)
			e.stopPoll = nil
		}
		return
	}
	if e.stopPoll != nil || e.ctx.Err() != nil {
		return
	}
	stop := make(chan struct{})
	e.stopPoll = stop
	go e.poll(stop, e.interval)
}

func (e *ext) poll(stop <-chan struct{}, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-stop:
			return
		case <-e.ctx.Done():
			return
		case <-t.C:
			e.invalidate()
			e.refreshView()
		}
	}
}

// refreshView asks Flint to resolve the panel again.
func (e *ext) refreshView() {
	e.fl.notify("view/refresh", map[string]string{"view": panelID})
}

// invalidate drops the last listing, so the next view lists again.
func (e *ext) invalidate() {
	e.mu.Lock()
	e.snap = nil
	e.mu.Unlock()
}

// refreshNow is the Refresh command: list now (an unreachable engine is
// the command's error, shown as a toast) and redraw the panel.
func (e *ext) refreshNow() error {
	e.invalidate()
	s := e.overview()
	e.refreshView()
	return s.err
}

// docker returns the client for the configured engine, made again when the
// host setting or the docker context changed.
func (e *ext) docker() (engineClient, engine.Endpoint, error) {
	e.mu.Lock()
	host := e.settings.Host
	e.mu.Unlock()
	if err := (settings{Host: host}).check(); err != nil {
		return nil, engine.Endpoint{}, err
	}
	ep, err := e.deps.resolve(host)
	if err != nil {
		return nil, engine.Endpoint{}, err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.client != nil && e.clientKey == ep.Key() {
		return e.client, ep, nil
	}
	c, err := e.deps.dial(host)
	if err != nil {
		return nil, engine.Endpoint{}, err
	}
	if e.client != nil {
		e.client.Close()
	}
	e.client, e.clientKey = c, ep.Key()
	return c, ep, nil
}
