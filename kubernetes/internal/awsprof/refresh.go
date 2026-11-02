package awsprof

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

// Timeouts of a refresh: SSO waits for the user to approve in the browser.
const (
	ssoRefreshTimeout   = 5 * time.Minute
	loginRefreshTimeout = 2 * time.Minute
	outputTailLines     = 6
)

// ErrNeedsInput: the login tool asked a question (MFA code, role choice…);
// the terminal login (Profile.Login) can answer it.
var ErrNeedsInput = errors.New("the login needs input: use Log In to run it in a terminal")

// refreshCommand is how a profile's credentials are renewed without a
// terminal: aws sso login, saml2aws without prompts (saved password and role),
// or an STS call that makes the CLI fetch and cache role or
// credential_process credentials.
func refreshCommand(p Profile) ([]string, time.Duration, bool) {
	switch {
	case p.Kind == KindSSO:
		return p.Login, ssoRefreshTimeout, len(p.Login) > 0
	case len(p.Login) > 0 && p.Login[0] == "saml2aws":
		return append(slices.Clone(p.Login), "--skip-prompt", "--force"), loginRefreshTimeout, true
	case p.Kind == KindRole || p.Kind == KindProcess:
		return []string{"aws", "sts", "get-caller-identity", "--profile", p.Name, "--output", "json"}, loginRefreshTimeout, true
	}
	return nil, 0, false
}

// Refresher runs at most one refresh per profile and can cancel it.
type Refresher struct {
	mu      sync.Mutex
	running map[string]context.CancelFunc
}

// NewRefresher returns an idle Refresher.
func NewRefresher() *Refresher { return &Refresher{running: map[string]context.CancelFunc{}} }

// Refresh renews p's credentials and returns the tail of the tool's output
// (SSO prints the verification code there).
func (r *Refresher) Refresh(ctx context.Context, p Profile) (string, error) {
	if !ValidName(p.Name) {
		return "", fmt.Errorf("invalid profile name %q", p.Name)
	}
	argv, timeout, ok := refreshCommand(p)
	if !ok {
		return "", fmt.Errorf("%s has nothing to refresh", p.Name)
	}
	bin, err := exec.LookPath(argv[0])
	if err != nil {
		return "", fmt.Errorf("%s is not installed", argv[0])
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	r.mu.Lock()
	if _, busy := r.running[p.Name]; busy {
		r.mu.Unlock()
		return "", fmt.Errorf("%s is already being refreshed", p.Name)
	}
	r.running[p.Name] = cancel
	r.mu.Unlock()
	defer func() {
		r.mu.Lock()
		delete(r.running, p.Name)
		r.mu.Unlock()
	}()

	cmd := exec.CommandContext(ctx, bin, argv[1:]...)
	cmd.Env = append(cmd.Environ(), "AWS_PAGER=", "AWS_CLI_AUTO_PROMPT=off")
	stopTree(cmd)
	// No stdin: a tool that asks a question gets EOF and fails instead of hanging.
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err = cmd.Run()
	tail := lastLines(out.String(), outputTailLines)
	switch {
	case err == nil:
		return tail, nil
	case errors.Is(ctx.Err(), context.Canceled):
		return tail, errors.New("refresh cancelled")
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return tail, fmt.Errorf("no answer within %s", timeout)
	case p.Kind != KindSSO && asksForInput(out.String()):
		return tail, ErrNeedsInput
	}
	if tail == "" {
		return "", err
	}
	return tail, errors.New(tail)
}

// Cancel stops a running refresh of profile; false when none runs.
func (r *Refresher) Cancel(profile string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	cancel, ok := r.running[profile]
	if ok {
		cancel()
	}
	return ok
}

// asksForInput spots prompts that failed on the missing stdin.
func asksForInput(out string) bool {
	low := strings.ToLower(out)
	for _, s := range []string{"eof", "mfa", "verification code", "please choose", "password"} {
		if strings.Contains(low, s) {
			return true
		}
	}
	return false
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}
