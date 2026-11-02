package main

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/flint-ide/extensions/kubernetes/internal/kube"
)

// The kubectl (and flux) command lines the panel runs in terminal tabs, and
// the agent prompt of a diagnosis: the same argv and text as Flint's built-in
// Kubernetes panel (frontend/src/features/kubernetes/kube.ts), built here so
// the process checks every value before Flint types the command.

// validContext is kube's rule for context names: free-form (EKS uses ARNs),
// without control characters or a leading dash.
func validContext(s string) bool {
	return s != "" && len(s) <= 512 && !strings.HasPrefix(s, "-") && !strings.ContainsFunc(s, func(r rune) bool { return r < 0x20 || r == 0x7f })
}

// kindRe is a resource kind as kubectl takes it ("pod", "deployment",
// "kustomizations.kustomize.toolkit.fluxcd.io").
var kindRe = regexp.MustCompile(`^[a-z][a-z0-9]*(\.[a-z0-9-]+)*$`)

// checkScope validates a context and an optional namespace.
func checkScope(context, namespace string) error {
	if !validContext(context) {
		return fmt.Errorf("invalid context %q", context)
	}
	if namespace != "" && !kube.ValidName(namespace) {
		return fmt.Errorf("invalid namespace %q", namespace)
	}
	return nil
}

// checkTarget validates "kind/name" ("pod/api-1", "deployment/api").
func checkTarget(target string) error {
	kind, name, ok := strings.Cut(target, "/")
	if !ok || !kindRe.MatchString(kind) || !kube.ValidName(name) {
		return fmt.Errorf("invalid target %q", target)
	}
	return nil
}

// kubectl pins context and namespace: the global current context doesn't matter.
func kubectl(context, namespace string, args ...string) []string {
	out := []string{"kubectl", "--context=" + context}
	if namespace != "" {
		out = append(out, "--namespace="+namespace)
	}
	return append(out, args...)
}

// resourceRef is "pod/api-1", "deployment/api"… from a kind as the API returns it.
func resourceRef(kind, name string) string { return strings.ToLower(kind) + "/" + name }

// logsCommand follows a target's logs; target is "pod/name", "deployment/name"…
func logsCommand(context, namespace, target string) []string {
	return kubectl(context, namespace, "logs", "-f", "--tail=200", "--all-containers", "--prefix", target)
}

// execCommand is a shell in a pod: bash when the image has it, else sh.
func execCommand(context, namespace, pod, container string) []string {
	args := []string{"exec", "-it", pod}
	if container != "" {
		args = append(args, "-c", container)
	}
	args = append(args, "--", "sh", "-c", "command -v bash >/dev/null && exec bash || exec sh")
	return kubectl(context, namespace, args...)
}

func describeCommand(context, namespace, target string) []string {
	return kubectl(context, namespace, "describe", target)
}

func portForwardCommand(context, namespace, target string, port int) []string {
	p := strconv.Itoa(port)
	return kubectl(context, namespace, "port-forward", target, p+":"+p)
}

func rolloutStatusCommand(context, namespace, target string) []string {
	return kubectl(context, namespace, "rollout", "status", target)
}

func rolloutHistoryCommand(context, namespace, target string) []string {
	return kubectl(context, namespace, "rollout", "history", target)
}

// fluxReconcileCommand reconciles a Kustomization and its source, in the
// Kustomization's namespace.
func fluxReconcileCommand(context, name, namespace string) []string {
	return []string{"flux", "--context=" + context, "reconcile", "kustomization", name, "--namespace=" + namespace, "--with-source"}
}

var (
	eksContext = regexp.MustCompile(`^arn:aws:eks:[^:]+:(\d+):cluster/(.+)$`)
	gkeContext = regexp.MustCompile(`^gke_([^_]+)_[^_]+_(.+)$`)
)

// shortContext is a readable context name: EKS ARNs become "cluster
// (account)", GKE names "cluster (project)"; anything else stays.
func shortContext(name string) string {
	if m := eksContext.FindStringSubmatch(name); m != nil {
		return m[2] + " (" + m[1] + ")"
	}
	if m := gkeContext.FindStringSubmatch(name); m != nil {
		return m[2] + " (" + m[1] + ")"
	}
	return name
}

// lastPart is what follows the last "/" ("api-1" of "pod/api-1").
func lastPart(s string) string {
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		return s[i+1:]
	}
	return s
}

// maxPrompt is agent/ask's limit on a prompt, in bytes.
const maxPrompt = 20000

// diagnosePrompt is the agent prompt for an unhealthy app: read-only
// investigation with the cluster pinned, the fix proposed where it belongs
// (manifests in the repo), the gathered diagnostics attached. The diagnostics
// are cut so that the prompt fits agent/ask.
func diagnosePrompt(context, namespace, app, diagnostics string) string {
	head := strings.Join([]string{
		fmt.Sprintf(`The Kubernetes app "%s" in namespace %s (context %s) is unhealthy. Find the root cause.`, app, namespace, context),
		"",
		fmt.Sprintf("- Investigate with read-only kubectl only, always with --context=%s --namespace=%s (get, describe, logs, events, rollout history).", context, namespace),
		"- Do not change the cluster (no apply, edit, delete, scale, rollout restart/undo) unless I ask.",
		"- If the fix belongs in manifests in this repository (Kustomize, Helm, Flux), find them and propose the change there.",
		"- Start with the root cause in one or two sentences, then the evidence, then the fix.",
		"",
		"What Flint gathered (secrets redacted):",
		"",
		"",
	}, "\n")
	body := strings.TrimSpace(diagnostics)
	const cutNote = "\n…(cut)"
	if room := maxPrompt - len(head); len(body) > room {
		body = cutUTF8(body, max(0, room-len(cutNote))) + cutNote
	}
	return head + body
}

// cutUTF8 shortens s to at most n bytes without splitting a character.
func cutUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
