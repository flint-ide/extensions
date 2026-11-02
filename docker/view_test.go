package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/flint-ide/extensions/docker/internal/engine"
)

func TestRootGroupsComposeProjectsAndStandaloneContainers(t *testing.T) {
	h := newHarness(t)
	h.found = []engine.ComposeProject{
		{Dir: "/p/shop", Files: []string{"/p/shop/compose.yaml"}}, // the running project's folder: listed once
		{Dir: "/p/blog", Files: []string{"/p/blog/compose.yaml"}},
	}
	v := h.resolve(t, "")
	if v.Kind != "tree" {
		t.Fatalf("kind %q", v.Kind)
	}
	want := []string{"project:/p/shop", "project:/p/blog", "container:api1", "container:lonely1"}
	if !slices.Equal(v.ids(), want) {
		t.Fatalf("ids %q, want %q (projects, then running containers first)", v.ids(), want)
	}
	shop, blog, api, lonely := v.Items[0], v.Items[1], v.Items[2], v.Items[3]
	if shop.Label != "shop" || shop.Description != "1/2 running" || !shop.HasChildren || shop.Icon != "ship" {
		t.Errorf("shop %+v", shop)
	}
	if !strings.Contains(shop.Tooltip, "/p/shop/compose.override.yaml") {
		t.Errorf("shop tooltip %q lacks its files", shop.Tooltip)
	}
	if got := actionIDs(shop.Actions); !slices.Equal(got, []string{"up", "restart", "stop", "down", "pull", "logs"}) {
		t.Errorf("shop actions %q", got)
	}
	if blog.Label != "blog" || blog.Description != "not started" || blog.HasChildren {
		t.Errorf("blog %+v", blog)
	}
	if got := actionIDs(blog.Actions); !slices.Equal(got, []string{"up", "pull"}) {
		t.Errorf("blog actions %q: a project that isn't running can only go up or pull", got)
	}
	if api.Label != "api" || api.Description != "Up 3 minutes · node:22 · :8080" || !strings.Contains(api.Tooltip, "localhost:8080 → 80/tcp") {
		t.Errorf("api %+v", api)
	}
	if got := actionIDs(api.Actions); !slices.Equal(got, []string{"logs", "shell", "stop", "restart", "port:8080"}) {
		t.Errorf("running container actions %q", got)
	}
	if got := actionIDs(lonely.Actions); !slices.Equal(got, []string{"logs", "start", "rm"}) {
		t.Errorf("stopped container actions %q", got)
	}
	if lonely.Description != "Exited (1) 1 hour ago · alpine" {
		t.Errorf("lonely %q", lonely.Description)
	}

	// The project's containers, by service.
	c := h.resolve(t, "project:/p/shop")
	if !slices.Equal(c.ids(), []string{"container:db1", "container:web1"}) || c.Items[0].Label != "db" || c.Items[1].Label != "web" {
		t.Errorf("children %+v", c.Items)
	}
	if got := h.resolve(t, "container:api1"); got.Kind != "tree" || len(got.Items) != 0 {
		t.Errorf("a container has no children: %+v", got)
	}
}

func TestStatusItemCountsRunningContainers(t *testing.T) {
	h := newHarness(t)
	h.resolve(t, "")
	h.resolve(t, "") // unchanged: not sent again
	st := h.fl.notified("status/update")
	if len(st) != 1 || st[0].Params["text"] != "Docker: 2 running" || st[0].Params["command"] != cmdRefresh || st[0].Params["id"] != statusID {
		t.Fatalf("status %+v", st)
	}
	h.eng.listErr = notRunning()
	h.resolve(t, "")
	st = h.fl.notified("status/update")
	if len(st) != 2 || st[1].Params["text"] != "" {
		t.Errorf("status while unreachable %+v: want it hidden", st)
	}
}

func TestUnreachableEngineIsAMessage(t *testing.T) {
	h := newHarness(t)
	h.eng.listErr = notRunning()
	v := h.resolve(t, "")
	if v.Kind != "message" || !strings.Contains(v.Text, "Docker isn't running") || !strings.Contains(v.Text, "Settings ▸ Docker") {
		t.Fatalf("view %+v", v)
	}
	if !slices.Equal(actionIDs(v.Actions), []string{"refresh"}) {
		t.Errorf("actions %+v", v.Actions)
	}
	h.eng.listErr = errBoom
	if v := h.resolve(t, ""); v.Kind != "message" || !strings.Contains(v.Text, "Can't reach Docker: boom") {
		t.Errorf("other error %+v", v)
	}
	// Retry lists again.
	h.eng.listErr = nil
	if err := h.act(t, "", "refresh"); err != nil {
		t.Fatal(err)
	}
	if v := h.resolve(t, ""); v.Kind != "tree" {
		t.Errorf("after retry %+v", v)
	}
}

func TestNothingToShow(t *testing.T) {
	h := newHarness(t)
	h.eng.containers = nil
	if v := h.resolve(t, ""); v.Kind != "message" || !strings.Contains(v.Text, "No containers") {
		t.Errorf("view %+v", v)
	}
}

func TestWithoutTheCLIComposeActionsAreHidden(t *testing.T) {
	h := newHarness(t)
	h.noCLI = true
	v := h.resolve(t, "")
	if v.ids()[0] != itemNoCLI || !strings.Contains(v.Items[0].Tooltip, "docker CLI") {
		t.Fatalf("ids %q", v.ids())
	}
	if got := actionIDs(v.Items[1].Actions); !slices.Equal(got, []string{"restart", "stop"}) {
		t.Errorf("project actions without the CLI %q", got)
	}
}

func TestPlainTCPIsFlagged(t *testing.T) {
	h := newHarness(t)
	h.notify(t, "settings/changed", map[string]any{"settings": map[string]any{"host": "tcp://10.0.0.5:2375", "logTailLines": 500}})
	v := h.resolve(t, "")
	if v.ids()[0] != itemWarning || !strings.Contains(v.Items[0].Tooltip, "without TLS") {
		t.Fatalf("items %+v", v.Items)
	}
}

func TestListingIsSharedWithinARefresh(t *testing.T) {
	h := newHarness(t)
	still := time.Unix(1_800_000_000, 0)
	h.e.deps.now = func() time.Time { return still } // the clock stands still
	h.resolve(t, "")
	h.resolve(t, "project:/p/shop")
	if h.eng.lists != 1 {
		t.Errorf("listed %d times for one refresh", h.eng.lists)
	}
	h.e.invalidate()
	h.resolve(t, "")
	if h.eng.lists != 2 {
		t.Errorf("listed %d times after invalidate", h.eng.lists)
	}
}

func TestBoundsItems(t *testing.T) {
	h := newHarness(t)
	h.eng.containers = nil
	for i := range maxItems + 10 {
		h.eng.containers = append(h.eng.containers, engine.Container{ID: fmt.Sprintf("c%05d", i), Name: strings.Repeat("n", 300) + fmt.Sprint(i), State: "exited"})
	}
	v := h.resolve(t, "")
	if len(v.Items) != maxItems {
		t.Errorf("%d items", len(v.Items))
	}
	if n := len([]rune(v.Items[0].Label)); n > maxLabel {
		t.Errorf("label of %d characters", n)
	}
}
