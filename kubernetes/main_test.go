package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flint-ide/extensions/kubernetes/internal/kube"
)

var devContexts = []kube.Context{
	{Name: "other", Cluster: "c-other", User: "u-other"},
	{Name: "dev", Cluster: "kind-dev", User: "u-dev", Namespace: "shop", Current: true, AWSProfile: "dev"},
}

func decode[T any](t *testing.T, v any) T {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var out T
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%s: %v", data, err)
	}
	return out
}

func TestInitializeAndSettings(t *testing.T) {
	e, _ := testExt(t, &fakeCluster{})
	res, err := e.handle("initialize", raw(map[string]any{"contract": 1, "settings": map[string]any{"refreshSeconds": 30}}))
	if err != nil || decode[map[string]any](t, res)["contract"] != float64(1) {
		t.Fatalf("initialize = %v, %v", res, err)
	}
	got, err := webview(e, "settings", nil)
	if err != nil || decode[map[string]int](t, got)["refreshSeconds"] != 30 {
		t.Fatalf("settings = %s, %v", got, err)
	}
	// Out of bounds: Flint's default.
	e.handle("initialize", raw(map[string]any{"settings": map[string]any{"refreshSeconds": 9999}}))
	got, _ = webview(e, "settings", nil)
	if decode[map[string]int](t, got)["refreshSeconds"] != defaultRefreshSeconds {
		t.Errorf("settings = %s", got)
	}
	if _, err := e.handle("nope", nil); err == nil || err.(*rpcError).Code != -32601 {
		t.Errorf("unknown method: %v", err)
	}
}

func TestReadRequests(t *testing.T) {
	k := &fakeCluster{contexts: devContexts, ov: kube.Overview{Pods: []kube.Pod{{Name: "web-1", Status: "Running"}}}}
	e, _ := testExt(t, k)
	got, err := webview(e, "contexts", nil)
	if err != nil || len(decode[[]kube.Context](t, got)) != 2 {
		t.Fatalf("contexts = %s, %v", got, err)
	}
	got, err = webview(e, "namespaces", map[string]string{"context": "dev"})
	if err != nil || !slices.Equal(decode[[]string](t, got), []string{"default", "shop"}) {
		t.Fatalf("namespaces = %s, %v", got, err)
	}
	got, err = webview(e, "overview", map[string]string{"context": "dev", "namespace": "shop"})
	if err != nil || decode[kube.Overview](t, got).Pods[0].Name != "web-1" {
		t.Fatalf("overview = %s, %v", got, err)
	}
	if want := []string{"contexts", "namespaces dev", "overview dev/shop"}; !slices.Equal(k.calls(), want) {
		t.Errorf("asked %v, want %v", k.calls(), want)
	}
	// kubectl's error reaches the page as is.
	k.err = errBoom
	if _, err := webview(e, "overview", map[string]string{"context": "dev", "namespace": "shop"}); err == nil || err.Error() != "boom" {
		t.Errorf("error = %v", err)
	}
}

func TestNoKubectl(t *testing.T) {
	e, _ := testExt(t, nil)
	if _, err := webview(e, "contexts", nil); err == nil || !strings.Contains(err.Error(), "kubectl is not installed") {
		t.Errorf("error = %v", err)
	}
	if _, err := e.handle("agentTool/call", raw(map[string]any{"name": "status", "arguments": map[string]any{}})); err == nil {
		t.Error("tool answered without kubectl")
	}
}

func TestRequestChecks(t *testing.T) {
	e, _ := testExt(t, &fakeCluster{})
	if _, err := e.handle("webview/request", raw(map[string]any{"view": "flint.kubernetes.other", "method": "contexts"})); err == nil {
		t.Error("a request for another view was answered")
	}
	if _, err := webview(e, "nope", nil); err == nil {
		t.Error("an unknown request was answered")
	}
	if _, err := webview(e, "scale", map[string]any{"context": "dev", "kind": "Deployment", "name": "web"}); err == nil {
		t.Error("scale without replicas was accepted")
	}
	if _, err := webview(e, "overview", "not an object"); err == nil {
		t.Error("invalid params were accepted")
	}
}

func TestActions(t *testing.T) {
	k := &fakeCluster{contexts: devContexts}
	e, _ := testExt(t, k)
	s := map[string]any{"context": "dev", "namespace": "shop"}
	with := func(kv ...any) map[string]any {
		out := map[string]any{}
		for k, v := range s {
			out[k] = v
		}
		for i := 0; i < len(kv); i += 2 {
			out[kv[i].(string)] = kv[i+1]
		}
		return out
	}
	for _, r := range []struct {
		method string
		params map[string]any
	}{
		{"restart", with("kind", "Deployment", "name", "web")},
		{"undo", with("kind", "StatefulSet", "name", "db")},
		{"scale", with("kind", "Deployment", "name", "web", "replicas", 3)},
		{"deletePod", with("name", "web-1")},
		{"useContext", map[string]any{"context": "other"}},
	} {
		if _, err := webview(e, r.method, r.params); err != nil {
			t.Fatalf("%s: %v", r.method, err)
		}
	}
	want := []string{"restart dev/shop Deployment/web", "undo dev/shop StatefulSet/db", "scale dev/shop Deployment/web |||", "delete dev/shop web-1", "use other"}
	if !slices.Equal(k.calls(), want) {
		t.Errorf("asked %v, want %v", k.calls(), want)
	}
}

func TestTerminalRequests(t *testing.T) {
	e, f := testExt(t, &fakeCluster{})
	ctx := "arn:aws:eks:eu-central-1:1:cluster/prod"
	for _, c := range []struct {
		method string
		params map[string]any
		title  string
		argv   string
	}{
		{"logs", map[string]any{"context": ctx, "namespace": "apps", "target": "deployment/api"}, "logs: api",
			"kubectl --context=" + ctx + " --namespace=apps logs -f --tail=200 --all-containers --prefix deployment/api"},
		{"exec", map[string]any{"context": "docker-desktop", "namespace": "default", "pod": "web-1", "container": "app"}, "sh: web-1",
			"kubectl --context=docker-desktop --namespace=default exec -it web-1 -c app -- sh -c command -v bash >/dev/null && exec bash || exec sh"},
		{"exec", map[string]any{"context": "dev", "namespace": "shop", "pod": "web-1"}, "sh: web-1",
			"kubectl --context=dev --namespace=shop exec -it web-1 -- sh -c command -v bash >/dev/null && exec bash || exec sh"},
		{"describe", map[string]any{"context": "dev", "namespace": "shop", "kind": "Pod", "name": "web-1"}, "describe: web-1",
			"kubectl --context=dev --namespace=shop describe pod/web-1"},
		{"describe", map[string]any{"context": "dev", "namespace": "shop", "kind": "kustomizations.kustomize.toolkit.fluxcd.io", "name": "shop"}, "describe: shop",
			"kubectl --context=dev --namespace=shop describe kustomizations.kustomize.toolkit.fluxcd.io/shop"},
		{"portForward", map[string]any{"context": "dev", "namespace": "shop", "target": "service/web", "port": 8080}, ":8080 → web",
			"kubectl --context=dev --namespace=shop port-forward service/web 8080:8080"},
		{"rolloutStatus", map[string]any{"context": "dev", "namespace": "shop", "kind": "Deployment", "name": "web"}, "rollout: web",
			"kubectl --context=dev --namespace=shop rollout status deployment/web"},
		{"rolloutHistory", map[string]any{"context": "dev", "namespace": "shop", "kind": "StatefulSet", "name": "db"}, "history: db",
			"kubectl --context=dev --namespace=shop rollout history statefulset/db"},
		{"reconcile", map[string]any{"context": "prod", "namespace": "flux-system", "name": "shop"}, "flux: shop",
			"flux --context=prod reconcile kustomization shop --namespace=flux-system --with-source"},
	} {
		if _, err := webview(e, c.method, c.params); err != nil {
			t.Fatalf("%s: %v", c.method, err)
		}
		var p struct {
			Title   string   `json:"title"`
			Command []string `json:"command"`
		}
		f.lastCall(t, "window/openTerminal", &p)
		if p.Title != c.title || strings.Join(p.Command, " ") != c.argv {
			t.Errorf("%s: %q %q\nwant %q %q", c.method, p.Title, strings.Join(p.Command, " "), c.title, c.argv)
		}
	}
	// Values that could read as flags or aren't names never reach a terminal.
	n := f.callCount()
	for _, bad := range []struct {
		method string
		params map[string]any
	}{
		{"logs", map[string]any{"context": "-x", "namespace": "a", "target": "pod/a"}},
		{"logs", map[string]any{"context": "dev", "namespace": "a", "target": "--all"}},
		{"logs", map[string]any{"context": "dev", "namespace": "A B", "target": "pod/a"}},
		{"exec", map[string]any{"context": "dev", "pod": "-it"}},
		{"exec", map[string]any{"context": "dev", "pod": "a", "container": "--x"}},
		{"describe", map[string]any{"context": "dev", "kind": "Pod", "name": "-v"}},
		{"portForward", map[string]any{"context": "dev", "target": "pod/a", "port": 0}},
		{"reconcile", map[string]any{"context": "dev", "name": "shop"}},
		{"logs", map[string]any{"context": "dev\n", "target": "pod/a"}},
	} {
		if _, err := webview(e, bad.method, bad.params); err == nil {
			t.Errorf("%s %v accepted", bad.method, bad.params)
		}
	}
	if f.callCount() != n {
		t.Error("a refused request opened a terminal")
	}
}

func TestShellUsesAnOverrideKubeconfig(t *testing.T) {
	t.Setenv("KUBECONFIG", "/home/me/.kube/config")
	e, f := testExt(t, &fakeCluster{contexts: devContexts})
	if _, err := webview(e, "shell", map[string]string{"context": "dev", "namespace": "apps"}); err != nil {
		t.Fatal(err)
	}
	var p struct {
		Env     map[string]string `json:"env"`
		Tag     string            `json:"tag"`
		Command []string          `json:"command"`
	}
	f.lastCall(t, "window/openTerminal", &p)
	if p.Tag != "k8s:dev/apps" || len(p.Command) != 0 {
		t.Errorf("tag %q, command %v", p.Tag, p.Command)
	}
	first, rest, _ := strings.Cut(p.Env["KUBECONFIG"], string(os.PathListSeparator))
	if rest != "/home/me/.kube/config" || filepath.Dir(first) != filepath.Join(e.data, "kube") {
		t.Fatalf("KUBECONFIG = %q", p.Env["KUBECONFIG"])
	}
	data, err := os.ReadFile(first)
	if err != nil || !strings.Contains(string(data), `current-context: "dev"`) || !strings.Contains(string(data), `namespace: "apps"`) {
		t.Errorf("override = %s, %v", data, err)
	}
	if st, err := os.Stat(first); err != nil || st.Mode().Perm() != 0o600 {
		t.Errorf("override mode: %v %v", st.Mode(), err)
	}
	if _, err := webview(e, "shell", map[string]string{"context": "gone", "namespace": "apps"}); err == nil {
		t.Error("a context not in the kubeconfig was accepted")
	}
}

func TestOverrideKubeconfigSetsOnlyContextAndNamespace(t *testing.T) {
	c := kube.Context{Name: "arn:aws:eks:eu-central-1:1:cluster/prod", Cluster: "arn:aws:eks:eu-central-1:1:cluster/prod", User: "u: \"x\"", Namespace: "default"}
	got := string(overrideKubeconfig(c, "apps"))
	want := `apiVersion: v1
kind: Config
current-context: "arn:aws:eks:eu-central-1:1:cluster/prod"
contexts:
- name: "arn:aws:eks:eu-central-1:1:cluster/prod"
  context:
    cluster: "arn:aws:eks:eu-central-1:1:cluster/prod"
    user: "u: \"x\""
    namespace: "apps"
`
	if got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}
	if strings.Contains(got, "users:") || strings.Contains(got, "clusters:") {
		t.Fatal("credentials or clusters copied")
	}
}

func TestDiagnoseAsksTheAgent(t *testing.T) {
	k := &fakeCluster{diag: "Problems:\n- pod crashed\n"}
	e, f := testExt(t, k)
	if _, err := webview(e, "diagnose", map[string]string{"context": "prod", "namespace": "shop", "app": "flux:shop", "name": "shop"}); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(k.calls(), []string{"diagnose prod/shop flux:shop"}) {
		t.Errorf("asked %v", k.calls())
	}
	var p struct{ Prompt string }
	f.lastCall(t, "agent/ask", &p)
	if !strings.Contains(p.Prompt, `"shop" in namespace shop (context prod)`) || !strings.HasSuffix(p.Prompt, "- pod crashed") {
		t.Errorf("prompt:\n%s", p.Prompt)
	}
	if _, err := webview(e, "diagnose", map[string]string{"context": "prod", "namespace": "shop"}); err == nil {
		t.Error("diagnose without an app was accepted")
	}
}

func TestStateIsRemembered(t *testing.T) {
	e, _ := testExt(t, &fakeCluster{})
	got, err := webview(e, "state", nil)
	if err != nil || decode[uiState](t, got).View != viewApps {
		t.Fatalf("empty state = %s, %v", got, err)
	}
	in := map[string]any{"context": "dev", "view": "resources", "namespaces": map[string]string{"dev": "shop", "bad": "Not A Name"}}
	if _, err := webview(e, "saveState", map[string]any{"state": in}); err != nil {
		t.Fatal(err)
	}
	s := decode[uiState](t, must(webview(e, "state", nil)))
	if s.Context != "dev" || s.View != viewResources || len(s.Namespaces) != 1 || s.Namespaces["dev"] != "shop" {
		t.Errorf("state = %+v", s)
	}
	if _, err := webview(e, "saveState", map[string]any{"state": strings.Repeat("x", maxStateBytes)}); err == nil {
		t.Error("a huge state was saved")
	}
}

func must(v any, err error) any {
	if err != nil {
		panic(err)
	}
	return v
}

func TestAWSLoginAndRefresh(t *testing.T) {
	e, f := testExt(t, &fakeCluster{})
	got, err := webview(e, "awsProfile", map[string]string{"profile": "dev"})
	if err != nil || decode[map[string]any](t, got)["status"] != "expired" {
		t.Fatalf("awsProfile = %s, %v", got, err)
	}
	if got, err := webview(e, "awsProfile", map[string]string{"profile": "missing"}); err != nil || string(got.(json.RawMessage)) != "null" {
		t.Errorf("missing profile = %s, %v", got, err)
	}
	if _, err := webview(e, "awsLogin", map[string]string{"profile": "dev"}); err != nil {
		t.Fatal(err)
	}
	var p struct {
		Title   string            `json:"title"`
		Command []string          `json:"command"`
		Env     map[string]string `json:"env"`
	}
	f.lastCall(t, "window/openTerminal", &p)
	if p.Title != "aws login: dev" || strings.Join(p.Command, " ") != "aws sso login --profile dev" || p.Env["AWS_PROFILE"] != "dev" || p.Env["AWS_REGION"] != "eu-central-1" {
		t.Errorf("login terminal = %+v", p)
	}
	if _, err := webview(e, "awsLogin", map[string]string{"profile": "keys"}); err == nil {
		t.Error("a profile without a login command opened a terminal")
	}

	// A refresh answers at once and posts how it ended.
	if _, err := webview(e, "awsRefresh", map[string]string{"profile": "dev"}); err != nil {
		t.Fatal(err)
	}
	select {
	case m := <-f.posted:
		var post struct {
			View    string
			Message struct {
				Type, Profile, Message string
				OK                     bool
			}
		}
		if err := json.Unmarshal(m, &post); err != nil {
			t.Fatal(err)
		}
		if post.View != panelID || post.Message.Type != "aws" || !post.Message.OK || post.Message.Message != "Successfully logged into Start URL" {
			t.Errorf("posted %s", m)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no message after the refresh")
	}
	if _, err := webview(e, "awsRefresh", map[string]string{"profile": "keys"}); err == nil {
		t.Error("a profile that can't be refreshed was refreshed")
	}
	if _, err := webview(e, "awsProfile", map[string]string{"profile": "bad name;"}); err == nil {
		t.Error("an invalid profile name was accepted")
	}
}

func TestAWSRefreshRunsOncePerProfile(t *testing.T) {
	e, f := testExt(t, &fakeCluster{})
	r := &fakeRefresher{err: errBoom, gate: make(chan struct{})}
	e.aws = r
	for range 3 {
		if _, err := webview(e, "awsRefresh", map[string]string{"profile": "dev"}); err != nil {
			t.Fatal(err)
		}
	}
	close(r.gate)
	m := <-f.posted
	if !strings.Contains(string(m), `"ok":false`) || !strings.Contains(string(m), `"error":"boom"`) {
		t.Errorf("posted %s", m)
	}
	select {
	case m := <-f.posted:
		t.Errorf("a second refresh ran: %s", m)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestNotificationsReachThePage(t *testing.T) {
	e, f := testExt(t, &fakeCluster{})
	e.notified("settings/changed", raw(map[string]any{"settings": map[string]any{"refreshSeconds": 12}}))
	if m := string(<-f.posted); !strings.Contains(m, `"refreshSeconds":12`) || !strings.Contains(m, `"type":"settings"`) {
		t.Errorf("posted %s", m)
	}
	e.notified("view/visible", raw(map[string]any{"view": panelID, "visible": false}))
	if m := string(<-f.posted); !strings.Contains(m, `"visible":false`) {
		t.Errorf("posted %s", m)
	}
	if _, err := e.handle("command/execute", raw(map[string]string{"id": refreshID})); err != nil {
		t.Fatal(err)
	}
	if m := string(<-f.posted); !strings.Contains(m, `"type":"refresh"`) {
		t.Errorf("posted %s", m)
	}
	if _, err := e.handle("command/execute", raw(map[string]string{"id": "flint.kubernetes.nope"})); err == nil {
		t.Error("an unknown command ran")
	}
}

func TestAgentTool(t *testing.T) {
	k := &fakeCluster{contexts: devContexts, ov: kube.Overview{
		Apps: []kube.App{{Name: "web", Source: "helm", Health: kube.HealthBad, Problems: []string{"0/2 pods ready"}}},
	}}
	e, _ := testExt(t, k)
	res, err := e.handle("agentTool/call", raw(map[string]any{"name": "status", "root": "/p", "arguments": map[string]any{"namespace": "kube-system"}}))
	if err != nil {
		t.Fatal(err)
	}
	text := res.(map[string]string)["text"]
	if !strings.Contains(text, "Context: dev (cluster kind-dev), namespace: kube-system") || !strings.Contains(text, "- web (helm, bad): 0/2 pods ready") {
		t.Errorf("text:\n%s", text)
	}
	if _, err := e.handle("agentTool/call", raw(map[string]any{"name": "other"})); err == nil {
		t.Error("an unknown tool answered")
	}
}

func TestBoundOverview(t *testing.T) {
	ov := kube.Overview{}
	for i := range 2000 {
		ov.Pods = append(ov.Pods, kube.Pod{Name: "pod-" + strings.Repeat("x", 100) + string(rune('a'+i%26)), Status: "Running"})
		ov.Events = append(ov.Events, kube.Event{Kind: "Pod", Name: "p", Message: strings.Repeat("m", 100)})
	}
	small := boundOverview(kube.Overview{Pods: ov.Pods[:2]}, 1<<20)
	if len(small.Pods) != 2 || len(small.Partial) != 0 {
		t.Errorf("a small overview was changed: %+v", small.Partial)
	}
	cut := boundOverview(ov, 100<<10)
	data, _ := json.Marshal(cut)
	if len(data) > 100<<10 {
		t.Fatalf("%d bytes", len(data))
	}
	if len(cut.Partial) == 0 || !strings.HasPrefix(cut.Partial[0], "events (first") {
		t.Errorf("partial = %v", cut.Partial)
	}
	if len(ov.Pods) != 2000 || len(ov.Partial) != 0 {
		t.Error("the input was changed")
	}
}
