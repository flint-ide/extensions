// Command stats is an example Flint extension, "Project Stats": it counts
// the files of the open (trusted) projects by type and shows them in a
// panel, a status bar item and a tool for agents, with settings of its own.
// It uses only the standard library and talks to Flint over stdin and
// stdout (see the extension guide on the docs site).
//
// Build it next to its manifest and install the folder from Flint
// (Install Extension from Folder…):
//
//	go build -o stats .
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	panelID  = "example.stats.files"
	statusID = "example.stats.count"
)

type ext struct {
	c *conn

	mu       sync.Mutex
	roots    []string
	settings settings
	counted  *stats // nil until the files are counted
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("stats: ") // log goes to stderr, Flint's log of the extension
	e := &ext{c: newConn(os.Stdout)}
	if err := e.c.serve(os.Stdin, e.handle, e.notified); err != nil {
		log.Fatal(err)
	}
	// Flint closed stdin (after "shutdown"): done.
}

// handle answers Flint's requests.
func (e *ext) handle(method string, params json.RawMessage) (any, error) {
	switch method {
	case "initialize":
		var p struct {
			Roots    []struct{ Path string } `json:"roots"`
			Settings settings                `json:"settings"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		e.mu.Lock()
		e.settings = p.Settings
		for _, r := range p.Roots {
			e.roots = append(e.roots, r.Path)
		}
		e.mu.Unlock()
		log.Printf("started for %d folder(s)", len(p.Roots))
		return map[string]any{"contract": 1, "capabilities": map[string]any{}}, nil
	case "shutdown":
		return nil, nil
	case "view/resolve":
		var p struct{ Parent string }
		_ = json.Unmarshal(params, &p)
		return e.view(p.Parent), nil
	case "view/action":
		var p struct{ Item, Action string }
		_ = json.Unmarshal(params, &p)
		if path, ok := strings.CutPrefix(p.Item, "file:"); ok && p.Action == "open" {
			return nil, e.c.call("window/openFile", map[string]any{"path": path}, nil)
		}
		return nil, nil
	case "command/execute":
		var p struct {
			ID   string
			Args struct{ Path string }
		}
		_ = json.Unmarshal(params, &p)
		return nil, e.command(p.ID, p.Args.Path)
	case "agentTool/call":
		var p struct {
			Name      string
			Root      string
			Arguments struct{ Top int }
		}
		_ = json.Unmarshal(params, &p)
		return map[string]string{"text": e.summary(p.Root, p.Arguments.Top)}, nil
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
			e.settings, e.counted = p.Settings, nil
			e.mu.Unlock()
			e.changed()
		}
	case "workspace/rootsChanged":
		var p struct{ Roots []struct{ Path string } }
		if json.Unmarshal(params, &p) == nil {
			e.mu.Lock()
			e.roots, e.counted = nil, nil
			for _, r := range p.Roots {
				e.roots = append(e.roots, r.Path)
			}
			e.mu.Unlock()
			e.changed()
		}
	}
}

func (e *ext) command(id, path string) error {
	switch id {
	case "example.stats.refresh":
		e.mu.Lock()
		e.counted = nil
		e.mu.Unlock()
		s := e.changed()
		e.c.notify("window/showMessage", map[string]string{"type": "info", "message": fmt.Sprintf("Counted %s files (%s).", thousands(s.Files), size(s.Bytes))})
	case "example.stats.open":
		if path == "" { // from the palette: the largest file of the most common type
			s := e.stats()
			if len(s.Types) == 0 || len(s.Types[0].Largest) == 0 {
				return &rpcError{Code: -32602, Message: "no files counted yet"}
			}
			path = s.Types[0].Largest[0].Path
		}
		return e.c.call("window/openFile", map[string]any{"path": path}, nil)
	}
	return nil
}

// changed counts again, then refreshes the panel and the status item.
func (e *ext) changed() stats {
	s := e.stats()
	e.c.notify("view/refresh", map[string]string{"view": panelID})
	e.c.notify("status/update", map[string]string{
		"id":      statusID,
		"text":    thousands(s.Files) + " files",
		"tooltip": fmt.Sprintf("Project Stats: %s files, %s. Click to count again.", thousands(s.Files), size(s.Bytes)),
		"command": "example.stats.refresh",
	})
	return s
}

// stats counts the files of the workspace, once until something changes.
func (e *ext) stats() stats {
	s, _ := e.count()
	return s
}

// count is stats, and whether it just counted.
func (e *ext) count() (stats, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.counted != nil {
		return *e.counted, false
	}
	s := scan(e.roots, e.settings)
	e.counted = &s
	log.Printf("counted %d files in %d folder(s)", s.Files, len(e.roots))
	return s, true
}

// view is the panel's content: file types, and under each its largest files.
func (e *ext) view(parent string) map[string]any {
	e.mu.Lock()
	none := len(e.roots) == 0
	e.mu.Unlock()
	if none {
		return map[string]any{"kind": "message", "text": "Open a trusted folder to count its files."}
	}
	s, fresh := e.count()
	if fresh {
		e.changed() // the status item follows the first count
	}
	items := []map[string]any{}
	if name, ok := strings.CutPrefix(parent, "type:"); ok {
		for _, t := range s.Types {
			if t.Name != name {
				continue
			}
			for _, f := range t.Largest {
				items = append(items, map[string]any{
					"id": "file:" + f.Path, "label": filepath.Base(f.Path), "description": size(f.Size),
					"tooltip": f.Path, "icon": "files",
					"command": map[string]any{"id": "example.stats.open", "args": map[string]string{"path": f.Path}},
					"actions": []map[string]string{{"id": "open", "label": "Open"}},
				})
			}
		}
		return map[string]any{"kind": "tree", "items": items}
	}
	for _, t := range s.Types {
		items = append(items, map[string]any{
			"id": "type:" + t.Name, "label": t.Name, "hasChildren": len(t.Largest) > 0,
			"description": fmt.Sprintf("%s files · %s", thousands(t.Files), size(t.Bytes)),
		})
	}
	return map[string]any{"kind": "tree", "items": items}
}

// summary is the agent tool's answer for one project folder.
func (e *ext) summary(root string, top int) string {
	if top <= 0 {
		top = 15
	}
	e.mu.Lock()
	cfg := e.settings
	e.mu.Unlock()
	s := scan([]string{root}, cfg)
	var b strings.Builder
	fmt.Fprintf(&b, "%s files, %s in %s", thousands(s.Files), size(s.Bytes), root)
	if s.Truncated {
		fmt.Fprintf(&b, " (stopped counting at %s files)", thousands(maxFiles))
	}
	b.WriteString("\n\ntype\tfiles\tsize\tlargest\n")
	for i, t := range s.Types {
		if i == top {
			fmt.Fprintf(&b, "… and %d more types\n", len(s.Types)-top)
			break
		}
		largest := ""
		if len(t.Largest) > 0 {
			rel, err := filepath.Rel(root, t.Largest[0].Path)
			if err != nil {
				rel = t.Largest[0].Path
			}
			largest = rel
		}
		fmt.Fprintf(&b, "%s\t%d\t%s\t%s\n", t.Name, t.Files, size(t.Bytes), largest)
	}
	return b.String()
}

// thousands writes 1234567 as 1,234,567.
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
