package main

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestInitializeNormalizesSettings(t *testing.T) {
	h := newHarness(t)
	res, err := h.request(t, "initialize", map[string]any{
		"contract": 1, "settings": map[string]any{"host": "  ", "logTailLines": 0},
		"roots": []map[string]any{{"path": "/p", "trusted": true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["contract"] != 1 {
		t.Errorf("answer %v", res)
	}
	if s := h.e.settings; s.Host != "" || s.LogTailLines != 500 || !slices.Equal(h.e.roots, []string{"/p"}) {
		t.Errorf("settings %+v roots %v", s, h.e.roots)
	}
	if _, err := h.request(t, "shutdown", map[string]any{}); err != nil {
		t.Fatal(err)
	}
}

func TestUnknownRequests(t *testing.T) {
	h := newHarness(t)
	var re *rpcError
	if _, err := h.request(t, "nope", nil); !errors.As(err, &re) || re.Code != -32601 {
		t.Errorf("unknown method: %v", err)
	}
	if _, err := h.request(t, "view/resolve", map[string]string{"view": "flint.docker.other"}); !errors.As(err, &re) || re.Code != -32602 {
		t.Errorf("unknown view: %v", err)
	}
	if _, err := h.request(t, "command/execute", map[string]string{"id": "flint.docker.nope"}); err == nil {
		t.Error("unknown command accepted")
	}
}

func TestSettingsChangedReconnectsAndRefreshes(t *testing.T) {
	h := newHarness(t)
	h.resolve(t, "")
	h.resolve(t, "") // same engine: the client is reused
	h.notify(t, "settings/changed", map[string]any{"settings": map[string]any{"host": "unix:///tmp/other.sock", "logTailLines": 20000}})
	if h.e.settings.LogTailLines != 10000 || h.e.settings.Host != "unix:///tmp/other.sock" {
		t.Errorf("settings %+v", h.e.settings)
	}
	if len(h.fl.notified("view/refresh")) != 1 {
		t.Error("the panel wasn't refreshed")
	}
	h.resolve(t, "")
	if !slices.Equal(h.dials, []string{"", "unix:///tmp/other.sock"}) || h.eng.closed != 1 {
		t.Errorf("dials %q, closed %d: want one client per engine", h.dials, h.eng.closed)
	}
}

func TestInvalidHostIsExplained(t *testing.T) {
	h := newHarness(t)
	h.notify(t, "settings/changed", map[string]any{"settings": map[string]any{"host": "ftp://x", "logTailLines": 500}})
	v := h.resolve(t, "")
	if v.Kind != "message" || !strings.Contains(v.Text, "unix:///path") {
		t.Fatalf("view %+v", v)
	}
	if len(h.dials) != 0 {
		t.Error("dialed an invalid host")
	}
}

func TestSettingsCheck(t *testing.T) {
	for host, ok := range map[string]bool{
		"": true, "/var/run/docker.sock": true, "unix:///var/run/docker.sock": true, "tcp://h:2376": true,
		"https://h:2376": true, "http://h:2375": true, "ftp://h": false, "h:2376": false, "tcp://a b": false,
		strings.Repeat("x", 600): false,
	} {
		if err := (settings{Host: host}).check(); (err == nil) != ok {
			t.Errorf("check(%q) = %v", host, err)
		}
	}
	if s := (settings{LogTailLines: -5}).normalize(); s.LogTailLines != 1 {
		t.Errorf("tail %d", s.LogTailLines)
	}
}

func TestRefreshCommand(t *testing.T) {
	h := newHarness(t)
	if _, err := h.request(t, "command/execute", map[string]string{"id": cmdRefresh}); err != nil {
		t.Fatal(err)
	}
	if len(h.fl.notified("view/refresh")) != 1 {
		t.Error("no view/refresh")
	}
	h.eng.listErr = notRunning()
	if _, err := h.request(t, "command/execute", map[string]string{"id": cmdRefresh}); err == nil || !strings.Contains(err.Error(), "isn't running") {
		t.Errorf("err = %v", err)
	}
}

func TestPollsOnlyWhileVisible(t *testing.T) {
	h := newHarness(t)
	h.e.interval = 5 * time.Millisecond
	h.notify(t, "view/visible", map[string]any{"view": "flint.docker.elsewhere", "visible": true})
	h.notify(t, "view/visible", map[string]any{"view": panelID, "visible": true})
	h.notify(t, "view/visible", map[string]any{"view": panelID, "visible": true}) // one timer only
	deadline := time.Now().Add(5 * time.Second)
	for len(h.fl.notified("view/refresh")) < 3 {
		if time.Now().After(deadline) {
			t.Fatal("no refreshes while visible")
		}
		time.Sleep(time.Millisecond)
	}
	h.notify(t, "view/visible", map[string]any{"view": panelID, "visible": false})
	time.Sleep(10 * time.Millisecond) // a tick in flight may still land
	n := len(h.fl.notified("view/refresh"))
	time.Sleep(30 * time.Millisecond)
	if got := len(h.fl.notified("view/refresh")); got != n {
		t.Errorf("refreshed %d more times while hidden", got-n)
	}
}

func TestShutdownStopsPolling(t *testing.T) {
	h := newHarness(t)
	h.e.interval = time.Millisecond
	h.notify(t, "view/visible", map[string]any{"view": panelID, "visible": true})
	if _, err := h.request(t, "shutdown", map[string]any{}); err != nil {
		t.Fatal(err)
	}
	h.notify(t, "view/visible", map[string]any{"view": panelID, "visible": true}) // too late: stays stopped
	h.e.mu.Lock()
	polling := h.e.stopPoll != nil
	h.e.mu.Unlock()
	if polling {
		t.Error("polling after shutdown")
	}
}
