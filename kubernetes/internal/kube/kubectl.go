// Package kube shows Kubernetes clusters through the kubectl CLI: the contexts
// of the kubeconfig, namespaces, workloads, pods and services, and a few
// actions (rollout restart, scale, delete pod, use-context). kubectl handles
// every auth plugin (aws eks get-token, gke, OIDC…), so Flint needs none.
package kube

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// Timeouts: listing is quick or the cluster is unreachable; actions may wait
// for the API server a little longer.
const (
	readTimeout   = 20 * time.Second
	actionTimeout = 30 * time.Second
)

// ErrNoKubectl: kubectl isn't on PATH.
var ErrNoKubectl = errors.New("kubectl is not installed (or not on the login shell's PATH)")

// Kubectl runs the kubectl binary found on PATH.
type Kubectl struct {
	bin string
}

// New finds kubectl.
func New() (*Kubectl, error) {
	bin, err := exec.LookPath("kubectl")
	if err != nil {
		return nil, ErrNoKubectl
	}
	return &Kubectl{bin: bin}, nil
}

// run executes kubectl with args; stderr becomes the error text.
func (k *Kubectl) run(ctx context.Context, timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, k.bin, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("kubectl %s: no answer within %s (cluster unreachable?)", args[0], timeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errors.New(msg)
	}
	return out, nil
}

// scope is the --context / --namespace pair every cluster call starts with;
// values are passed as --flag=value so they can never read as other flags.
func scope(kubeContext, namespace string) ([]string, error) {
	if !validContext(kubeContext) {
		return nil, fmt.Errorf("invalid context %q", kubeContext)
	}
	args := []string{"--context=" + kubeContext, "--request-timeout=15s"}
	if namespace != "" {
		if !validName(namespace) {
			return nil, fmt.Errorf("invalid namespace %q", namespace)
		}
		args = append(args, "--namespace="+namespace)
	}
	return args, nil
}

// Context names are free-form (EKS uses ARNs); only control characters and a
// leading dash are refused.
func validContext(s string) bool {
	return s != "" && len(s) <= 512 && !strings.HasPrefix(s, "-") && !strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// nameRe is a Kubernetes object name (DNS-1123 subdomain).
var nameRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9.]{0,251}[a-z0-9])?$`)

func validName(s string) bool { return nameRe.MatchString(s) }

// ValidName reports whether s is a valid object or namespace name.
func ValidName(s string) bool { return validName(s) }
