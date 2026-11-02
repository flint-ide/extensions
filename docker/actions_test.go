package main

import (
	"errors"
	"slices"
	"strings"
	"testing"
)

func toasts(h *harness) []string {
	var out []string
	for _, n := range h.fl.notified("window/showMessage") {
		out = append(out, n.Params["type"].(string)+": "+n.Params["message"].(string))
	}
	return out
}

func strs(v any) []string {
	var out []string
	for _, s := range v.([]any) {
		out = append(out, s.(string))
	}
	return out
}

func TestContainerActionsCallTheEngine(t *testing.T) {
	h := newHarness(t)
	for _, a := range []string{"stop", "restart"} {
		if err := h.act(t, "container:api1", a); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.act(t, "container:lonely1", "start"); err != nil {
		t.Fatal(err)
	}
	acts, _ := h.eng.snapshot()
	if !slices.Equal(acts, []string{"stop api1", "restart api1", "start lonely1"}) {
		t.Errorf("acts %q", acts)
	}
	if got := toasts(h); !slices.Equal(got, []string{"info: Stopped api.", "info: Restarted api.", "info: Started lonely."}) {
		t.Errorf("toasts %q", got)
	}
	// Busy, then done: the panel is refreshed twice per action.
	if n := len(h.fl.notified("view/refresh")); n != 6 {
		t.Errorf("%d refreshes", n)
	}
}

func TestBusyRowShowsTheRunningAction(t *testing.T) {
	h := newHarness(t)
	h.e.busy["container:api1"] = "Stopping…"
	v := h.resolve(t, "")
	for _, it := range v.Items {
		if it.ID == "container:api1" && it.Description != "Stopping…" {
			t.Errorf("description %q", it.Description)
		}
	}
	if err := h.act(t, "container:api1", "stop"); err != nil {
		t.Fatal(err)
	}
	if acts, _ := h.eng.snapshot(); len(acts) != 0 {
		t.Errorf("a second action ran on a busy row: %q", acts)
	}
}

func TestRemoveAsksFirst(t *testing.T) {
	h := newHarness(t)
	h.fl.answers["window/showMessageRequest"] = map[string]any{} // cancelled
	if err := h.act(t, "container:lonely1", "rm"); err != nil {
		t.Fatal(err)
	}
	if acts, _ := h.eng.snapshot(); len(acts) != 0 {
		t.Fatalf("removed without a yes: %q", acts)
	}
	q := h.fl.called("window/showMessageRequest")
	if len(q) != 1 || q[0].Params["message"] != "Remove container lonely?" || !slices.Equal(strs(q[0].Params["actions"]), []string{"Remove"}) {
		t.Errorf("question %+v", q)
	}
	h.fl.answers["window/showMessageRequest"] = map[string]any{"action": "Remove"}
	if err := h.act(t, "container:lonely1", "rm"); err != nil {
		t.Fatal(err)
	}
	if acts, _ := h.eng.snapshot(); !slices.Equal(acts, []string{"rm lonely1"}) {
		t.Errorf("acts %q", acts)
	}
}

func TestFailedActionIsAnErrorToast(t *testing.T) {
	h := newHarness(t)
	h.eng.actErr = errors.New("docker: container is paused")
	if err := h.act(t, "container:api1", "stop"); err != nil {
		t.Fatal(err)
	}
	if got := toasts(h); len(got) != 1 || got[0] != "error: Couldn't stop api: docker: container is paused" {
		t.Errorf("toasts %q", got)
	}
	if len(h.e.busy) != 0 {
		t.Error("the row stays busy")
	}
}

func TestBadActionsAreRefused(t *testing.T) {
	h := newHarness(t)
	for _, c := range [][2]string{
		{"container:../etc", "stop"},
		{"container:api1", "kill"},
		{"container:api1", "port:99999"},
		{"project:/nowhere", "up"},
		{"project:/p/shop", "build"},
	} {
		if err := h.act(t, c[0], c[1]); err == nil {
			t.Errorf("%s %s accepted", c[0], c[1])
		}
	}
	if acts, composes := h.eng.snapshot(); len(acts)+len(composes) != 0 {
		t.Errorf("ran %q %q", acts, composes)
	}
	if err := h.act(t, itemWarning, "anything"); err != nil {
		t.Errorf("rows without actions: %v", err)
	}
}

func TestComposeActions(t *testing.T) {
	h := newHarness(t)
	for _, a := range []string{"up", "pull", "restart", "stop"} {
		if err := h.act(t, "project:/p/shop", a); err != nil {
			t.Fatal(err)
		}
	}
	h.fl.answers["window/showMessageRequest"] = map[string]any{"action": "Down"}
	if err := h.act(t, "project:/p/shop", "down"); err != nil {
		t.Fatal(err)
	}
	_, composes := h.eng.snapshot()
	if !slices.Equal(composes, []string{"up shop", "pull shop", "restart shop", "stop shop", "down shop"}) {
		t.Errorf("composes %q", composes)
	}
	q := h.fl.called("window/showMessageRequest")
	if len(q) != 1 || !strings.Contains(q[0].Params["message"].(string), "Stop and remove the containers of shop") {
		t.Errorf("down must ask first: %+v", q)
	}
	if got := toasts(h); got[0] != "info: Compose up finished for shop." {
		t.Errorf("toasts %q", got)
	}
}

func TestLogsAndShellOpenTerminals(t *testing.T) {
	h := newHarness(t)
	for _, c := range [][2]string{{"container:api1", "logs"}, {"container:api1", "shell"}, {"project:/p/shop", "logs"}} {
		if err := h.act(t, c[0], c[1]); err != nil {
			t.Fatal(err)
		}
	}
	calls := h.fl.called("window/openTerminal")
	if len(calls) != 3 {
		t.Fatalf("calls %+v", calls)
	}
	want := [][]string{
		{"docker", "logs", "-f", "--tail", "500", "api1"},
		{"docker", "exec", "-it", "api1", "sh", "-c", "command -v bash >/dev/null && exec bash || exec sh"},
		{"docker", "compose", "-p", "shop", "logs", "-f", "--tail", "500"},
	}
	titles := []string{"Logs api", "Shell api", "Logs shop"}
	for i, c := range calls {
		if got := strs(c.Params["command"]); !slices.Equal(got, want[i]) {
			t.Errorf("command %q, want %q", got, want[i])
		}
		if c.Params["title"] != titles[i] || c.Params["tag"] != "Docker" {
			t.Errorf("params %+v", c.Params)
		}
		if _, ok := c.Params["env"]; ok {
			t.Errorf("env without a host setting: %+v", c.Params["env"])
		}
	}
}

func TestTerminalsReachTheConfiguredEngine(t *testing.T) {
	h := newHarness(t)
	for host, env := range map[string]map[string]any{
		"https://docker.example:2376": {"DOCKER_HOST": "tcp://docker.example:2376", "DOCKER_TLS_VERIFY": "1"},
		"tcp://10.0.0.5":              {"DOCKER_HOST": "tcp://10.0.0.5:2375"},
		"/run/user/1000/docker.sock":  {"DOCKER_HOST": "unix:///run/user/1000/docker.sock"},
	} {
		h.fl = newFakeFlint()
		h.e.fl = h.fl
		h.notify(t, "settings/changed", map[string]any{"settings": map[string]any{"host": host, "logTailLines": 42}})
		if err := h.act(t, "container:api1", "logs"); err != nil {
			t.Fatal(err)
		}
		c := h.fl.called("window/openTerminal")[0]
		got, _ := c.Params["env"].(map[string]any)
		if len(got) != len(env) {
			t.Errorf("%s: env %v, want %v", host, got, env)
		}
		for k, v := range env {
			if got[k] != v {
				t.Errorf("%s: %s=%v, want %v", host, k, got[k], v)
			}
		}
		if cmd := strs(c.Params["command"]); cmd[4] != "42" {
			t.Errorf("tail %q", cmd)
		}
	}
}

func TestOldFlintWithoutTerminals(t *testing.T) {
	h := newHarness(t)
	h.fl.errs["window/openTerminal"] = &rpcError{Code: -32601, Message: "method not found"}
	if err := h.act(t, "container:api1", "logs"); err == nil || !strings.Contains(err.Error(), "update Flint") {
		t.Errorf("err = %v", err)
	}
}

func TestOpenPort(t *testing.T) {
	h := newHarness(t)
	if err := h.act(t, "container:api1", "port:8080"); err != nil {
		t.Fatal(err)
	}
	c := h.fl.called("window/openUrl")
	if len(c) != 1 || c[0].Params["url"] != "http://localhost:8080" {
		t.Errorf("calls %+v", c)
	}
}
