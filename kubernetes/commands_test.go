package main

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/flint-ide/extensions/kubernetes/internal/kube"
)

// Flint's frontend/src/features/kubernetes/kube.test.ts, for what moved to Go.

func TestShortContext(t *testing.T) {
	for in, want := range map[string]string{
		"arn:aws:eks:eu-central-1:200283852841:cluster/das-sese-dev": "das-sese-dev (200283852841)",
		"gke_acme-prod_europe-west1-b_main":                          "main (acme-prod)",
		"docker-desktop":                                             "docker-desktop",
	} {
		if got := shortContext(in); got != want {
			t.Errorf("shortContext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCommandsPinContextAndNamespace(t *testing.T) {
	ctx := "arn:aws:eks:eu-central-1:1:cluster/prod"
	if got := strings.Join(logsCommand(ctx, "apps", "deployment/api"), " "); got != "kubectl --context="+ctx+" --namespace=apps logs -f --tail=200 --all-containers --prefix deployment/api" {
		t.Errorf("logs: %s", got)
	}
	want := []string{"kubectl", "--context=docker-desktop", "--namespace=default", "exec", "-it", "web-1", "-c", "app", "--", "sh", "-c", "command -v bash >/dev/null && exec bash || exec sh"}
	if got := execCommand("docker-desktop", "default", "web-1", "app"); !slices.Equal(got, want) {
		t.Errorf("exec: %q", got)
	}
	if got := strings.Join(fluxReconcileCommand("prod", "shop", "flux-system"), " "); got != "flux --context=prod reconcile kustomization shop --namespace=flux-system --with-source" {
		t.Errorf("flux: %s", got)
	}
	if got := strings.Join(kubectl("dev", "", "get", "pods"), " "); got != "kubectl --context=dev get pods" {
		t.Errorf("no namespace: %s", got)
	}
}

func TestDiagnosePrompt(t *testing.T) {
	p := diagnosePrompt("prod", "shop", "shop", "Problems:\n- pod crashed\n")
	for _, want := range []string{"--context=prod --namespace=shop", "Do not change the cluster", "What Flint gathered (secrets redacted):\n\nProblems:"} {
		if !strings.Contains(p, want) {
			t.Errorf("missing %q in:\n%s", want, p)
		}
	}
	if !strings.HasSuffix(p, "- pod crashed") {
		t.Errorf("prompt ends with %q", p[len(p)-20:])
	}
	// Long diagnostics are cut to fit agent/ask, never inside a character.
	long := diagnosePrompt("prod", "shop", "shop", strings.Repeat("é", 30000))
	if len(long) > maxPrompt || !strings.HasSuffix(long, "…(cut)") || !strings.Contains(long, "Find the root cause") {
		t.Errorf("long prompt: %d bytes, ends %q", len(long), long[len(long)-10:])
	}
	if !strings.HasSuffix(strings.TrimSuffix(long, "\n…(cut)"), "é") {
		t.Error("a character was split")
	}
}

func TestChecks(t *testing.T) {
	for _, ok := range []string{"pod/web-1", "deployment/api", "kustomizations.kustomize.toolkit.fluxcd.io/shop"} {
		if err := checkTarget(ok); err != nil {
			t.Errorf("%s: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "pod", "--all", "pod/-x", "Pod/x", "pod/x y"} {
		if checkTarget(bad) == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if checkScope("arn:aws:eks:eu-central-1:1:cluster/prod", "") != nil || checkScope("-x", "") == nil || checkScope("dev", "--all") == nil {
		t.Error("scope checks")
	}
}

func TestKubeStatus(t *testing.T) {
	k := &fakeCluster{
		contexts: []kube.Context{{Name: "other"}, {Name: "dev", Cluster: "kind-dev", Namespace: "shop", Current: true}},
		ov: kube.Overview{
			Apps: []kube.App{
				{Name: "web", Source: "helm", Health: kube.HealthBad, Problems: []string{"0/2 pods ready"}},
				{Name: "db", Source: "label", Health: kube.HealthOK},
			},
			Workloads: []kube.Workload{
				{Kind: "Deployment", Name: "web", Ready: 0, Desired: 2, Rollout: kube.RolloutFailed, RolloutMessage: "deadline exceeded"},
				{Kind: "Deployment", Name: "db", Ready: 1, Desired: 1, Rollout: kube.RolloutComplete},
			},
			Pods: []kube.Pod{
				{Name: "web-1", Status: "CrashLoopBackOff", Ready: 0, Total: 1, Restarts: 7, Problem: "OOMKilled (exit 137) in app"},
				{Name: "db-0", Status: "Running", Healthy: true, Ready: 1, Total: 1},
			},
			Jobs:    []kube.Job{{Kind: "Job", Name: "migrate", Status: "failed (3)"}, {Kind: "Job", Name: "ok", Status: "complete"}},
			Events:  []kube.Event{{Kind: "Pod", Name: "web-1", Reason: "BackOff", Message: "Back-off restarting", Count: 12, LastSeen: "1m"}},
			Partial: []string{"ingresses"},
		},
	}
	ctx := context.Background()
	out, err := kubeStatus(ctx, k, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, w := range []string{
		"Context: dev (cluster kind-dev), namespace: shop", "Apps: 2 (1 ok, 0 warning, 1 failing)",
		"- web (helm, bad): 0/2 pods ready", "- Deployment/web: 0/2 ready, rollout failed deadline exceeded",
		"- web-1: CrashLoopBackOff, 0/1 ready, 7 restarts — OOMKilled", "- Job/migrate: failed (3)",
		"- Pod/web-1 BackOff (×12, 1m): Back-off restarting", "Not readable (permissions or missing CRDs): ingresses",
	} {
		if !strings.Contains(out, w) {
			t.Errorf("missing %q in:\n%s", w, out)
		}
	}
	if strings.Contains(out, "db-0") || strings.Contains(out, "Deployment/db") || strings.Contains(out, "Job/ok") {
		t.Errorf("healthy objects listed:\n%s", out)
	}
	if _, err := kubeStatus(ctx, k, "kube-system"); err != nil {
		t.Fatal(err)
	}
	if got := k.calls(); !slices.Equal(got, []string{"contexts", "overview dev/shop", "contexts", "overview dev/kube-system"}) {
		t.Errorf("asked %v", got)
	}
	if _, err := kubeStatus(ctx, k, "--all"); err == nil {
		t.Error("invalid namespace accepted")
	}
	k.contexts = []kube.Context{{Name: "x"}}
	if _, err := kubeStatus(ctx, k, ""); err == nil {
		t.Error("no current context accepted")
	}
}

func TestKubeStatusCutsLongLists(t *testing.T) {
	k := &fakeCluster{contexts: []kube.Context{{Name: "dev", Current: true}}}
	for range maxKubeItems + 5 {
		k.ov.Pods = append(k.ov.Pods, kube.Pod{Name: "p", Status: "Pending"})
	}
	out, err := kubeStatus(context.Background(), k, "")
	if err != nil || !strings.Contains(out, "- … and 5 more") || !strings.Contains(out, "namespace: default") {
		t.Errorf("%v\n%s", err, out)
	}
}
