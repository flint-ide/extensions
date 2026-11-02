package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestListNegotiatesVersionAndConvertsContainers(t *testing.T) {
	f := newFake(t)
	d := f.client()
	cs, err := d.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	calls := f.callList()
	if calls[0] != "GET /_ping" || calls[1] != "GET /v1.47/containers/json?all=1" {
		t.Fatalf("calls %q: want ping, then the negotiated v1.47 list", calls)
	}
	if len(cs) != 2 {
		t.Fatalf("containers %+v", cs)
	}
	db := cs[0]
	if db.Name != "shop-db-1" || db.Project != "shop" || db.Service != "db" || db.WorkingDir != "/p/shop" || db.ConfigFiles != "/p/shop/compose.yaml,/p/shop/compose.override.yaml" {
		t.Fatalf("labels %+v", db)
	}
	if len(db.Ports) != 1 || db.Ports[0].HostPort != 5432 || db.Ports[0].Container != "5432/tcp" || db.Ports[0].Host != "0.0.0.0" {
		t.Fatalf("ports %+v (IPv6 twin and unpublished port must go)", db.Ports)
	}
	if cs[1].Project != "" || len(cs[1].Ports) != 0 || cs[1].State != "exited" || cs[1].Created == "" {
		t.Fatalf("plain container %+v", cs[1])
	}
	// The version is negotiated once.
	if _, err := d.List(context.Background()); err != nil {
		t.Fatal(err)
	}
	pings := 0
	for _, c := range f.callList() {
		if c == "GET /_ping" {
			pings++
		}
	}
	if pings != 1 {
		t.Fatalf("pinged %d times", pings)
	}
}

func TestOlderEngineGetsItsOwnVersion(t *testing.T) {
	f := newFake(t)
	f.pingHdr = "1.30"
	if _, err := f.client().List(context.Background()); err == nil || !strings.Contains(err.Error(), "page not found") {
		// The fake only serves 1.47; an engine at 1.30 must be asked for v1.30.
		t.Fatalf("err = %v", err)
	}
	if got := f.callList()[1]; !strings.HasPrefix(got, "GET /v1.30/") {
		t.Fatalf("second call %q", got)
	}
	f2 := newFake(t)
	f2.pingHdr = "1.12"
	if _, err := f2.client().List(context.Background()); err == nil || !strings.Contains(err.Error(), "too old") {
		t.Fatalf("err = %v", err)
	}
}

func TestActsThroughTheAPI(t *testing.T) {
	f := newFake(t)
	d := f.client()
	ctx := context.Background()
	for _, a := range []string{"start", "stop", "restart", "rm"} {
		if err := d.Act(ctx, "shop-db-1", a); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
	}
	if err := d.Act(ctx, "already-up", "start"); err != nil {
		t.Fatalf("304 must count as done: %v", err)
	}
	if err := d.Act(ctx, "busy", "rm"); err == nil || !strings.Contains(err.Error(), "cannot remove a running container") {
		t.Fatalf("engine message lost: %v", err)
	}
	got := strings.Join(f.callList()[1:], "|") // after the ping
	want := "POST /v1.47/containers/shop-db-1/start|POST /v1.47/containers/shop-db-1/stop|POST /v1.47/containers/shop-db-1/restart|DELETE /v1.47/containers/shop-db-1|POST /v1.47/containers/already-up/start|DELETE /v1.47/containers/busy"
	if got != want {
		t.Fatalf("calls\n%s\nwant\n%s", got, want)
	}
}

func TestRejectsUnsafeInput(t *testing.T) {
	f := newFake(t)
	d := f.client()
	ctx := context.Background()
	for _, err := range []error{
		d.Act(ctx, "--privileged", "rm"),
		d.Act(ctx, "../images/prune", "rm"),
		d.Act(ctx, "a/b", "start"),
		d.Act(ctx, "x", "exec"),
		d.Act(ctx, "", "start"),
		d.Compose(ctx, ComposeProject{Name: "-x", Dir: "/", Files: []string{"/a.yaml"}}, "up"),
		d.Compose(ctx, ComposeProject{Name: "a", Dir: "/", Files: []string{"/a.yaml"}}, "rm -f"),
		func() error { _, err := d.Logs(ctx, "x y", 10); return err }(),
		func() error { _, err := d.Inspect(ctx, "a?b"); return err }(),
		func() error { _, err := d.OpenShell(ctx, "$(x)", 80, 24); return err }(),
	} {
		if err == nil {
			t.Error("unsafe input accepted")
		}
	}
	if calls := f.callList(); len(calls) != 0 {
		t.Fatalf("engine was called: %q", calls)
	}
}

func TestInspect(t *testing.T) {
	f := newFake(t)
	d := f.client()
	det, err := d.Inspect(context.Background(), "web")
	if err != nil || det.Name != "web" || !det.Running || det.TTY || det.Image != "busybox" {
		t.Fatalf("%+v %v", det, err)
	}
	if _, err := d.Inspect(context.Background(), "ghost"); err == nil || !strings.Contains(err.Error(), "No such container") {
		t.Fatalf("err = %v", err)
	}
}

func TestDaemonDownIsExplained(t *testing.T) {
	d, err := New(Options{Host: "unix://" + filepath.Join(t.TempDir(), "none.sock")})
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.List(context.Background())
	if !errors.Is(err, ErrNotRunning) || !strings.Contains(err.Error(), "Docker isn't running") {
		t.Fatalf("err = %v", err)
	}
}

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestLogsDemuxAndTerminalLineEnds(t *testing.T) {
	f := newFake(t)
	d := f.client()
	rc, err := d.Logs(context.Background(), "web", 20000)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if got := readAll(t, rc); got != "out 1\r\nout err 1\r\n2\r\n" {
		t.Fatalf("logs %q", got)
	}
	var logsCall string
	for _, c := range f.callList() {
		if strings.Contains(c, "/logs") {
			logsCall = c
		}
	}
	for _, want := range []string{"follow=1", "stdout=1", "stderr=1", "tail=10000"} {
		if !strings.Contains(logsCall, want) {
			t.Errorf("%q lacks %s (tail must be capped)", logsCall, want)
		}
	}
}

func TestTTYLogsAreNotDemuxed(t *testing.T) {
	f := newFake(t)
	f.tty = true
	rc, err := f.client().Logs(context.Background(), "web", 5)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	if got := readAll(t, rc); got != "tty line 1\r\ntty line 2\r\n" {
		t.Fatalf("logs %q", got)
	}
}

func TestDemuxFrames(t *testing.T) {
	in := string(frame(1, "a\n")) + string(frame(2, "b")) + string(frame(1, "")) + string(frame(1, "c\n"))
	if got := readAll(t, Demux(strings.NewReader(in))); got != "a\nbc\n" {
		t.Fatalf("got %q", got)
	}
	// A frame cut short ends the stream with what arrived.
	if got := readAll(t, Demux(strings.NewReader(string(frame(1, "hello"))[:10]))); got != "he" {
		t.Fatalf("truncated %q", got)
	}
	if _, err := io.ReadAll(Demux(strings.NewReader("GARBAGE!!more"))); err == nil {
		t.Fatal("a stream that isn't multiplexed must be an error")
	}
}

func TestWatchDebouncesAndSkipsExecNoise(t *testing.T) {
	f := newFake(t)
	d := f.client()
	ch := make(chan struct{}, 4)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- d.Watch(ctx, 30*time.Millisecond, func() { ch <- struct{}{} }) }()
	for _, a := range []string{"exec_start: sh", "exec_create: sh", "top"} {
		f.events <- a
	}
	select {
	case <-ch:
		t.Fatal("exec/top noise triggered a refresh")
	case <-time.After(150 * time.Millisecond):
	}
	f.events <- "start"
	f.events <- "stop"
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("no change reported")
	}
	select {
	case <-ch:
		t.Fatal("start+stop were not debounced into one change")
	case <-time.After(150 * time.Millisecond):
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("watch ended with %v", err)
	}
	var evCall string
	for _, c := range f.callList() {
		if strings.Contains(c, "/events") {
			evCall = c
		}
	}
	if !strings.Contains(evCall, "filters=") || !strings.Contains(evCall, "container") {
		t.Fatalf("events call %q", evCall)
	}
}

func TestShellOverHijackedConnection(t *testing.T) {
	f := newFake(t)
	d := f.client()
	ctx := context.Background()
	sh, err := d.OpenShell(ctx, "web", 120, 40)
	if err != nil {
		t.Fatal(err)
	}
	defer sh.Close()
	buf := make([]byte, 2)
	if _, err := io.ReadFull(sh, buf); err != nil || string(buf) != "$ " {
		t.Fatalf("prompt %q %v", buf, err)
	}
	if _, err := sh.Write([]byte("ls\n")); err != nil {
		t.Fatal(err)
	}
	echo := make([]byte, 3)
	if _, err := io.ReadFull(sh, echo); err != nil || string(echo) != "LS\n" {
		t.Fatalf("echo %q %v", echo, err)
	}
	if err := sh.Resize(ctx, 90, 30); err != nil {
		t.Fatal(err)
	}
	if err := sh.Resize(ctx, 0, 30); err == nil {
		t.Error("zero size accepted")
	}
	f.mu.Lock()
	resizes := strings.Join(f.resizes, ",")
	f.mu.Unlock()
	if resizes != "120x40,90x30" {
		t.Fatalf("resizes %q", resizes)
	}
	sh.Close()
	if _, err := io.ReadAll(sh); err == nil {
		// closed connection: the read ends with an error or EOF, both fine
		_ = err
	}
	if code := sh.ExitCode(ctx); code != 3 {
		t.Fatalf("exit code %d", code)
	}
}

func TestShellNeedsARunningContainer(t *testing.T) {
	f := newFake(t)
	f.running = false
	if _, err := f.client().OpenShell(context.Background(), "web", 80, 24); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("err = %v", err)
	}
}

// fakeCLI puts a docker script on PATH that records its arguments.
func fakeCLI(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "args.log")
	script := "#!/bin/sh\necho \"$@\" >> " + log + "\n"
	if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return log
}

func TestComposeUsesTheCLIWhenPresent(t *testing.T) {
	log := fakeCLI(t)
	f := newFake(t)
	d := f.client()
	proj := ComposeProject{Name: "shop", Dir: t.TempDir(), Files: []string{"/p/shop/compose.yaml"}}
	for _, a := range []string{"up", "restart"} {
		if err := d.Compose(context.Background(), proj, a); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.Compose(context.Background(), ComposeProject{Name: "a", Dir: "/", Files: []string{"rel.yaml"}}, "up"); err == nil {
		t.Error("relative compose file accepted")
	}
	b, _ := os.ReadFile(log)
	want := "compose -p shop -f /p/shop/compose.yaml up -d\ncompose -p shop -f /p/shop/compose.yaml restart\n"
	if string(b) != want {
		t.Fatalf("cli calls %q, want %q", b, want)
	}
	if !ComposeAvailable() {
		t.Error("compose should be available")
	}
}

func TestComposeWithoutCLI(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if ComposeAvailable() {
		t.Fatal("no CLI expected")
	}
	f := newFake(t)
	d := f.client()
	ctx := context.Background()
	proj := ComposeProject{Name: "shop", Dir: "/p/shop", Files: []string{"/p/shop/compose.yaml"}}
	for _, a := range []string{"up", "down", "pull"} {
		if err := d.Compose(ctx, proj, a); !errors.Is(err, ErrComposeNeedsCLI) {
			t.Errorf("%s: err = %v", a, err)
		}
	}
	if err := d.Compose(ctx, proj, "restart"); err != nil {
		t.Fatal(err)
	}
	if err := d.Compose(ctx, proj, "stop"); err != nil {
		t.Fatal(err)
	}
	calls := strings.Join(f.callList(), "|")
	// Only the running container of the project is touched; the unrelated one is not.
	if !strings.Contains(calls, "POST /v1.47/containers/abc123def456/restart") || !strings.Contains(calls, "POST /v1.47/containers/abc123def456/stop") || strings.Contains(calls, "def456abc123") {
		t.Fatalf("calls %s", calls)
	}
}

func TestFindComposeFiles(t *testing.T) {
	root := t.TempDir()
	for _, p := range []string{"compose.yaml", "api/docker-compose.yml", "node_modules/x/compose.yaml", ".hidden/compose.yaml", "deep/a/compose.yaml"} {
		full := filepath.Join(root, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte("services: {}\n"), 0o644)
	}
	got := FindComposeFiles([]string{root})
	if len(got) != 2 || got[0].Dir != root || got[1].Dir != filepath.Join(root, "api") {
		t.Fatalf("found %+v", got)
	}
}
