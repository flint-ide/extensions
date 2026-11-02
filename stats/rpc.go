package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// A minimal JSON-RPC 2.0 connection over stdin and stdout, one message per
// line: everything an extension needs to talk to Flint. stdout carries only
// protocol messages; logs go to stderr, which Flint keeps in the extension's
// log (Settings ▸ Extensions ▸ Show Log).

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return fmt.Sprintf("%s (%d)", e.Message, e.Code) }

// handler answers a request from Flint: a result, or an error.
type handler func(method string, params json.RawMessage) (any, error)

type conn struct {
	out    io.Writer
	outMu  sync.Mutex
	nextID atomic.Int64

	mu      sync.Mutex
	waiting map[int64]chan message
}

func newConn(out io.Writer) *conn {
	return &conn{out: out, waiting: map[int64]chan message{}}
}

func (c *conn) send(v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	c.outMu.Lock()
	defer c.outMu.Unlock()
	_, _ = c.out.Write(append(data, '\n'))
}

// notify sends a notification to Flint.
func (c *conn) notify(method string, params any) {
	c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

// call sends a request to Flint and waits for its answer.
func (c *conn) call(method string, params, result any) error {
	id := c.nextID.Add(1)
	ch := make(chan message, 1)
	c.mu.Lock()
	c.waiting[id] = ch
	c.mu.Unlock()
	c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	m := <-ch
	if m.Error != nil {
		return m.Error
	}
	if result != nil {
		return json.Unmarshal(m.Result, result)
	}
	return nil
}

// serve reads messages until stdin closes: requests are answered by h,
// each in its own goroutine (an answer may wait for a call to Flint);
// notifications go to onNotify in order.
func (c *conn) serve(in io.Reader, h handler, onNotify func(method string, params json.RawMessage)) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 64<<10), 4<<20) // Flint's limit is 4 MiB per message
	for sc.Scan() {
		var m message
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			continue
		}
		switch {
		case m.Method == "": // an answer to one of our calls
			var id int64
			if json.Unmarshal(m.ID, &id) == nil {
				c.mu.Lock()
				ch := c.waiting[id]
				delete(c.waiting, id)
				c.mu.Unlock()
				if ch != nil {
					ch <- m
				}
			}
		case m.ID == nil:
			onNotify(m.Method, m.Params)
		default:
			go func() {
				result, err := h(m.Method, m.Params)
				if err != nil {
					code := -32603
					if e, ok := err.(*rpcError); ok {
						code = e.Code
					}
					c.send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "error": rpcError{Code: code, Message: err.Error()}})
					return
				}
				c.send(map[string]any{"jsonrpc": "2.0", "id": m.ID, "result": result})
			}()
		}
	}
	return sc.Err()
}
