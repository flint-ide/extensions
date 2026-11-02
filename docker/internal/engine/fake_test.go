package engine

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fakeEngine is a small Docker Engine API on a temp unix socket. It answers
// like a daemon that speaks API 1.52 and rejects any version prefix other
// than the one Flint should have negotiated (1.47).
type fakeEngine struct {
	t      *testing.T
	sock   string
	events chan string // event actions to stream to /events watchers

	mu      sync.Mutex
	calls   []string // "METHOD path?query"
	pingHdr string
	tty     bool
	running bool

	shellIn  strings.Builder // bytes the hijacked exec received
	resizes  []string
	hijacked chan struct{}
}

const listFixture = `[
 {"Id":"abc123def456","Names":["/shop-db-1"],"Image":"postgres:16","ImageID":"sha256:1","Command":"postgres","Created":1767000000,
  "Ports":[{"IP":"0.0.0.0","PrivatePort":5432,"PublicPort":5432,"Type":"tcp"},{"IP":"::","PrivatePort":5432,"PublicPort":5432,"Type":"tcp"},{"PrivatePort":9187,"Type":"tcp"}],
  "Labels":{"com.docker.compose.project":"shop","com.docker.compose.service":"db","com.docker.compose.project.config_files":"/p/shop/compose.yaml,/p/shop/compose.override.yaml","com.docker.compose.project.working_dir":"/p/shop"},
  "State":"running","Status":"Up 2 hours (healthy)"},
 {"Id":"def456abc123","Names":["/lonely"],"Image":"alpine","Created":1767000100,"Ports":[],"Labels":{},"State":"exited","Status":"Exited (0) 5 minutes ago"}
]`

func newFake(t *testing.T) *fakeEngine {
	t.Helper()
	dir, err := os.MkdirTemp("", "fd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	f := &fakeEngine{t: t, sock: filepath.Join(dir, "d.sock"), events: make(chan string, 16), pingHdr: "1.52", running: true, hijacked: make(chan struct{})}
	ln, err := net.Listen("unix", f.sock)
	if err != nil {
		t.Fatal(err)
	}
	srv := &httptest.Server{Listener: ln, Config: &http.Server{Handler: http.HandlerFunc(f.serve)}}
	srv.Start()
	t.Cleanup(srv.Close)
	return f
}

func (f *fakeEngine) client() *Docker {
	f.t.Helper()
	d, err := New(Options{Host: "unix://" + f.sock})
	if err != nil {
		f.t.Fatal(err)
	}
	f.t.Cleanup(d.Close)
	return d
}

func (f *fakeEngine) record(r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, r.Method+" "+r.URL.RequestURI())
}

func (f *fakeEngine) callList() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func jsonOut(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func frame(stream byte, s string) []byte {
	b := make([]byte, 8, 8+len(s))
	b[0] = stream
	binary.BigEndian.PutUint32(b[4:], uint32(len(s)))
	return append(b, s...)
}

func (f *fakeEngine) serve(w http.ResponseWriter, r *http.Request) {
	f.record(r)
	if r.URL.Path == "/_ping" {
		w.Header().Set("Api-Version", f.pingHdr)
		_, _ = io.WriteString(w, "OK")
		return
	}
	path, ok := strings.CutPrefix(r.URL.Path, "/v1.47")
	if !ok {
		jsonOut(w, 404, map[string]string{"message": "page not found"})
		return
	}
	switch {
	case r.Method == "GET" && path == "/containers/json":
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, listFixture)
	case r.Method == "GET" && strings.HasSuffix(path, "/json") && strings.HasPrefix(path, "/containers/"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "/containers/"), "/json")
		if id == "ghost" {
			jsonOut(w, 404, map[string]string{"message": "No such container: ghost"})
			return
		}
		jsonOut(w, 200, map[string]any{"Id": id + "full", "Name": "/" + id, "State": map[string]any{"Running": f.running}, "Config": map[string]any{"Tty": f.tty, "Image": "busybox"}})
	case r.Method == "POST" && (strings.HasSuffix(path, "/start") || strings.HasSuffix(path, "/stop") || strings.HasSuffix(path, "/restart")) && strings.HasPrefix(path, "/containers/"):
		if strings.Contains(path, "already") {
			w.WriteHeader(304)
			return
		}
		w.WriteHeader(204)
	case r.Method == "DELETE" && strings.HasPrefix(path, "/containers/"):
		if strings.Contains(path, "busy") {
			jsonOut(w, 409, map[string]string{"message": "cannot remove a running container"})
			return
		}
		w.WriteHeader(204)
	case r.Method == "GET" && strings.HasSuffix(path, "/logs"):
		f.logs(w)
	case r.Method == "GET" && path == "/events":
		f.streamEvents(w, r)
	case r.Method == "POST" && strings.HasSuffix(path, "/exec") && strings.HasPrefix(path, "/containers/"):
		var req struct {
			Tty bool
			Cmd []string
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !req.Tty || len(req.Cmd) == 0 {
			jsonOut(w, 400, map[string]string{"message": "bad exec"})
			return
		}
		jsonOut(w, 201, map[string]string{"Id": "exec1"})
	case r.Method == "POST" && path == "/exec/exec1/start":
		_, _ = io.Copy(io.Discard, r.Body)
		f.hijack(w)
	case r.Method == "POST" && path == "/exec/exec1/resize":
		f.mu.Lock()
		f.resizes = append(f.resizes, r.URL.Query().Get("w")+"x"+r.URL.Query().Get("h"))
		f.mu.Unlock()
		w.WriteHeader(200)
	case r.Method == "GET" && path == "/exec/exec1/json":
		jsonOut(w, 200, map[string]any{"ExitCode": 3, "Running": false})
	default:
		jsonOut(w, 404, map[string]string{"message": "page not found: " + path})
	}
}

func (f *fakeEngine) logs(w http.ResponseWriter) {
	if f.tty {
		_, _ = io.WriteString(w, "tty line 1\r\ntty line 2\r\n")
		return
	}
	w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	_, _ = w.Write(frame(1, "out 1\nout "))
	_, _ = w.Write(frame(2, "err 1\n"))
	_, _ = w.Write(frame(1, "2\n"))
}

func (f *fakeEngine) streamEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	w.(http.Flusher).Flush()
	for {
		select {
		case a := <-f.events:
			_ = json.NewEncoder(w).Encode(map[string]any{"Type": "container", "Action": a, "id": "abc"})
			w.(http.Flusher).Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// hijack answers 101 and then behaves like a TTY: it prints a prompt and echoes input uppercased.
func (f *fakeEngine) hijack(w http.ResponseWriter) {
	conn, brw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		f.t.Error(err)
		return
	}
	defer conn.Close()
	_, _ = brw.WriteString("HTTP/1.1 101 UPGRADED\r\nContent-Type: application/vnd.docker.raw-stream\r\nConnection: Upgrade\r\nUpgrade: tcp\r\n\r\n$ ")
	_ = brw.Flush()
	close(f.hijacked)
	rd := bufio.NewReader(brw)
	buf := make([]byte, 256)
	for {
		n, err := rd.Read(buf)
		if n > 0 {
			f.mu.Lock()
			f.shellIn.Write(buf[:n])
			f.mu.Unlock()
			_, _ = conn.Write([]byte(strings.ToUpper(string(buf[:n]))))
		}
		if err != nil {
			return
		}
	}
}
