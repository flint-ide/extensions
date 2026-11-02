package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

func TestListGetAndSave(t *testing.T) {
	var out bytes.Buffer
	e := &ext{c: newConn(&out), notes: []note{{ID: "1", Title: "Welcome"}}, next: 2}
	got, err := e.request(noteView, "", "list", nil)
	if err != nil || len(got.([]note)) != 1 {
		t.Fatalf("list = %v, %v", got, err)
	}
	if _, err := e.request(noteView, "1", "save", json.RawMessage(`{"Title":"Plan","Text":"x"}`)); err != nil {
		t.Fatal(err)
	}
	n, err := e.request(noteView, "1", "get", nil)
	if err != nil || n.(note).Title != "Plan" || n.(note).Text != "x" {
		t.Errorf("get = %+v, %v", n, err)
	}
	// Saving tells the list panel.
	if !strings.Contains(out.String(), `"method":"webview/post"`) || !strings.Contains(out.String(), `"view":"example.notes.list"`) {
		t.Errorf("sent %s", out.String())
	}
	if _, err := e.request(noteView, "9", "get", nil); err == nil {
		t.Error("got a note that doesn't exist")
	}
	if _, err := e.request(noteView, "1", "nope", nil); err == nil {
		t.Error("an unknown request was answered")
	}
}

func TestNewNoteOpensItsTab(t *testing.T) {
	r, w := io.Pipe()
	defer r.Close()
	e := &ext{c: newConn(w), notes: []note{}, next: 2}
	lines := make(chan []byte)
	go func() {
		sc := bufio.NewScanner(r)
		for sc.Scan() {
			lines <- append([]byte(nil), sc.Bytes()...)
		}
	}()
	done := make(chan error, 1)
	go func() { done <- e.open(e.add()) }()

	// Answer the window/openTab call like Flint does (a webview/post may come first).
	var call message
	for call.Method != "window/openTab" {
		select {
		case line := <-lines:
			if err := json.Unmarshal(line, &call); err != nil {
				t.Fatal(err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("no window/openTab")
		}
	}
	if !strings.Contains(string(call.Params), `"key":"2"`) || !strings.Contains(string(call.Params), `"view":"example.notes.note"`) {
		t.Errorf("openTab params = %s", call.Params)
	}
	var id int64
	if err := json.Unmarshal(call.ID, &id); err != nil {
		t.Fatal(err)
	}
	e.c.mu.Lock()
	ch := e.c.waiting[id]
	e.c.mu.Unlock()
	ch <- message{Result: json.RawMessage("null")}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	// Drain the list panel's notification so the writer isn't left blocked.
	go func() {
		for range lines {
		}
	}()
}
