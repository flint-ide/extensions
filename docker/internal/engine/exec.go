package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// shellCmd runs bash when the container has it, else sh.
var shellCmd = []string{"sh", "-c", "command -v bash >/dev/null && exec bash || exec sh"}

// Shell is an interactive terminal inside a container: an exec with a TTY on
// a hijacked connection. Read and Write are the terminal's output and input.
type Shell struct {
	d      *Docker
	execID string
	conn   net.Conn
	br     *bufio.Reader

	closeOnce sync.Once
}

// Read returns the terminal output.
func (s *Shell) Read(p []byte) (int, error) { return s.br.Read(p) }

// Write sends keystrokes.
func (s *Shell) Write(p []byte) (int, error) { return s.conn.Write(p) }

// Close hangs up the connection (the shell gets a hang-up like a closed window).
func (s *Shell) Close() { s.closeOnce.Do(func() { _ = s.conn.Close() }) }

// Resize tells the engine the terminal's size.
func (s *Shell) Resize(ctx context.Context, cols, rows int) error {
	if cols <= 0 || rows <= 0 || cols > 10000 || rows > 10000 {
		return fmt.Errorf("invalid terminal size %dx%d", cols, rows)
	}
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	q := url.Values{"w": {strconv.Itoa(cols)}, "h": {strconv.Itoa(rows)}}
	resp, err := s.d.request(ctx, http.MethodPost, "/exec/"+s.execID+"/resize", q, nil)
	if err != nil {
		return err
	}
	return resp.Body.Close()
}

// ExitCode is the shell's exit status once it ended (-1 when unknown).
func (s *Shell) ExitCode(ctx context.Context) int {
	ctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	var info struct{ ExitCode *int }
	if err := s.d.getJSON(ctx, "/exec/"+s.execID+"/json", nil, &info, true); err != nil || info.ExitCode == nil {
		return -1
	}
	return *info.ExitCode
}

// OpenShell starts a shell in a running container.
func (d *Docker) OpenShell(ctx context.Context, id string, cols, rows int) (*Shell, error) {
	if !validID(id) {
		return nil, fmt.Errorf("invalid container id %q", id)
	}
	details, err := d.Inspect(ctx, id)
	if err != nil {
		return nil, err
	}
	if !details.Running {
		return nil, fmt.Errorf("container %s is not running", details.Name)
	}
	cctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	create := map[string]any{
		"AttachStdin": true, "AttachStdout": true, "AttachStderr": true, "Tty": true,
		"Cmd": shellCmd, "Env": []string{"TERM=xterm-256color"},
	}
	resp, err := d.request(cctx, http.MethodPost, "/containers/"+id+"/exec", nil, create)
	if err != nil {
		return nil, err
	}
	var created struct {
		ID string `json:"Id"`
	}
	err = decodeCapped(resp.Body, &created)
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	if !validID(created.ID) {
		return nil, fmt.Errorf("docker: unexpected exec id %q", created.ID)
	}
	conn, br, err := d.hijack(ctx, "/exec/"+created.ID+"/start", map[string]any{"Detach": false, "Tty": true})
	if err != nil {
		return nil, err
	}
	sh := &Shell{d: d, execID: created.ID, conn: conn, br: br}
	if cols > 0 && rows > 0 {
		_ = sh.Resize(ctx, cols, rows) // best effort: the first resize event follows from the view
	}
	return sh, nil
}

// hijack POSTs body with an Upgrade and returns the raw connection once the
// engine switched protocols (101) or started the raw stream (200).
func (d *Docker) hijack(ctx context.Context, path string, body any) (net.Conn, *bufio.Reader, error) {
	v, err := d.apiVersion(ctx)
	if err != nil {
		return nil, nil, err
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, nil, err
	}
	dctx, cancel := context.WithTimeout(ctx, cmdTimeout)
	defer cancel()
	conn, err := d.ep.dial(dctx)
	if err != nil {
		return nil, nil, d.connErr(err)
	}
	req, err := http.NewRequest(http.MethodPost, d.base+"/v"+v+path, bytes.NewReader(payload))
	if err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "tcp")
	req.Header.Set("User-Agent", "flint")
	if dl, ok := dctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}
	if err := req.Write(conn); err != nil {
		_ = conn.Close()
		return nil, nil, err
	}
	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, req)
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("docker: %w", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols && resp.StatusCode != http.StatusOK {
		defer conn.Close()
		return nil, nil, statusError(resp)
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, br, nil
}

// compile-time check: a Shell is a terminal stream's reader and writer.
var _ io.ReadWriter = (*Shell)(nil)
