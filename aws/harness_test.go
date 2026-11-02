package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/flint-ide/extensions/aws/internal/awsprof"
)

// flint plays Flint's side of the connection: it sends requests and
// notifications to the extension, answers the extension's calls, and keeps
// everything the extension sent.
type flint struct {
	t      *testing.T
	e      *ext
	toExt  *io.PipeWriter
	nextID int

	mu      sync.Mutex
	sent    []message
	answers map[string]any // the result Flint gives per called method
	changed chan struct{}
}

func newFlint(t *testing.T, d deps) *flint {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	f := &flint{t: t, toExt: inW, answers: map[string]any{}, changed: make(chan struct{}, 1)}
	f.e = newExt(newConn(outW), d)
	go func() { _ = f.e.c.serve(inR, f.e.handle, f.e.notified) }()
	go f.read(outR)
	t.Cleanup(func() {
		f.e.stop()
		_ = inW.Close()
		_ = outW.Close()
	})
	return f
}

func (f *flint) read(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		var m message
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			f.t.Errorf("not JSON-RPC on stdout: %s", sc.Bytes())
			continue
		}
		f.mu.Lock()
		f.sent = append(f.sent, m)
		answer, has := f.answers[m.Method]
		f.mu.Unlock()
		select {
		case f.changed <- struct{}{}:
		default:
		}
		if m.Method != "" && m.ID != nil { // a call to Flint
			if !has {
				answer = nil
			}
			f.write(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": answer})
		}
	}
}

func (f *flint) write(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := f.toExt.Write(append(data, '\n')); err != nil {
		f.t.Errorf("write: %v", err)
	}
}

func (f *flint) answer(method string, result any) {
	f.mu.Lock()
	f.answers[method] = result
	f.mu.Unlock()
}

// request sends a request and waits for its answer.
func (f *flint) request(method string, params any) message {
	f.t.Helper()
	f.nextID++
	id := f.nextID
	f.write(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	want, _ := json.Marshal(id)
	return f.wait("answer to "+method, func(m message) bool { return m.Method == "" && string(m.ID) == string(want) })
}

func (f *flint) notify(method string, params any) {
	f.write(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// wait returns the first message sent that matches ok.
func (f *flint) wait(what string, ok func(message) bool) message {
	f.t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		f.mu.Lock()
		sent := slices.Clone(f.sent)
		f.mu.Unlock()
		for _, m := range sent {
			if ok(m) {
				return m
			}
		}
		select {
		case <-f.changed:
		case <-time.After(10 * time.Millisecond):
		case <-deadline:
			f.t.Fatalf("no %s; sent %s", what, f.dump())
			return message{}
		}
	}
}

// count is how many messages sent match ok.
func (f *flint) count(ok func(message) bool) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, m := range f.sent {
		if ok(m) {
			n++
		}
	}
	return n
}

func (f *flint) dump() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var b strings.Builder
	for _, m := range f.sent {
		data, _ := json.Marshal(m)
		b.Write(data)
		b.WriteByte('\n')
	}
	return b.String()
}

func method(name string) func(message) bool {
	return func(m message) bool { return m.Method == name }
}

func paramsContain(name, text string) func(message) bool {
	return func(m message) bool { return m.Method == name && strings.Contains(string(m.Params), text) }
}

// resolve answers view/resolve as a view.
func (f *flint) resolve(parent string) view {
	f.t.Helper()
	m := f.request("view/resolve", map[string]string{"view": panelID, "parent": parent})
	if m.Error != nil {
		f.t.Fatalf("view/resolve: %v", m.Error)
	}
	var v view
	if err := json.Unmarshal(m.Result, &v); err != nil {
		f.t.Fatal(err)
	}
	return v
}

func (f *flint) act(item, action string) message {
	f.t.Helper()
	return f.request("view/action", map[string]string{"view": panelID, "item": item, "action": action})
}

// fakeRefresher answers Refresh from a function and notes the calls.
type fakeRefresher struct {
	mu       sync.Mutex
	calls    []string
	cancels  []string
	refresh  func(ctx context.Context, p awsprof.Profile) (string, error)
	canceled chan string
}

func (r *fakeRefresher) Refresh(ctx context.Context, p awsprof.Profile) (string, error) {
	r.mu.Lock()
	r.calls = append(r.calls, p.Name)
	r.mu.Unlock()
	return r.refresh(ctx, p)
}

func (r *fakeRefresher) Cancel(profile string) bool {
	r.mu.Lock()
	r.cancels = append(r.cancels, profile)
	r.mu.Unlock()
	if r.canceled != nil {
		r.canceled <- profile
	}
	return true
}

var testNow = time.Date(2026, 11, 2, 12, 0, 0, 0, time.UTC)

// fixture is one profile of each kind that matters to the panel.
func fixture() []awsprof.Profile {
	return []awsprof.Profile{
		{Name: "default", Kind: awsprof.KindKeys, Status: awsprof.StatusStatic, Region: "eu-central-1"},
		{Name: "dev", Kind: awsprof.KindSSO, Status: awsprof.StatusValid, Region: "eu-west-1", Account: "111111111111", Role: "Admin",
			ExpiresAt: testNow.Add(2*time.Hour + 5*time.Minute).Format(time.RFC3339), Login: []string{"aws", "sso", "login", "--profile", "dev"}, Refreshable: true},
		{Name: "old-sso", Kind: awsprof.KindSSO, Status: awsprof.StatusLoggedOut, Login: []string{"aws", "sso", "login", "--profile", "old-sso"}, Refreshable: true},
		{Name: "saml", Kind: awsprof.KindTemporary, Status: awsprof.StatusExpired, Account: "222222222222", Role: "Dev",
			ExpiresAt: testNow.Add(-time.Minute).Format(time.RFC3339),
			Login:     []string{"saml2aws", "login", "--profile", "saml", "--role", "arn:aws:iam::222222222222:role/Dev"}, Refreshable: true},
		{Name: "ops", Kind: awsprof.KindRole, Status: awsprof.StatusUnknown, Source: "default", Account: "333333333333", Role: "Ops", Refreshable: true},
	}
}

func testDeps() deps {
	return deps{
		load: func() ([]awsprof.Profile, error) { return fixture(), nil },
		identity: func(ctx context.Context, profile string) (awsprof.Identity, error) {
			return awsprof.Identity{Account: "111111111111", Arn: "arn:aws:sts::111111111111:assumed-role/Admin/jane", UserID: "AID:jane"}, nil
		},
		refresher: &fakeRefresher{refresh: func(context.Context, awsprof.Profile) (string, error) {
			return "Successfully logged into Start URL", nil
		}},
		hasCLI: func() bool { return true },
		now:    func() time.Time { return testNow },
		scale:  1,
	}
}
