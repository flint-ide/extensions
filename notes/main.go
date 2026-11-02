// Command notes is an example Flint extension with webviews: a panel listing
// notes and a tab per note, both drawn by the extension's own HTML
// (ui/list.html, ui/note.html) in Flint's sandboxed frames. The pages talk to
// this process with flint.request (webview/request here), and it talks back
// with webview/post and opens tabs with window/openTab. Notes live in memory.
//
// Build it next to its manifest and install the folder from Flint
// (Install Extension from Folder…):
//
//	go build -o notes .
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"slices"
	"strconv"
	"sync"
)

const (
	listView = "example.notes.list"
	noteView = "example.notes.note"
)

type note struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Text  string `json:"text"`
}

type ext struct {
	c *conn

	mu    sync.Mutex
	notes []note
	next  int
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("notes: ")
	e := &ext{c: newConn(os.Stdout), notes: []note{{ID: "1", Title: "Welcome", Text: "Notes are drawn by the extension's own HTML."}}, next: 2}
	if err := e.c.serve(os.Stdin, e.handle, func(string, json.RawMessage) {}); err != nil {
		log.Fatal(err)
	}
}

func (e *ext) handle(method string, params json.RawMessage) (any, error) {
	switch method {
	case "initialize":
		return map[string]any{"capabilities": map[string]any{}}, nil
	case "shutdown":
		return nil, nil
	case "command/execute":
		var p struct{ ID string }
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if p.ID != "example.notes.new" {
			return nil, &rpcError{Code: -32602, Message: "unknown command " + p.ID}
		}
		return nil, e.open(e.add())
	case "webview/request":
		var p struct {
			View   string          `json:"view"`
			Key    string          `json:"key"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		return e.request(p.View, p.Key, p.Method, p.Params)
	}
	return nil, &rpcError{Code: -32601, Message: "method not found: " + method}
}

// request answers a page's flint.request(method, params).
func (e *ext) request(view, key, method string, params json.RawMessage) (any, error) {
	switch method {
	case "list":
		e.mu.Lock()
		defer e.mu.Unlock()
		return slices.Clone(e.notes), nil
	case "new":
		return nil, e.open(e.add())
	case "open":
		var id string
		if err := json.Unmarshal(params, &id); err != nil {
			return nil, err
		}
		n, ok := e.find(id)
		if !ok {
			return nil, fmt.Errorf("no note %q", id)
		}
		return nil, e.open(n)
	case "get":
		n, ok := e.find(key)
		if !ok {
			return nil, fmt.Errorf("no note %q", key)
		}
		return n, nil
	case "save":
		var in struct{ Title, Text string }
		if err := json.Unmarshal(params, &in); err != nil {
			return nil, err
		}
		e.mu.Lock()
		i := slices.IndexFunc(e.notes, func(n note) bool { return n.ID == key })
		if i >= 0 {
			e.notes[i].Title, e.notes[i].Text = in.Title, in.Text
		}
		e.mu.Unlock()
		if i < 0 {
			return nil, fmt.Errorf("no note %q", key)
		}
		// The list panel (every frame of it) shows the new title.
		e.c.notify("webview/post", map[string]any{"view": listView, "message": map[string]any{"changed": key}})
		log.Printf("saved note %s (from %s)", key, view)
		return true, nil
	}
	return nil, &rpcError{Code: -32601, Message: "unknown request " + method}
}

func (e *ext) add() note {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := note{ID: strconv.Itoa(e.next), Title: "Note " + strconv.Itoa(e.next)}
	e.next++
	e.notes = append(e.notes, n)
	go e.c.notify("webview/post", map[string]any{"view": listView, "message": map[string]any{"changed": n.ID}})
	return n
}

func (e *ext) find(id string) (note, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	i := slices.IndexFunc(e.notes, func(n note) bool { return n.ID == id })
	if i < 0 {
		return note{}, false
	}
	return e.notes[i], true
}

// open shows the note's tab (window/openTab: Flint opens or focuses it).
func (e *ext) open(n note) error {
	return e.c.call("window/openTab", map[string]any{"view": noteView, "key": n.ID, "title": n.Title}, nil)
}
