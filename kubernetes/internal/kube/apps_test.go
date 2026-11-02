package kube

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

const appsCore = `{"items": [
 {"kind": "Deployment", "metadata": {"name": "shop", "generation": 4, "labels": {"kustomize.toolkit.fluxcd.io/name": "shop", "kustomize.toolkit.fluxcd.io/namespace": "flux-system"}, "annotations": {"deployment.kubernetes.io/revision": "7"}},
  "spec": {"replicas": 2, "selector": {"matchLabels": {"app": "shop"}}, "template": {"metadata": {"labels": {"app": "shop"}}, "spec": {"containers": [{"name": "app", "image": "acme/shop:1.4.0"}]}}},
  "status": {"observedGeneration": 4, "replicas": 3, "updatedReplicas": 2, "readyReplicas": 1, "availableReplicas": 1, "conditions": [{"type": "Progressing", "status": "True", "reason": "ReplicaSetUpdated"}]}},
 {"kind": "ReplicaSet", "metadata": {"name": "shop-new", "ownerReferences": [{"kind": "Deployment", "name": "shop"}], "annotations": {"deployment.kubernetes.io/revision": "7"}}},
 {"kind": "ReplicaSet", "metadata": {"name": "shop-old", "ownerReferences": [{"kind": "Deployment", "name": "shop"}], "annotations": {"deployment.kubernetes.io/revision": "6"}}},
 {"kind": "Pod", "metadata": {"name": "shop-new-a", "labels": {"app": "shop"}, "ownerReferences": [{"kind": "ReplicaSet", "name": "shop-new"}]},
  "spec": {"containers": [{"name": "app"}]},
  "status": {"phase": "Running", "containerStatuses": [{"name": "app", "ready": false, "restartCount": 3, "state": {"waiting": {"reason": "CrashLoopBackOff", "message": "back-off 40s restarting failed container"}}, "lastState": {"terminated": {"reason": "OOMKilled", "exitCode": 137}}}]}},
 {"kind": "Pod", "metadata": {"name": "shop-old-b", "labels": {"app": "shop"}, "ownerReferences": [{"kind": "ReplicaSet", "name": "shop-old"}]},
  "spec": {"containers": [{"name": "app"}]}, "status": {"phase": "Running", "containerStatuses": [{"name": "app", "ready": true, "state": {}}]}},
 {"kind": "Service", "metadata": {"name": "shop"}, "spec": {"type": "ClusterIP", "selector": {"app": "shop"}, "ports": [{"port": 80, "targetPort": 8080}]}},
 {"kind": "Service", "metadata": {"name": "orphan"}, "spec": {"type": "ClusterIP", "selector": {"app": "gone"}, "ports": [{"port": 80}]}},
 {"kind": "Deployment", "metadata": {"name": "worker", "generation": 2, "labels": {"app.kubernetes.io/instance": "billing"}},
  "spec": {"replicas": 1, "template": {"spec": {"containers": [{"name": "w", "image": "acme/worker:2"}]}}},
  "status": {"observedGeneration": 2, "replicas": 1, "updatedReplicas": 1, "conditions": [{"type": "Progressing", "status": "False", "reason": "ProgressDeadlineExceeded", "message": "ReplicaSet worker-9 has timed out progressing."}]}},
 {"kind": "Deployment", "metadata": {"name": "web", "generation": 1, "annotations": {"deployment.kubernetes.io/revision": "1"}},
  "spec": {"replicas": 1, "selector": {"matchLabels": {"app": "web"}}, "template": {"metadata": {"labels": {"app": "web"}}}},
  "status": {"observedGeneration": 1, "replicas": 1, "updatedReplicas": 1, "readyReplicas": 1, "availableReplicas": 1}},
 {"kind": "Service", "metadata": {"name": "web"}, "spec": {"selector": {"app": "web"}, "ports": [{"port": 80}]}},
 {"kind": "Pod", "metadata": {"name": "web-1", "labels": {"app": "web", "pod-template-hash": "x"}, "ownerReferences": [{"kind": "ReplicaSet", "name": "web-x"}]}, "spec": {"containers": [{"name": "c"}]}, "status": {"phase": "Running", "containerStatuses": [{"name": "c", "ready": true, "state": {}}]}}
]}`

const appsIngress = `{"items": [{"kind": "Ingress", "metadata": {"name": "shop"}, "spec": {"ingressClassName": "nginx", "tls": [{"hosts": ["shop.example.com"]}], "rules": [{"host": "shop.example.com", "http": {"paths": [{"path": "/", "backend": {"service": {"name": "shop", "port": {"number": 80}}}}]}}]}, "status": {"loadBalancer": {"ingress": [{"hostname": "lb.example.com"}]}}}]}`

const appsKustomizations = `{"items": [{"kind": "Kustomization", "metadata": {"name": "shop", "namespace": "flux-system"}, "spec": {}, "status": {"lastAppliedRevision": "main@sha1:abc123", "conditions": [{"type": "Ready", "status": "False", "reason": "HealthCheckFailed", "message": "health check failed after 5m: Deployment/shop not ready"}]}}]}`

const appsEvents = `{"items": [
 {"involvedObject": {"kind": "Pod", "name": "shop-new-a"}, "reason": "BackOff", "message": "Back-off restarting failed container app", "count": 9, "lastTimestamp": "2026-10-31T09:55:00Z"},
 {"involvedObject": {"kind": "Pod", "name": "shop-new-a"}, "reason": "BackOff", "message": "older duplicate", "count": 1, "lastTimestamp": "2026-10-31T09:50:00Z"},
 {"involvedObject": {"kind": "Pod", "name": "web-1"}, "reason": "Unhealthy", "message": "stale", "lastTimestamp": "2026-10-31T07:00:00Z"}
]}`

func appsFixture(t *testing.T) Overview {
	t.Helper()
	core, err := parseItems([]byte(appsCore))
	if err != nil {
		t.Fatal(err)
	}
	ing, _ := parseItems([]byte(appsIngress))
	ks, _ := parseItems([]byte(appsKustomizations))
	var ev struct {
		Items []eventRaw `json:"items"`
	}
	if err := json.Unmarshal([]byte(appsEvents), &ev); err != nil {
		t.Fatal(err)
	}
	return buildOverview(core, ing, ev.Items, ks, time.Date(2026, 10, 31, 10, 0, 0, 0, time.UTC))
}

func TestRolloutAndPodProblems(t *testing.T) {
	ov := appsFixture(t)
	w := map[string]Workload{}
	for _, x := range ov.Workloads {
		w[x.Name] = x
	}
	if w["shop"].Rollout != RolloutProgressing || w["shop"].RolloutMessage != "1 old replicas pending termination" || w["shop"].Revision != "7" || w["shop"].Images[0] != "acme/shop:1.4.0" {
		t.Errorf("shop %+v", w["shop"])
	}
	if w["worker"].Rollout != RolloutFailed || !strings.Contains(w["worker"].RolloutMessage, "timed out") {
		t.Errorf("worker %+v", w["worker"])
	}
	if w["web"].Rollout != RolloutComplete {
		t.Errorf("web %+v", w["web"])
	}
	pods := map[string]Pod{}
	for _, p := range ov.Pods {
		pods[p.Name] = p
	}
	if p := pods["shop-new-a"]; p.Owner != "shop" || p.Old || p.Problem != "app: CrashLoopBackOff, last OOMKilled (exit 137): back-off 40s restarting failed container" {
		t.Errorf("new pod %+v", p)
	}
	if p := pods["shop-old-b"]; !p.Old || p.Owner != "shop" {
		t.Errorf("old pod %+v", p)
	}
	if p := pods["web-1"]; p.Owner != "web" {
		t.Errorf("web pod without listed ReplicaSet %+v", p)
	}
}

func TestGroupApps(t *testing.T) {
	ov := appsFixture(t)
	apps := map[string]App{}
	var order []string
	for _, a := range ov.Apps {
		apps[a.Key] = a
		order = append(order, a.Key)
	}
	shop := apps["flux:flux-system/shop"]
	if shop.Source != "flux" || shop.Kustomization == nil || shop.Kustomization.Revision != "main@sha1:abc123" {
		t.Fatalf("shop %+v", shop)
	}
	if strings.Join(shop.Ingresses, ",") != "shop" || strings.Join(shop.Services, ",") != "shop" || strings.Join(shop.Workloads, ",") != "Deployment/shop" {
		t.Errorf("shop members %+v", shop)
	}
	if shop.Health != HealthBad || shop.Critical != 2 || !strings.HasPrefix(shop.Problems[0], "Flux: health check failed") {
		t.Errorf("shop health %s %q", shop.Health, shop.Problems)
	}
	if !strings.Contains(strings.Join(shop.Problems, "\n"), "Pod shop-new-a: BackOff") {
		t.Errorf("events not attached: %q", shop.Problems)
	}
	if strings.Contains(strings.Join(shop.Problems, "\n"), "older duplicate") {
		t.Errorf("duplicate event kept: %q", shop.Problems)
	}
	if b := apps["label:billing"]; b.Health != HealthBad || !strings.Contains(b.Problems[0], "rollout failed") {
		t.Errorf("billing %+v", b)
	}
	if web := apps["workload:Deployment/web"]; web.Health != HealthOK || strings.Join(web.Services, ",") != "web" {
		t.Errorf("web %+v", web)
	}
	if o := apps["service:orphan"]; o.Health != HealthBad || o.Problems[0] != "service orphan has no ready pods" {
		t.Errorf("orphan %+v", o)
	}
	if order[len(order)-1] != "workload:Deployment/web" {
		t.Errorf("healthy apps not last: %q", order)
	}
	if len(ov.Ingresses) != 1 || ov.Ingresses[0].Address != "lb.example.com" || !ov.Ingresses[0].TLS || ov.Ingresses[0].Routes[0].Port != "80" {
		t.Errorf("ingress %+v", ov.Ingresses)
	}
	for _, s := range ov.Services {
		if s.Name == "shop" && (s.Backends != 2 || s.Ready != 1) {
			t.Errorf("shop endpoints %+v", s)
		}
	}
}
