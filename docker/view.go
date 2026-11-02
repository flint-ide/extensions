package main

import (
	"cmp"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/flint-ide/extensions/docker/internal/engine"
)

// Limits on what the panel sends (Flint cuts the rest: 5,000 items, 2,000
// characters per text).
const (
	maxItems   = 5000
	maxLabel   = 200
	maxText    = 2000
	maxPortsUI = 3 // "Open localhost:…" actions per container
	// snapTTL lets the nodes Flint resolves together after one refresh (the
	// root, then each expanded project) share one listing.
	snapTTL = time.Second
)

// Item ids: what a row is, then its key.
const (
	prefixProject   = "project:"   // + the project's folder
	prefixContainer = "container:" // + the container's id
	itemWarning     = "warning"
	itemNoCLI       = "info:cli"
)

// noCLIHint is shown when the docker CLI is missing.
const noCLIHint = "Compose up, down and pull need the docker CLI"

// snapshot is one listing of the engine.
type snapshot struct {
	at         time.Time
	containers []engine.Container
	projects   []engine.ComposeProject
	composeOK  bool
	endpoint   engine.Endpoint
	err        error
}

// overview lists the engine, once per snapTTL; the status item follows it.
func (e *ext) overview() snapshot {
	e.mu.Lock()
	if s := e.snap; s != nil && e.deps.now().Sub(s.at) < snapTTL {
		e.mu.Unlock()
		return *s
	}
	roots := slices.Clone(e.roots)
	e.mu.Unlock()

	s := snapshot{at: e.deps.now()}
	dk, ep, err := e.docker()
	if err == nil {
		s.endpoint = ep
		s.containers, err = dk.List(e.ctx)
	}
	if err != nil {
		s.err = err
		s.containers = nil
	} else {
		s.projects = projects(s.containers, e.deps.findCompose(roots))
		s.composeOK = e.deps.composeAvailable()
	}
	e.mu.Lock()
	e.snap = &s
	e.mu.Unlock()
	e.updateStatus(s)
	return s
}

// projects merges running Compose projects (from container labels) with the
// compose files found in the workspace, one entry per folder.
func projects(cs []engine.Container, found []engine.ComposeProject) []engine.ComposeProject {
	out := []engine.ComposeProject{}
	byDir := map[string]bool{}
	for _, c := range cs {
		if c.Project == "" || c.WorkingDir == "" || byDir[c.WorkingDir] {
			continue
		}
		byDir[c.WorkingDir] = true
		out = append(out, engine.ComposeProject{Name: c.Project, Dir: c.WorkingDir, Files: splitFiles(c.ConfigFiles)})
	}
	for _, p := range found {
		if !byDir[p.Dir] {
			byDir[p.Dir] = true
			out = append(out, p)
		}
	}
	return out
}

func splitFiles(s string) []string {
	var out []string
	for _, f := range strings.Split(s, ",") {
		if f = strings.TrimSpace(f); f != "" {
			out = append(out, f)
		}
	}
	return out
}

// tree groups the containers: by project folder, and the ones outside any
// project (running first, then by name).
func tree(s snapshot) (map[string][]engine.Container, []engine.Container) {
	dirs := map[string]bool{}
	for _, p := range s.projects {
		dirs[p.Dir] = true
	}
	byDir := map[string][]engine.Container{}
	var others []engine.Container
	for _, c := range s.containers {
		if c.Project != "" && dirs[c.WorkingDir] {
			byDir[c.WorkingDir] = append(byDir[c.WorkingDir], c)
			continue
		}
		others = append(others, c)
	}
	for _, cs := range byDir {
		slices.SortFunc(cs, func(a, b engine.Container) int {
			return cmp.Or(cmp.Compare(a.Service, b.Service), cmp.Compare(a.Name, b.Name))
		})
	}
	slices.SortFunc(others, func(a, b engine.Container) int {
		ra, rb := a.State == "running", b.State == "running"
		if ra != rb {
			if ra {
				return -1
			}
			return 1
		}
		return cmp.Compare(a.Name, b.Name)
	})
	return byDir, others
}

// view is the panel's content for parent ("" for the top level).
func (e *ext) view(parent string) map[string]any {
	s := e.overview()
	if s.err != nil {
		return unreachable(s.err)
	}
	byDir, others := tree(s)
	if dir, ok := strings.CutPrefix(parent, prefixProject); ok {
		items := []map[string]any{}
		for _, c := range byDir[dir] {
			items = append(items, e.containerItem(c, cmp.Or(c.Service, c.Name)))
		}
		return treeView(items)
	}
	if parent != "" {
		return treeView(nil) // a node that is gone
	}
	if len(s.containers) == 0 && len(s.projects) == 0 {
		return map[string]any{"kind": "message", "text": "No containers or compose files.", "actions": []action{{"refresh", "Refresh"}}}
	}
	items := []map[string]any{}
	if w := s.endpoint.Warning; w != "" {
		items = append(items, map[string]any{"id": itemWarning, "label": "Not protected: plain tcp:// without TLS", "tooltip": cut(w, maxText)})
	}
	if !s.composeOK && len(s.projects) > 0 {
		items = append(items, map[string]any{"id": itemNoCLI, "label": noCLIHint, "tooltip": engine.ErrComposeNeedsCLI.Error()})
	}
	for _, p := range s.projects {
		items = append(items, e.projectItem(p, byDir[p.Dir], s.composeOK))
	}
	for _, c := range others {
		items = append(items, e.containerItem(c, c.Name))
	}
	return treeView(items)
}

func treeView(items []map[string]any) map[string]any {
	if items == nil {
		items = []map[string]any{}
	}
	if len(items) > maxItems {
		log.Printf("showing %d of %d items", maxItems, len(items))
		items = items[:maxItems]
	}
	return map[string]any{"kind": "tree", "items": items}
}

// unreachable is the panel when the engine can't be listed.
func unreachable(err error) map[string]any {
	text := "Can't reach Docker: " + err.Error() + "."
	if errors.Is(err, engine.ErrNotRunning) {
		text = err.Error() + ". Or set the engine address in Settings ▸ Docker."
	}
	return map[string]any{"kind": "message", "text": cut(text, maxText), "actions": []action{{"refresh", "Retry"}}}
}

type action struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

func (e *ext) projectItem(p engine.ComposeProject, cs []engine.Container, composeOK bool) map[string]any {
	running := 0
	for _, c := range cs {
		if c.State == "running" {
			running++
		}
	}
	id := prefixProject + p.Dir
	desc := "not started"
	if len(cs) > 0 {
		desc = fmt.Sprintf("%d/%d running", running, len(cs))
	}
	tip := []string{"Compose project " + projectName(p), p.Dir}
	if len(p.Files) > 0 {
		tip = append(tip, "Files: "+strings.Join(p.Files, ", "))
	}
	if !composeOK {
		tip = append(tip, noCLIHint+".")
	}
	var acts []action
	if composeOK {
		acts = append(acts, action{"up", "Up"})
	}
	if p.Name != "" && len(cs) > 0 {
		acts = append(acts, action{"restart", "Restart"})
		if running > 0 {
			acts = append(acts, action{"stop", "Stop"})
		}
		if composeOK {
			acts = append(acts, action{"down", "Down…"})
		}
	}
	if composeOK {
		acts = append(acts, action{"pull", "Pull Images"})
		if p.Name != "" && len(cs) > 0 {
			acts = append(acts, action{"logs", "Logs"})
		}
	}
	return map[string]any{
		"id": cut(id, 256), "label": cut(projectName(p), maxLabel), "description": e.busyOr(id, desc),
		"tooltip": cut(strings.Join(tip, " · "), maxText), "icon": "ship", "hasChildren": len(cs) > 0, "actions": acts,
	}
}

// projectName is the project's name, else its folder's.
func projectName(p engine.ComposeProject) string {
	if p.Name != "" {
		return p.Name
	}
	return filepath.Base(p.Dir)
}

func (e *ext) containerItem(c engine.Container, label string) map[string]any {
	id := prefixContainer + c.ID
	running := c.State == "running"
	parts := []string{cmp.Or(c.Status, c.State), c.Image}
	var ports []string
	for _, p := range c.Ports {
		ports = append(ports, ":"+strconv.Itoa(p.HostPort))
	}
	if len(ports) > 0 {
		parts = append(parts, strings.Join(ports, " "))
	}
	tip := []string{c.Name, "Image " + c.Image, cmp.Or(c.Status, c.State), "Id " + shortID(c.ID)}
	if c.Project != "" {
		tip = append(tip, "Compose "+c.Project+" / "+c.Service)
	}
	for _, p := range c.Ports {
		tip = append(tip, fmt.Sprintf("localhost:%d → %s", p.HostPort, p.Container))
	}
	acts := []action{{"logs", "Logs"}}
	if running {
		acts = append(acts, action{"shell", "Open Shell"}, action{"stop", "Stop"}, action{"restart", "Restart"})
	} else {
		acts = append(acts, action{"start", "Start"}, action{"rm", "Remove…"})
	}
	for i, p := range c.Ports {
		if i == maxPortsUI {
			break
		}
		acts = append(acts, action{"port:" + strconv.Itoa(p.HostPort), "Open localhost:" + strconv.Itoa(p.HostPort)})
	}
	return map[string]any{
		"id": cut(id, 256), "label": cut(cmp.Or(label, shortID(c.ID)), maxLabel),
		"description": e.busyOr(id, cut(strings.Join(parts, " · "), maxText)),
		"tooltip":     cut(strings.Join(tip, " · "), maxText), "actions": acts,
	}
}

// busyOr is "Stopping…" while an action runs on the item, else desc.
func (e *ext) busyOr(id, desc string) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if b := e.busy[id]; b != "" {
		return b
	}
	return desc
}

func shortID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// cut shortens s to limit runes.
func cut(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	return string([]rune(s)[:limit-1]) + "…"
}

// updateStatus shows the number of running containers in the status bar
// ("" hides the item while the engine is unreachable).
func (e *ext) updateStatus(s snapshot) {
	text, tip := "", ""
	if s.err == nil {
		running := 0
		for _, c := range s.containers {
			if c.State == "running" {
				running++
			}
		}
		text = fmt.Sprintf("Docker: %d running", running)
		tip = fmt.Sprintf("%d of %d containers running (%s). Click to refresh.", running, len(s.containers), s.endpoint.Host)
	}
	e.mu.Lock()
	same := e.status == text
	e.status = text
	e.mu.Unlock()
	if same {
		return
	}
	params := map[string]string{"id": statusID, "text": text}
	if text != "" {
		params["tooltip"], params["command"] = tip, cmdRefresh
	}
	e.fl.notify("status/update", params)
}
