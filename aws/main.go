// Command aws-ext is Flint's "AWS Profiles" extension: the AWS CLI profiles
// of ~/.aws (config and credentials) with the state of their credentials, in
// a panel. From a profile you can log in, refresh its credentials without a
// terminal, ask STS who you are, and open a terminal working as the profile.
// It uses only the standard library and talks to Flint over stdin and stdout
// (site/extensions/protocol.md in the Flint repository).
//
// Build it next to its manifest and install the folder from Flint
// (Install Extension from Folder…):
//
//	go build -o aws-ext .
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/flint-ide/extensions/aws/internal/awsprof"
)

const (
	panelID   = "flint.aws.profiles"
	refreshID = "flint.aws.refresh"
)

// Bounds of refreshSeconds, the same as Flint's refresh.awsSeconds.
const (
	defaultRefreshSeconds = 30
	minRefreshSeconds     = 5
	maxRefreshSeconds     = 3600
)

// doneNoteFor is how long a successful refresh stays noted under its profile.
const doneNoteFor = 8 * time.Second

type settings struct {
	RefreshSeconds int `json:"refreshSeconds"`
}

// interval is how often the panel is re-read while it is shown.
func (s settings) interval() time.Duration {
	n := s.RefreshSeconds
	switch {
	case n == 0:
		n = defaultRefreshSeconds
	case n < minRefreshSeconds:
		n = minRefreshSeconds
	case n > maxRefreshSeconds:
		n = maxRefreshSeconds
	}
	return time.Duration(n) * time.Second
}

// refresher renews credentials without a terminal (awsprof.Refresher).
type refresher interface {
	Refresh(ctx context.Context, p awsprof.Profile) (string, error)
	Cancel(profile string) bool
}

// deps is what the extension reads and runs; tests replace them.
type deps struct {
	load      func() ([]awsprof.Profile, error)
	identity  func(ctx context.Context, profile string) (awsprof.Identity, error)
	refresher refresher
	hasCLI    func() bool
	now       func() time.Time
	// scale shortens every timer in tests (1 in the program).
	scale time.Duration
}

func realDeps() deps {
	return deps{
		load: func() ([]awsprof.Profile, error) {
			files, err := awsprof.DefaultFiles()
			if err != nil {
				return nil, err
			}
			return awsprof.Load(files, time.Now())
		},
		identity:  awsprof.CallerIdentity,
		refresher: awsprof.NewRefresher(),
		hasCLI: func() bool {
			_, err := exec.LookPath("aws")
			return err == nil
		},
		now:   time.Now,
		scale: 1,
	}
}

type ext struct {
	c *conn
	d deps
	// ctx ends on shutdown and stops running refreshes and identity checks.
	ctx    context.Context
	cancel context.CancelFunc

	mu         sync.Mutex
	settings   settings
	profiles   []awsprof.Profile // as last read
	loadErr    error
	identities map[string]identityState
	refreshes  map[string]refreshState
	refreshSeq int
	poll       *poller // non-nil while the panel is shown
}

// identityState is the last Check Identity of a profile.
type identityState struct {
	checking bool
	id       awsprof.Identity
	err      string
}

// refreshState is a credential refresh: running, or how the last one ended.
type refreshState struct {
	running    bool
	ok         bool
	message    string // the tool's last line, or the error
	needsInput bool
	seq        int // tells refreshes apart (ext.refreshSeq)
}

func newExt(c *conn, d deps) *ext {
	ctx, cancel := context.WithCancel(context.Background())
	return &ext{
		c: c, d: d, ctx: ctx, cancel: cancel,
		settings:   settings{RefreshSeconds: defaultRefreshSeconds},
		identities: map[string]identityState{},
		refreshes:  map[string]refreshState{},
	}
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("aws: ") // stderr is Flint's log of the extension
	e := newExt(newConn(os.Stdout), realDeps())
	err := e.c.serve(os.Stdin, e.handle, e.notified)
	e.stop()
	if err != nil {
		log.Fatal(err)
	}
}

// handle answers Flint's requests.
func (e *ext) handle(method string, params json.RawMessage) (any, error) {
	switch method {
	case "initialize":
		var p struct {
			Settings settings `json:"settings"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid initialize params"}
		}
		e.mu.Lock()
		e.settings = p.Settings
		e.mu.Unlock()
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
		if p.Parent != "" {
			return e.children(p.Parent), nil
		}
		e.reload()
		return e.root(), nil
	case "view/action":
		var p struct{ View, Item, Action string }
		_ = json.Unmarshal(params, &p)
		return nil, e.action(p.Item, p.Action)
	case "command/execute":
		var p struct{ ID string }
		_ = json.Unmarshal(params, &p)
		if p.ID != refreshID {
			return nil, &rpcError{Code: -32602, Message: "unknown command " + p.ID}
		}
		e.reload()
		e.refreshView()
		return nil, nil
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
			e.settings = p.Settings
			e.mu.Unlock()
			e.restartPolling()
		}
	case "view/visible":
		var p struct {
			View    string
			Visible bool
		}
		if json.Unmarshal(params, &p) == nil && p.View == panelID {
			e.setVisible(p.Visible)
		}
	}
}

// stop ends polling and whatever still runs.
func (e *ext) stop() {
	e.setVisible(false)
	e.cancel()
}

// reload reads ~/.aws again (two small files and the SSO token cache).
func (e *ext) reload() {
	profiles, err := e.d.load()
	if err != nil {
		log.Printf("reading the profiles: %v", err)
	}
	e.mu.Lock()
	e.profiles, e.loadErr = profiles, err
	e.mu.Unlock()
}

// profile is the profile named name as last read.
func (e *ext) profile(name string) (awsprof.Profile, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, p := range e.profiles {
		if p.Name == name {
			return p, true
		}
	}
	return awsprof.Profile{}, false
}

func (e *ext) refreshView() {
	e.c.notify("view/refresh", map[string]string{"view": panelID})
}

func (e *ext) toast(kind, message string) {
	e.c.notify("window/showMessage", map[string]string{"type": kind, "message": cut(message, 1000)})
}

// cut shortens s to at most n runes.
func cut(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
