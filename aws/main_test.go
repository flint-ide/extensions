package main

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flint-ide/extensions/aws/internal/awsprof"
)

func TestInitializeAndUnknownMethods(t *testing.T) {
	f := newFlint(t, testDeps())
	m := f.request("initialize", map[string]any{"contract": 1, "settings": map[string]any{"refreshSeconds": 60}})
	if m.Error != nil || !strings.Contains(string(m.Result), `"contract":1`) {
		t.Fatalf("initialize = %s, %v", m.Result, m.Error)
	}
	f.e.mu.Lock()
	got := f.e.settings.RefreshSeconds
	f.e.mu.Unlock()
	if got != 60 {
		t.Errorf("refreshSeconds = %d", got)
	}
	if m := f.request("nope/never", nil); m.Error == nil || m.Error.Code != -32601 {
		t.Errorf("unknown method answered %+v", m)
	}
	if m := f.request("view/resolve", map[string]string{"view": "other.view"}); m.Error == nil {
		t.Error("resolved a view that isn't ours")
	}
	if m := f.request("shutdown", nil); m.Error != nil || string(m.Result) != "null" {
		t.Errorf("shutdown = %s, %v", m.Result, m.Error)
	}
}

func TestRootListsProfilesWithStatusAndActions(t *testing.T) {
	f := newFlint(t, testDeps())
	v := f.resolve("")
	if v.Kind != "tree" || len(v.Items) != 5 {
		t.Fatalf("view = %+v", v)
	}
	byID := map[string]item{}
	for _, it := range v.Items {
		byID[it.ID] = it
		if it.Icon != "cloud" || !it.HasChildren || it.Label == "" {
			t.Errorf("item %+v", it)
		}
	}
	want := map[string]string{
		"profile:default": "keys · access keys · eu-central-1",
		"profile:dev":     "2h 05m · SSO · 111111111111 · eu-west-1",
		"profile:old-sso": "logged out · SSO",
		"profile:saml":    "expired · session keys · 222222222222",
		"profile:ops":     "— · assumed role · 333333333333",
	}
	for id, desc := range want {
		if got := byID[id].Description; got != desc {
			t.Errorf("%s description = %q, want %q", id, got, desc)
		}
	}
	if tip := byID["profile:ops"].Tooltip; !strings.Contains(tip, "Uses the credentials of default") || !strings.Contains(tip, "role Ops") {
		t.Errorf("ops tooltip = %q", tip)
	}
	ids := func(it item) []string {
		var out []string
		for _, a := range it.Actions {
			out = append(out, a.ID)
		}
		return out
	}
	if got := ids(byID["profile:dev"]); !slices.Equal(got, []string{actTerminal, actRefresh, actLogin, actIdentity}) {
		t.Errorf("dev actions = %v", got)
	}
	if got := ids(byID["profile:default"]); !slices.Equal(got, []string{actTerminal, actIdentity}) {
		t.Errorf("default actions = %v", got)
	}
	if got := ids(byID["profile:ops"]); !slices.Equal(got, []string{actTerminal, actRefresh, actIdentity}) {
		t.Errorf("ops actions = %v", got)
	}
}

func TestChildrenShowDetails(t *testing.T) {
	f := newFlint(t, testDeps())
	f.resolve("")
	v := f.resolve("profile:dev")
	if v.Kind != "tree" {
		t.Fatalf("kind %s", v.Kind)
	}
	var labels []string
	for _, it := range v.Items {
		labels = append(labels, it.Label+"="+it.Description)
		if it.HasChildren {
			t.Errorf("detail %s has children", it.ID)
		}
		if name, ok := profileName(it.ID); !ok || name != "dev" {
			t.Errorf("item %s doesn't name its profile", it.ID)
		}
	}
	got := strings.Join(labels, "\n")
	for _, w := range []string{"Kind=SSO", "Account=111111111111", "Role=Admin", "Region=eu-west-1", "Log in with=aws sso login --profile dev", "Terminal=AWS_PROFILE=dev"} {
		if !strings.Contains(got, w) {
			t.Errorf("children lack %q:\n%s", w, got)
		}
	}
	if v := f.resolve("profile:missing"); len(v.Items) != 0 {
		t.Errorf("children of a missing profile: %+v", v)
	}
}

func TestMessageViews(t *testing.T) {
	d := testDeps()
	d.load = func() ([]awsprof.Profile, error) { return nil, nil }
	d.hasCLI = func() bool { return false }
	f := newFlint(t, d)
	v := f.resolve("")
	if v.Kind != "message" || !strings.Contains(v.Text, "No profiles in ~/.aws/config") || !strings.Contains(v.Text, "isn't installed") {
		t.Errorf("no profiles, no CLI: %+v", v)
	}
	if len(v.Actions) != 1 || v.Actions[0].ID != actReload {
		t.Errorf("actions %+v", v.Actions)
	}
	if m := f.act("", actReload); m.Error != nil {
		t.Errorf("reload: %v", m.Error)
	}
	f.wait("view/refresh", method("view/refresh"))

	d = testDeps()
	d.load = func() ([]awsprof.Profile, error) { return nil, errors.New("permission denied") }
	f = newFlint(t, d)
	if v := f.resolve(""); v.Kind != "message" || !strings.Contains(v.Text, "permission denied") {
		t.Errorf("load error: %+v", v)
	}
}

// terminalParams is what the extension asked of window/openTerminal.
type terminalParams struct {
	Title   string            `json:"title"`
	Env     map[string]string `json:"env"`
	Command []string          `json:"command"`
	Tag     string            `json:"tag"`
}

func lastTerminal(t *testing.T, f *flint) terminalParams {
	t.Helper()
	var p terminalParams
	var last message
	f.mu.Lock()
	for _, m := range f.sent {
		if m.Method == "window/openTerminal" {
			last = m
		}
	}
	f.mu.Unlock()
	if err := json.Unmarshal(last.Params, &p); err != nil {
		t.Fatalf("no terminal: %v", err)
	}
	return p
}

func TestTerminalsCarryTheProfile(t *testing.T) {
	f := newFlint(t, testDeps())
	f.resolve("")

	if m := f.act("profile:dev", actTerminal); m.Error != nil {
		t.Fatal(m.Error)
	}
	p := lastTerminal(t, f)
	if p.Tag != "aws:dev" || len(p.Command) != 0 || p.Env["AWS_PROFILE"] != "dev" || p.Env["AWS_REGION"] != "eu-west-1" || len(p.Env) != 2 {
		t.Errorf("terminal = %+v", p)
	}

	if m := f.act("profile:saml", actLogin); m.Error != nil {
		t.Fatal(m.Error)
	}
	p = lastTerminal(t, f)
	if p.Title != "aws login: saml" || !slices.Equal(p.Command, fixture()[3].Login) || p.Env["AWS_PROFILE"] != "saml" || len(p.Env) != 1 {
		t.Errorf("login terminal = %+v", p)
	}
	for k := range p.Env {
		if k == "PATH" || k == "HOME" || k == "SHELL" || strings.HasPrefix(k, "FLINT_") {
			t.Errorf("sets %s", k)
		}
	}

	// A detail row's menu works on its profile too.
	if m := f.act("detail:region:dev", actTerminal); m.Error != nil {
		t.Error(m.Error)
	}
	// No login command, no terminal.
	if m := f.act("profile:default", actLogin); m.Error == nil {
		t.Error("logged in to a profile without a login command")
	}
	if m := f.act("profile:missing", actTerminal); m.Error == nil {
		t.Error("terminal for a profile that doesn't exist")
	}
	if m := f.act("profile:dev", "explode"); m.Error == nil {
		t.Error("unknown action answered")
	}
}

func TestInvalidProfileNameGetsNoTerminal(t *testing.T) {
	d := testDeps()
	d.load = func() ([]awsprof.Profile, error) {
		return []awsprof.Profile{{Name: "--debug", Kind: awsprof.KindConfig, Login: []string{"aws", "sso", "login"}}}, nil
	}
	f := newFlint(t, d)
	f.resolve("")
	if m := f.act("profile:--debug", actTerminal); m.Error == nil {
		t.Error("terminal opened for an invalid name")
	}
	if m := f.act("profile:--debug", actLogin); m.Error == nil {
		t.Error("login opened for an invalid name")
	}
	if n := f.count(method("window/openTerminal")); n != 0 {
		t.Errorf("%d terminals", n)
	}
}

func TestCheckIdentityShowsUnderTheProfile(t *testing.T) {
	d := testDeps()
	release := make(chan struct{})
	d.identity = func(ctx context.Context, profile string) (awsprof.Identity, error) {
		<-release
		if profile == "saml" {
			return awsprof.Identity{}, errors.New("The security token included in the request is expired")
		}
		return awsprof.Identity{Account: "111111111111", Arn: "arn:aws:sts::111111111111:assumed-role/Admin/jane"}, nil
	}
	f := newFlint(t, d)
	f.resolve("")
	if m := f.act("profile:dev", actIdentity); m.Error != nil {
		t.Fatal(m.Error)
	}
	if v := f.resolve("profile:dev"); v.Items[0].Label != "Checking…" {
		t.Errorf("first row while checking = %+v", v.Items[0])
	}
	before := f.count(method("view/refresh"))
	close(release)
	f.wait("view/refresh after the check", func(message) bool { return f.count(method("view/refresh")) > before })
	v := f.resolve("profile:dev")
	if v.Items[0].ID != "identity:dev" || v.Items[0].Label != "111111111111 · arn:aws:sts::111111111111:assumed-role/Admin/jane" {
		t.Errorf("identity row = %+v", v.Items[0])
	}
	if root := f.resolve(""); !strings.Contains(root.Items[1].Tooltip, "signed in as arn:aws:sts::111111111111") {
		t.Errorf("tooltip = %q", root.Items[1].Tooltip)
	}

	f.act("identity:saml", actIdentity)
	deadline := time.Now().Add(5 * time.Second)
	for {
		v = f.resolve("profile:saml")
		if v.Items[0].Label != "Checking…" || time.Now().After(deadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(v.Items[0].Label, "expired") {
		t.Errorf("failed check = %+v", v.Items[0])
	}
}

func TestRefreshCredentialsSucceeds(t *testing.T) {
	d := testDeps()
	d.scale = 1000 // the success note goes after 8 ms
	release := make(chan struct{})
	r := &fakeRefresher{refresh: func(context.Context, awsprof.Profile) (string, error) {
		<-release
		return "Attempting to open your browser\nSuccessfully logged into Start URL: https://x.awsapps.com/start\n", nil
	}}
	d.refresher = r
	f := newFlint(t, d)
	f.resolve("")
	if m := f.act("profile:dev", actRefresh); m.Error != nil {
		t.Fatal(m.Error)
	}
	root := f.resolve("")
	if !strings.HasPrefix(root.Items[1].Description, "browser…") || root.Items[1].Actions[1].ID != actStop {
		t.Errorf("running = %+v", root.Items[1])
	}
	if c := f.resolve("profile:dev"); c.Items[0].Label != "Approve the login in your browser…" {
		t.Errorf("running row = %+v", c.Items[0])
	}
	// A second click while it runs does nothing.
	f.act("profile:dev", actRefresh)
	close(release)
	f.wait("success toast", paramsContain("window/showMessage", `"message":"Refreshed dev"`))
	r.mu.Lock()
	calls := slices.Clone(r.calls)
	r.mu.Unlock()
	if !slices.Equal(calls, []string{"dev"}) {
		t.Errorf("refreshes = %v", calls)
	}
	// The note goes away by itself.
	deadline := time.Now().Add(5 * time.Second)
	for {
		c := f.resolve("profile:dev")
		if c.Items[0].ID != "refresh:dev" {
			break
		}
		if c.Items[0].Label != "Successfully logged into Start URL: https://x.awsapps.com/start" && c.Items[0].Label != "" {
			t.Fatalf("note = %q", c.Items[0].Label)
		}
		if time.Now().After(deadline) {
			t.Fatal("the success note stayed")
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestRefreshCanBeStopped(t *testing.T) {
	d := testDeps()
	r := &fakeRefresher{canceled: make(chan string, 1)}
	r.refresh = func(ctx context.Context, p awsprof.Profile) (string, error) {
		<-r.canceled
		return "", errors.New("refresh cancelled")
	}
	d.refresher = r
	f := newFlint(t, d)
	f.resolve("")
	f.act("profile:ops", actRefresh)
	if m := f.act("refresh:ops", actStop); m.Error != nil {
		t.Fatal(m.Error)
	}
	f.wait("error toast", paramsContain("window/showMessage", "Refreshing ops failed: refresh cancelled"))
	c := f.resolve("profile:ops")
	if c.Items[0].ID != "refresh:ops" || c.Items[0].Label != "refresh cancelled" || c.Items[0].Actions[0].ID != actDismiss {
		t.Errorf("failed row = %+v", c.Items[0])
	}
	f.act("refresh:ops", actDismiss)
	if c := f.resolve("profile:ops"); c.Items[0].ID == "refresh:ops" {
		t.Error("dismissed note stayed")
	}
}

func TestRefreshThatNeedsInputOffersTheLogin(t *testing.T) {
	d := testDeps()
	d.refresher = &fakeRefresher{refresh: func(context.Context, awsprof.Profile) (string, error) {
		return "", awsprof.ErrNeedsInput
	}}
	f := newFlint(t, d)
	f.answer("window/showMessageRequest", map[string]string{"action": "Log In"})
	f.resolve("")
	f.act("profile:saml", actRefresh)
	q := f.wait("question", method("window/showMessageRequest"))
	if !strings.Contains(string(q.Params), `"actions":["Log In"]`) || !strings.Contains(string(q.Params), "needs input") {
		t.Errorf("question = %s", q.Params)
	}
	f.wait("login terminal", method("window/openTerminal"))
	if p := lastTerminal(t, f); p.Title != "aws login: saml" || p.Command[0] != "saml2aws" {
		t.Errorf("terminal = %+v", p)
	}
}

func TestRefreshRefusesWhatHasNothingToRefresh(t *testing.T) {
	f := newFlint(t, testDeps())
	f.resolve("")
	if m := f.act("profile:default", actRefresh); m.Error == nil {
		t.Error("refreshed long-term keys")
	}
}

func TestRefreshCommandReadsAgain(t *testing.T) {
	d := testDeps()
	reads := 0
	d.load = func() ([]awsprof.Profile, error) { reads++; return fixture(), nil }
	f := newFlint(t, d)
	if m := f.request("command/execute", map[string]any{"id": refreshID}); m.Error != nil {
		t.Fatal(m.Error)
	}
	f.wait("view/refresh", paramsContain("view/refresh", panelID))
	if reads != 1 {
		t.Errorf("read %d times", reads)
	}
	if m := f.request("command/execute", map[string]any{"id": "flint.aws.other"}); m.Error == nil {
		t.Error("unknown command answered")
	}
}

func TestPollsOnlyWhileVisible(t *testing.T) {
	d := testDeps()
	d.scale = 1000 // seconds become milliseconds
	f := newFlint(t, d)
	f.request("initialize", map[string]any{"settings": map[string]any{"refreshSeconds": 5}})
	refreshes := func() int { return f.count(method("view/refresh")) }

	time.Sleep(30 * time.Millisecond)
	if n := refreshes(); n != 0 {
		t.Fatalf("%d refreshes while hidden", n)
	}
	f.notify("view/visible", map[string]any{"view": panelID, "visible": true})
	f.wait("polling", func(message) bool { return refreshes() >= 3 })

	f.notify("view/visible", map[string]any{"view": panelID, "visible": false})
	f.request("initialize", map[string]any{}) // a round trip: the notification was handled
	time.Sleep(10 * time.Millisecond)
	stopped := refreshes()
	time.Sleep(40 * time.Millisecond)
	if n := refreshes(); n != stopped {
		t.Errorf("still polling when hidden: %d → %d", stopped, n)
	}

	// Another view's visibility changes nothing.
	f.notify("view/visible", map[string]any{"view": "other.view", "visible": true})
	f.request("initialize", map[string]any{})
	f.e.mu.Lock()
	polling := f.e.poll != nil
	f.e.mu.Unlock()
	if polling {
		t.Error("polls for another view")
	}
}

func TestSettingsChangeTheInterval(t *testing.T) {
	d := testDeps()
	d.scale = 1000
	f := newFlint(t, d)
	f.notify("view/visible", map[string]any{"view": panelID, "visible": true})
	f.notify("settings/changed", map[string]any{"settings": map[string]any{"refreshSeconds": 3600}})
	f.request("initialize", map[string]any{"settings": map[string]any{"refreshSeconds": 3600}})
	f.e.mu.Lock()
	got := f.e.settings.interval()
	f.e.mu.Unlock()
	if got != time.Hour {
		t.Errorf("interval = %s", got)
	}
	// 3.6 s between reads now: none in the next 50 ms.
	before := f.count(method("view/refresh"))
	time.Sleep(50 * time.Millisecond)
	if n := f.count(method("view/refresh")); n != before {
		t.Errorf("%d refreshes after slowing down", n-before)
	}
}

func TestIntervalBounds(t *testing.T) {
	for in, want := range map[int]time.Duration{0: 30 * time.Second, 1: 5 * time.Second, 30: 30 * time.Second, 99999: time.Hour} {
		if got := (settings{RefreshSeconds: in}).interval(); got != want {
			t.Errorf("interval(%d) = %s, want %s", in, got, want)
		}
	}
}
