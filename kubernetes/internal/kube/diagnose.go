package kube

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/flint-ide/extensions/kubernetes/internal/redact"
)

// Limits of what a diagnosis gathers: enough for an agent to find the
// cause, small enough to send.
const (
	diagnosePods     = 4
	diagnoseLogLines = 60
	maxDiagnosis     = 40 << 10
)

// Diagnose gathers what an AI agent needs to find why an app is unhealthy:
// its rollouts, images, problems and warning events, and the recent (and,
// after restarts, previous) logs of its failing containers. Credentials and
// the home directory are redacted: this text goes to an AI provider.
func (k *Kubectl) Diagnose(ctx context.Context, kubeContext, namespace, appKey string) (string, error) {
	ov, err := k.Overview(ctx, kubeContext, namespace)
	if err != nil {
		return "", err
	}
	var app *App
	for i := range ov.Apps {
		if ov.Apps[i].Key == appKey {
			app = &ov.Apps[i]
		}
	}
	if app == nil {
		return "", fmt.Errorf("app %q is no longer in namespace %s", appKey, namespace)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Context: %s\nNamespace: %s\nApp: %s (%s), health: %s\n", kubeContext, namespace, app.Name, app.Source, app.Health)
	if ks := app.Kustomization; ks != nil {
		fmt.Fprintf(&b, "Flux Kustomization %s/%s: ready=%s %s %s, revision %s, suspended=%v\n", ks.Namespace, ks.Name, ks.Ready, ks.Reason, ks.Message, ks.Revision, ks.Suspended)
	}
	if len(app.Problems) > 0 {
		b.WriteString("\nProblems:\n")
		for _, p := range app.Problems {
			b.WriteString("- " + p + "\n")
		}
	}
	b.WriteString("\nWorkloads:\n")
	var failing []Pod
	for _, ref := range app.Workloads {
		for _, w := range ov.Workloads {
			if w.Kind+"/"+w.Name != ref {
				continue
			}
			fmt.Fprintf(&b, "- %s %s: %d/%d ready, %d updated, rollout %s %s, revision %s, images %s\n",
				w.Kind, w.Name, w.Ready, w.Desired, w.Updated, w.Rollout, w.RolloutMessage, w.Revision, strings.Join(w.Images, ", "))
			for _, p := range ov.Pods {
				if p.Owner != w.Name {
					continue
				}
				fmt.Fprintf(&b, "  - pod %s: %s, %d/%d ready, %d restarts, node %s%s\n", p.Name, p.Status, p.Ready, p.Total, p.Restarts, p.Node, oldTag(p.Old))
				if !p.Healthy && p.Status != "Terminating" {
					if p.Problem != "" {
						b.WriteString("    problem: " + p.Problem + "\n")
					}
					failing = append(failing, p)
				}
			}
		}
	}
	for _, name := range app.Services {
		for _, s := range ov.Services {
			if s.Name == name {
				fmt.Fprintf(&b, "- Service %s (%s): %d of %d backing pods ready\n", s.Name, s.Type, s.Ready, s.Backends)
			}
		}
	}
	for _, name := range app.Ingresses {
		for _, in := range ov.Ingresses {
			if in.Name == name {
				for _, r := range in.Routes {
					fmt.Fprintf(&b, "- Ingress %s: %s%s → %s:%s\n", in.Name, r.Host, r.Path, r.Service, r.Port)
				}
			}
		}
	}
	for _, p := range failing[:min(len(failing), diagnosePods)] {
		for _, c := range p.Containers {
			b.WriteString(k.podLogs(ctx, kubeContext, namespace, p.Name, c, false))
			if p.Restarts > 0 {
				b.WriteString(k.podLogs(ctx, kubeContext, namespace, p.Name, c, true))
			}
		}
	}
	out := redact.String(b.String())
	if len(out) > maxDiagnosis {
		cut := maxDiagnosis
		for cut > 0 && !utf8.RuneStart(out[cut]) {
			cut-- // never split a character
		}
		out = out[:cut] + "\n…(cut)\n"
	}
	return out, nil
}

func oldTag(old bool) string {
	if old {
		return " (previous revision)"
	}
	return ""
}

// podLogs is a fenced block of a container's last log lines ("" when there are none).
func (k *Kubectl) podLogs(ctx context.Context, kubeContext, namespace, pod, container string, previous bool) string {
	args, err := scope(kubeContext, namespace)
	if err != nil || !validName(pod) || !validName(container) {
		return ""
	}
	args = append(args, "logs", pod, "-c", container, fmt.Sprintf("--tail=%d", diagnoseLogLines))
	label := "logs"
	if previous {
		args, label = append(args, "--previous"), "logs of the previous (crashed) run"
	}
	out, err := k.run(ctx, readTimeout, args...)
	text := strings.TrimSpace(string(out))
	if err != nil {
		text = "(unavailable: " + err.Error() + ")"
	}
	if text == "" {
		return ""
	}
	return fmt.Sprintf("\n%s, pod %s container %s:\n```\n%s\n```\n", label, pod, container, text)
}
