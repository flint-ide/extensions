package main

import (
	"bufio"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// TestOverStdio drives the extension as Flint does: a webview/request for
// logs arrives on stdin, the extension calls window/openTerminal, the test
// answers it like Flint, and the page's request is answered.
func TestOverStdio(t *testing.T) {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	c := newConn(outW)
	e, _ := testExt(t, &fakeCluster{})
	e.c = c
	go func() { _ = c.serve(inR, e.handle, e.notified) }()
	defer inW.Close()

	lines := make(chan message, 4)
	go func() {
		sc := bufio.NewScanner(outR)
		for sc.Scan() {
			var m message
			if json.Unmarshal(sc.Bytes(), &m) == nil {
				lines <- m
			}
		}
	}()
	next := func() message {
		select {
		case m := <-lines:
			return m
		case <-time.After(5 * time.Second):
			t.Fatal("no message from the extension")
		}
		return message{}
	}
	write := func(v any) {
		data, _ := json.Marshal(v)
		if _, err := inW.Write(append(data, '\n')); err != nil {
			t.Fatal(err)
		}
	}

	write(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "webview/request", "params": map[string]any{
		"view": panelID, "method": "logs", "params": map[string]any{"context": "dev", "namespace": "shop", "target": "pod/web-1"},
	}})
	call := next()
	if call.Method != "window/openTerminal" || !strings.Contains(string(call.Params), `"logs: web-1"`) {
		t.Fatalf("got %+v", call)
	}
	write(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(call.ID), "result": nil})
	answer := next()
	if string(answer.ID) != "1" || answer.Error != nil {
		t.Fatalf("answer %+v", answer)
	}

	// An error from Flint (no terminal permission, say) reaches the page.
	write(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "webview/request", "params": map[string]any{
		"view": panelID, "method": "logs", "params": map[string]any{"context": "dev", "namespace": "shop", "target": "pod/web-1"},
	}})
	call = next()
	write(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(call.ID), "error": map[string]any{"code": -32001, "message": "permission denied"}})
	answer = next()
	if answer.Error == nil || answer.Error.Code != -32001 {
		t.Fatalf("answer %+v", answer)
	}
}
