package kube

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const configJSON = `{
  "current-context": "docker-desktop",
  "contexts": [
    {"name": "arn:aws:eks:eu-central-1:111:cluster/prod", "context": {"cluster": "arn:aws:eks:eu-central-1:111:cluster/prod", "user": "eks-prod", "namespace": "apps"}},
    {"name": "docker-desktop", "context": {"cluster": "docker-desktop", "user": "dd"}},
    {"name": "legacy", "context": {"cluster": "c", "user": "iam"}}
  ],
  "users": [
    {"name": "eks-prod", "user": {"exec": {"command": "aws", "args": ["eks", "get-token", "--cluster-name", "prod"], "env": [{"name": "AWS_PROFILE", "value": "prod-admin"}]}}},
    {"name": "dd", "user": {"client-certificate-data": "DATA+OMITTED", "client-key-data": "DATA+OMITTED"}},
    {"name": "iam", "user": {"exec": {"command": "/usr/local/bin/aws-iam-authenticator", "args": ["token", "-i", "c", "--profile", "legacy-profile"]}}}
  ]
}`

func TestParseContexts(t *testing.T) {
	got, err := parseContexts([]byte(configJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].Name != "docker-desktop" || !got[0].Current || got[0].Auth != "certificate" {
		t.Fatalf("contexts %+v", got)
	}
	eks := got[1]
	if eks.Auth != "exec:aws" || eks.AWSProfile != "prod-admin" || eks.Namespace != "apps" {
		t.Errorf("eks %+v", eks)
	}
	if got[2].Auth != "exec:aws-iam-authenticator" || got[2].AWSProfile != "legacy-profile" {
		t.Errorf("legacy %+v", got[2])
	}
	if strings.Contains(strings.ToLower(eks.Name+eks.User+eks.Auth), "data") {
		t.Error("credential data in output")
	}
}

const overviewJSON = `{"items": [
  {"kind": "Deployment", "metadata": {"name": "api", "creationTimestamp": "2026-10-30T10:00:00Z"}, "spec": {"replicas": 3}, "status": {"readyReplicas": 2, "updatedReplicas": 3}},
  {"kind": "DaemonSet", "metadata": {"name": "agent"}, "status": {"desiredNumberScheduled": 2, "numberReady": 2}},
  {"kind": "Pod", "metadata": {"name": "api-7d9f-abcde", "labels": {"pod-template-hash": "7d9f"}, "ownerReferences": [{"kind": "ReplicaSet", "name": "api-7d9f"}]},
   "spec": {"nodeName": "n1", "containers": [{"name": "app", "ports": [{"containerPort": 8080}]}, {"name": "sidecar"}]},
   "status": {"phase": "Running", "containerStatuses": [{"ready": true, "restartCount": 1, "state": {}}, {"ready": false, "restartCount": 4, "state": {"waiting": {"reason": "CrashLoopBackOff"}}}]}},
  {"kind": "Pod", "metadata": {"name": "ok", "deletionTimestamp": ""}, "spec": {"containers": [{"name": "a"}]}, "status": {"phase": "Running", "containerStatuses": [{"ready": true, "state": {}}]}},
  {"kind": "Pod", "metadata": {"name": "migrate", "deletionTimestamp": "2026-10-31T10:00:00Z"}, "spec": {"containers": [{"name": "a"}]}, "status": {"phase": "Running"}},
  {"kind": "Service", "metadata": {"name": "api"}, "spec": {"type": "ClusterIP", "clusterIP": "10.0.0.1", "ports": [{"port": 80, "targetPort": 8080, "protocol": "TCP"}, {"port": 443, "targetPort": "https", "protocol": "TCP"}]}},
  {"kind": "Job", "metadata": {"name": "nightly-123", "ownerReferences": [{"kind": "CronJob", "name": "nightly"}]}, "status": {"succeeded": 1}},
  {"kind": "Job", "metadata": {"name": "seed"}, "spec": {"completions": 1}, "status": {"failed": 2}},
  {"kind": "CronJob", "metadata": {"name": "nightly"}, "spec": {"schedule": "0 3 * * *", "suspend": true}}
]}`

func TestParseOverview(t *testing.T) {
	ov, err := parseOverview([]byte(overviewJSON))
	if err != nil {
		t.Fatal(err)
	}
	if len(ov.Workloads) != 2 || ov.Workloads[1].Name != "api" || ov.Workloads[1].Ready != 2 || ov.Workloads[1].Desired != 3 {
		t.Errorf("workloads %+v", ov.Workloads)
	}
	pods := map[string]Pod{}
	for _, p := range ov.Pods {
		pods[p.Name] = p
	}
	crash := pods["api-7d9f-abcde"]
	if crash.Status != "CrashLoopBackOff" || crash.Healthy || crash.Ready != 1 || crash.Total != 2 || crash.Restarts != 5 || crash.Owner != "api" || len(crash.Ports) != 1 {
		t.Errorf("crashing pod %+v", crash)
	}
	if !pods["ok"].Healthy || pods["migrate"].Status != "Terminating" {
		t.Errorf("pods %+v", pods)
	}
	if len(ov.Services) != 1 || ov.Services[0].Ports[0].Target != "8080" || ov.Services[0].Ports[1].Target != "https" {
		t.Errorf("services %+v", ov.Services)
	}
	if len(ov.Jobs) != 2 || ov.Jobs[0].Status != "suspended" || ov.Jobs[1].Status != "failed (2)" {
		t.Errorf("jobs %+v", ov.Jobs)
	}
}

func fakeKubectl(t *testing.T) (*Kubectl, string) {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "args.log")
	bin := filepath.Join(dir, "kubectl")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho \"$@\" >> "+log+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &Kubectl{bin: bin}, log
}

func TestActionsBuildExactCommands(t *testing.T) {
	k, log := fakeKubectl(t)
	ctx, kc := t.Context(), "arn:aws:eks:eu-central-1:111:cluster/prod"
	for _, err := range []error{
		k.RolloutRestart(ctx, kc, "apps", "Deployment", "api"),
		k.Scale(ctx, kc, "apps", "StatefulSet", "db", 0),
		k.DeletePod(ctx, kc, "apps", "api-7d9f-abcde"),
		k.UseContext(ctx, kc),
		k.RolloutUndo(ctx, kc, "apps", "Deployment", "api"),
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	b, _ := os.ReadFile(log)
	want := "--context=" + kc + " --request-timeout=15s --namespace=apps rollout restart Deployment/api\n" +
		"--context=" + kc + " --request-timeout=15s --namespace=apps scale StatefulSet/db --replicas=0\n" +
		"--context=" + kc + " --request-timeout=15s --namespace=apps delete pod api-7d9f-abcde --wait=false\n" +
		"config use-context " + kc + "\n" +
		"--context=" + kc + " --request-timeout=15s --namespace=apps rollout undo Deployment/api\n"
	if string(b) != want {
		t.Fatalf("calls:\n%s\nwant:\n%s", b, want)
	}
}

func TestRejectsUnsafeInput(t *testing.T) {
	k, log := fakeKubectl(t)
	ctx := t.Context()
	for _, err := range []error{
		k.RolloutRestart(ctx, "--kubeconfig=/tmp/x", "apps", "Deployment", "api"),
		k.RolloutRestart(ctx, "c", "apps", "Pod", "api"),
		k.Scale(ctx, "c", "apps", "Deployment", "api", 5000),
		k.DeletePod(ctx, "c", "apps", "--all"),
		k.DeletePod(ctx, "c", "--all-namespaces", "x"),
		k.UseContext(ctx, "-x"),
	} {
		if err == nil {
			t.Error("unsafe input accepted")
		}
	}
	if b, _ := os.ReadFile(log); len(b) > 0 {
		t.Fatalf("kubectl was called: %s", b)
	}
}
