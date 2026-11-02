package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/flint-ide/extensions/kubernetes/internal/awsprof"
	"github.com/flint-ide/extensions/kubernetes/internal/kube"
)

// fakeCluster stands in for kubectl and records what was asked.
type fakeCluster struct {
	mu       sync.Mutex
	contexts []kube.Context
	ov       kube.Overview
	err      error
	diag     string
	asked    []string
}

func (f *fakeCluster) note(s string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.asked = append(f.asked, s)
}

func (f *fakeCluster) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.asked...)
}

func (f *fakeCluster) Contexts(context.Context) ([]kube.Context, error) {
	f.note("contexts")
	return f.contexts, f.err
}
func (f *fakeCluster) Namespaces(_ context.Context, c string) ([]string, error) {
	f.note("namespaces " + c)
	return []string{"default", "shop"}, f.err
}
func (f *fakeCluster) Overview(_ context.Context, c, ns string) (kube.Overview, error) {
	f.note("overview " + c + "/" + ns)
	return f.ov, f.err
}
func (f *fakeCluster) RolloutRestart(_ context.Context, c, ns, kind, name string) error {
	f.note("restart " + c + "/" + ns + " " + kind + "/" + name)
	return f.err
}
func (f *fakeCluster) RolloutUndo(_ context.Context, c, ns, kind, name string) error {
	f.note("undo " + c + "/" + ns + " " + kind + "/" + name)
	return f.err
}
func (f *fakeCluster) Scale(_ context.Context, c, ns, kind, name string, replicas int) error {
	f.note("scale " + c + "/" + ns + " " + kind + "/" + name + " " + strings.Repeat("|", replicas))
	return f.err
}
func (f *fakeCluster) DeletePod(_ context.Context, c, ns, name string) error {
	f.note("delete " + c + "/" + ns + " " + name)
	return f.err
}
func (f *fakeCluster) UseContext(_ context.Context, name string) error {
	f.note("use " + name)
	return f.err
}
func (f *fakeCluster) Diagnose(_ context.Context, c, ns, app string) (string, error) {
	f.note("diagnose " + c + "/" + ns + " " + app)
	return f.diag, f.err
}

// sent is one call or notification to Flint.
type sent struct {
	method string
	params json.RawMessage
}

// fakeFlint answers the extension's calls and records everything sent.
type fakeFlint struct {
	mu      sync.Mutex
	calls   []sent
	notes   []sent
	callErr error
	posted  chan json.RawMessage
}

func newFakeFlint() *fakeFlint { return &fakeFlint{posted: make(chan json.RawMessage, 16)} }

func raw(v any) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func (f *fakeFlint) call(method string, params, _ any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, sent{method, raw(params)})
	return f.callErr
}

func (f *fakeFlint) notify(method string, params any) {
	f.mu.Lock()
	f.notes = append(f.notes, sent{method, raw(params)})
	f.mu.Unlock()
	if method == "webview/post" {
		f.posted <- raw(params)
	}
}

// lastCall is the last call's params decoded into v.
func (f *fakeFlint) lastCall(t *testing.T, method string, v any) {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 || f.calls[len(f.calls)-1].method != method {
		t.Fatalf("calls %v, want %s last", f.calls, method)
	}
	if err := json.Unmarshal(f.calls[len(f.calls)-1].params, v); err != nil {
		t.Fatal(err)
	}
}

func (f *fakeFlint) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// fakeRefresher renews AWS credentials in tests.
type fakeRefresher struct {
	out  string
	err  error
	gate chan struct{}
}

func (r *fakeRefresher) Refresh(context.Context, awsprof.Profile) (string, error) {
	if r.gate != nil {
		<-r.gate
	}
	return r.out, r.err
}

// testExt is an extension wired to fakes, with its data in a temporary folder.
func testExt(t *testing.T, k *fakeCluster) (*ext, *fakeFlint) {
	t.Helper()
	f := newFakeFlint()
	e := newExt(f, t.TempDir())
	e.kubectl = func() (cluster, error) {
		if k == nil {
			return nil, kube.ErrNoKubectl
		}
		return k, nil
	}
	e.profiles = func() ([]awsprof.Profile, error) {
		return []awsprof.Profile{
			{Name: "dev", Kind: awsprof.KindSSO, Status: awsprof.StatusExpired, Region: "eu-central-1", Login: []string{"aws", "sso", "login", "--profile", "dev"}, Refreshable: true},
			{Name: "keys", Kind: awsprof.KindKeys, Status: awsprof.StatusStatic},
		}, nil
	}
	e.aws = &fakeRefresher{out: "Attempting to open your browser\nSuccessfully logged into Start URL"}
	return e, f
}

// webview sends a webview/request from the panel as Flint does.
func webview(e *ext, method string, params any) (any, error) {
	return e.handle("webview/request", raw(map[string]any{"view": panelID, "method": method, "params": params}))
}

var errBoom = errors.New("boom")
