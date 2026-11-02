package awsprof

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// identityTimeout bounds `aws sts get-caller-identity` (it may refresh SSO
// role credentials or run a credential_process first).
const identityTimeout = 30 * time.Second

// ErrNoCLI: the aws command isn't on PATH.
var ErrNoCLI = errors.New("the AWS CLI (aws) is not installed")

// Identity is who a profile's credentials belong to.
type Identity struct {
	Account string `json:"account"`
	Arn     string `json:"arn"`
	UserID  string `json:"userId"`
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9_.@+=,/][A-Za-z0-9_.@+=,/-]*$`)

// ValidName accepts profile names that are safe to pass to the CLI and to put
// in a shell environment (never anything that reads as a flag).
func ValidName(name string) bool { return len(name) <= 128 && nameRe.MatchString(name) }

// CallerIdentity asks STS who the profile's credentials belong to.
func CallerIdentity(ctx context.Context, profile string) (Identity, error) {
	if !ValidName(profile) {
		return Identity{}, fmt.Errorf("invalid profile name %q", profile)
	}
	bin, err := exec.LookPath("aws")
	if err != nil {
		return Identity{}, ErrNoCLI
	}
	ctx, cancel := context.WithTimeout(ctx, identityTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, "sts", "get-caller-identity", "--profile", profile, "--output", "json")
	cmd.Env = append(cmd.Environ(), "AWS_PAGER=", "AWS_CLI_AUTO_PROMPT=off")
	stopTree(cmd)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return Identity{}, errors.New(msg)
	}
	var id struct {
		Account, Arn, UserId string
	}
	if err := json.Unmarshal(out, &id); err != nil {
		return Identity{}, fmt.Errorf("unexpected aws output: %w", err)
	}
	return Identity{Account: id.Account, Arn: id.Arn, UserID: id.UserId}, nil
}
