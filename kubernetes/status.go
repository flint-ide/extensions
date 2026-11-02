package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/flint-ide/extensions/kubernetes/internal/kube"
)

// The agent tool kubernetes_status: Flint's get_kubernetes_status
// (internal/mcptools/kube.go) moved into the extension.

// maxKubeItems bounds each list of the cluster summary.
const maxKubeItems = 30

// kubeStatus summarises what is wrong in namespace (the current context's
// own namespace when empty) of the kubeconfig's current context. Only
// statuses, reasons and event messages are read; never secrets or logs.
func kubeStatus(ctx context.Context, k cluster, namespace string) (string, error) {
	if namespace != "" && !kube.ValidName(namespace) {
		return "", fmt.Errorf("invalid namespace %q", namespace)
	}
	contexts, err := k.Contexts(ctx)
	if err != nil {
		return "", err
	}
	var current *kube.Context
	for i := range contexts {
		if contexts[i].Current {
			current = &contexts[i]
		}
	}
	if current == nil {
		return "", errors.New("the kubeconfig has no current context")
	}
	if namespace == "" {
		namespace = current.Namespace
	}
	if namespace == "" {
		namespace = "default"
	}
	ov, err := k.Overview(ctx, current.Name, namespace)
	if err != nil {
		return "", err
	}
	return formatOverview(*current, namespace, ov), nil
}

func formatOverview(c kube.Context, namespace string, ov kube.Overview) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Context: %s (cluster %s), namespace: %s\n", c.Name, c.Cluster, namespace)
	health := map[string]int{}
	for _, a := range ov.Apps {
		health[a.Health]++
	}
	fmt.Fprintf(&b, "Apps: %d (%d ok, %d warning, %d failing); pods: %d\n",
		len(ov.Apps), health[kube.HealthOK], health[kube.HealthWarn], health[kube.HealthBad], len(ov.Pods))

	section(&b, "Apps with problems", ov.Apps, func(a kube.App) (string, bool) {
		if a.Health == kube.HealthOK {
			return "", false
		}
		return fmt.Sprintf("%s (%s, %s): %s", a.Name, a.Source, a.Health, strings.Join(a.Problems, "; ")), true
	})
	section(&b, "Workloads not rolled out", ov.Workloads, func(w kube.Workload) (string, bool) {
		if w.Rollout == kube.RolloutComplete || w.Rollout == kube.RolloutScaledDown {
			return "", false
		}
		return fmt.Sprintf("%s/%s: %d/%d ready, rollout %s %s", w.Kind, w.Name, w.Ready, w.Desired, w.Rollout, w.RolloutMessage), true
	})
	section(&b, "Unhealthy pods", ov.Pods, func(p kube.Pod) (string, bool) {
		if p.Healthy || p.Status == "Terminating" {
			return "", false
		}
		line := fmt.Sprintf("%s: %s, %d/%d ready, %d restarts", p.Name, p.Status, p.Ready, p.Total, p.Restarts)
		if p.Problem != "" {
			line += " — " + p.Problem
		}
		return line, true
	})
	section(&b, "Failed jobs", ov.Jobs, func(j kube.Job) (string, bool) {
		if !strings.HasPrefix(j.Status, "failed") {
			return "", false
		}
		return j.Kind + "/" + j.Name + ": " + j.Status, true
	})
	section(&b, "Recent warning events", ov.Events, func(e kube.Event) (string, bool) {
		return fmt.Sprintf("%s/%s %s (×%d, %s): %s", e.Kind, e.Name, e.Reason, e.Count, e.LastSeen, e.Message), true
	})
	if len(ov.Partial) > 0 {
		fmt.Fprintf(&b, "\nNot readable (permissions or missing CRDs): %s\n", strings.Join(ov.Partial, ", "))
	}
	return strings.TrimRight(b.String(), "\n")
}

// section writes the items line picks, at most maxKubeItems of them.
func section[T any](b *strings.Builder, title string, items []T, line func(T) (string, bool)) {
	var lines []string
	for _, it := range items {
		if l, ok := line(it); ok {
			lines = append(lines, l)
		}
	}
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s (%d):\n", title, len(lines))
	for _, l := range lines[:min(len(lines), maxKubeItems)] {
		b.WriteString("- " + l + "\n")
	}
	if len(lines) > maxKubeItems {
		fmt.Fprintf(b, "- … and %d more\n", len(lines)-maxKubeItems)
	}
}
