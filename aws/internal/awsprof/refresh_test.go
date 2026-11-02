package awsprof

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func (r *Refresher) busy(profile string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.running[profile]
	return ok
}

// fakeTools puts scripts on PATH; each logs its arguments to <name>.log.
func fakeTools(t *testing.T, scripts map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range scripts {
		script := "#!/bin/sh\necho \"$@\" > " + filepath.Join(dir, name+".log") + "\n" + body
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir+":/bin:/usr/bin")
	return dir
}

func logged(t *testing.T, dir, name string) string {
	t.Helper()
	b, _ := os.ReadFile(filepath.Join(dir, name+".log"))
	return strings.TrimSpace(string(b))
}

func TestRefreshRunsTheRightCommand(t *testing.T) {
	dir := fakeTools(t, map[string]string{
		"aws":      "echo 'Successfully logged into Start URL'\n",
		"saml2aws": "echo 'Saving credentials'\n",
	})
	r := NewRefresher()
	sso := Profile{Name: "dev", Kind: KindSSO, Login: []string{"aws", "sso", "login", "--profile", "dev"}}
	if out, err := r.Refresh(t.Context(), sso); err != nil || !strings.Contains(out, "Successfully") {
		t.Fatalf("sso: %q %v", out, err)
	}
	if got := logged(t, dir, "aws"); got != "sso login --profile dev" {
		t.Errorf("aws args %q", got)
	}
	saml := Profile{Name: "prod", Kind: KindTemporary, Login: []string{"saml2aws", "login", "--profile", "prod", "--role", "arn:aws:iam::1:role/Ops"}}
	if _, err := r.Refresh(t.Context(), saml); err != nil {
		t.Fatal(err)
	}
	if got := logged(t, dir, "saml2aws"); got != "login --profile prod --role arn:aws:iam::1:role/Ops --skip-prompt --force" {
		t.Errorf("saml2aws args %q", got)
	}
	role := Profile{Name: "deploy", Kind: KindRole}
	if _, err := r.Refresh(t.Context(), role); err != nil {
		t.Fatal(err)
	}
	if got := logged(t, dir, "aws"); got != "sts get-caller-identity --profile deploy --output json" {
		t.Errorf("role args %q", got)
	}
	if _, err := r.Refresh(t.Context(), Profile{Name: "keys", Kind: KindKeys}); err == nil {
		t.Error("long-term keys refreshed")
	}
}

func TestRefreshThatNeedsAnAnswerFailsFast(t *testing.T) {
	fakeTools(t, map[string]string{
		"saml2aws": "echo '? Please choose the role'\necho 'error: EOF' >&2\nexit 1\n",
	})
	p := Profile{Name: "prod", Kind: KindTemporary, Login: []string{"saml2aws", "login", "--profile", "prod"}}
	if _, err := NewRefresher().Refresh(t.Context(), p); !errors.Is(err, ErrNeedsInput) {
		t.Fatalf("err = %v", err)
	}
}

func TestRefreshCanBeCancelledAndRunsOncePerProfile(t *testing.T) {
	fakeTools(t, map[string]string{"aws": "sleep 10\n"})
	r := NewRefresher()
	p := Profile{Name: "dev", Kind: KindSSO, Login: []string{"aws", "sso", "login", "--profile", "dev"}}
	done := make(chan error, 1)
	go func() {
		_, err := r.Refresh(t.Context(), p)
		done <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !r.busy("dev") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := r.Refresh(t.Context(), p); err == nil || !strings.Contains(err.Error(), "already") {
		t.Errorf("second refresh: %v", err)
	}
	if !r.Cancel("dev") {
		t.Fatal("nothing to cancel")
	}
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "cancelled") {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("refresh not cancelled")
	}
}

func TestRefreshableKinds(t *testing.T) {
	now := time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC)
	got, err := Load(writeFixture(t), now)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"default": false, "legacy-sso": true, "new-sso": true, "deploy": true, "tool": true, "azure-dev": false, "saml-prod": true, "azure-old": false}
	for _, p := range got {
		if p.Refreshable != want[p.Name] {
			t.Errorf("%s refreshable = %v", p.Name, p.Refreshable)
		}
	}
}
