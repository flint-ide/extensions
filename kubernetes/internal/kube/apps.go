package kube

import (
	"slices"
	"strings"
	"time"
)

// App is a group of objects deployed together: a Flux Kustomization, an Argo
// CD application, a Helm release, an app.kubernetes.io label, or one workload.
type App struct {
	Key    string `json:"key"`
	Name   string `json:"name"`
	Source string `json:"source"` // flux, argo, helm, label, workload
	// Kustomization of a Flux app (may live in flux-system).
	Kustomization *Kustomization `json:"kustomization,omitempty"`
	Ingresses     []string       `json:"ingresses"`
	Services      []string       `json:"services"`
	Workloads     []string       `json:"workloads"` // "Kind/name"
	Health        string         `json:"health"`    // ok, warn, bad
	// Problems summarise what is wrong: the first Critical of them break the
	// app, the rest are warnings.
	Problems []string `json:"problems"`
	Critical int      `json:"critical"`
}

// Health of apps and their parts.
const (
	HealthOK   = "ok"
	HealthWarn = "warn"
	HealthBad  = "bad"
)

// appKey reads which app an object belongs to from the labels deploy tools set.
func appKey(labels map[string]string, annotations map[string]string) (key, name, source string) {
	if n := labels["kustomize.toolkit.fluxcd.io/name"]; n != "" {
		ns := labels["kustomize.toolkit.fluxcd.io/namespace"]
		return "flux:" + ns + "/" + n, n, "flux"
	}
	if n := labels["argocd.argoproj.io/instance"]; n != "" {
		return "argo:" + n, n, "argo"
	}
	if n := annotations["meta.helm.sh/release-name"]; n != "" {
		return "helm:" + n, n, "helm"
	}
	for _, l := range []string{"app.kubernetes.io/instance", "app.kubernetes.io/name", "app"} {
		if n := labels[l]; n != "" {
			return "label:" + n, n, "label"
		}
	}
	return "", "", ""
}

// groupApps puts workloads, services and ingresses into apps: by deploy-tool
// labels first, then a Service joins the app of the workload its selector
// picks, and an Ingress the app of its backend Service.
func groupApps(ov *Overview) []App {
	apps := map[string]*App{}
	var order []string
	get := func(key, name, source string) *App {
		if a, ok := apps[key]; ok {
			return a
		}
		a := &App{Key: key, Name: name, Source: source, Ingresses: []string{}, Services: []string{}, Workloads: []string{}, Problems: []string{}}
		apps[key] = a
		order = append(order, key)
		return a
	}
	for i := range ov.Workloads {
		w := &ov.Workloads[i]
		key, name, source := appKey(w.labels, nil)
		if key == "" {
			key, name, source = "workload:"+w.Kind+"/"+w.Name, w.Name, "workload"
		}
		w.App = key
		a := get(key, name, source)
		a.Workloads = append(a.Workloads, w.Kind+"/"+w.Name)
	}
	serviceApp := map[string]string{}
	for i := range ov.Services {
		s := &ov.Services[i]
		key, name, source := appKey(s.labels, nil)
		if key == "" {
			for _, w := range ov.Workloads {
				if matchesSelector(s.selector, w.templateLabels) {
					key = w.App
					break
				}
			}
		}
		if key == "" {
			key, name, source = "service:"+s.Name, s.Name, "workload"
		}
		s.App, serviceApp[s.Name] = key, key
		a := get(key, name, source)
		a.Services = append(a.Services, s.Name)
	}
	for i := range ov.Ingresses {
		in := &ov.Ingresses[i]
		key, name, source := appKey(in.labels, nil)
		if key == "" {
			for _, r := range in.Routes {
				if k := serviceApp[r.Service]; k != "" {
					key = k
					break
				}
			}
		}
		if key == "" {
			key, name, source = "ingress:"+in.Name, in.Name, "workload"
		}
		in.App = key
		a := get(key, name, source)
		a.Ingresses = append(a.Ingresses, in.Name)
	}
	for i := range ov.Kustomizations {
		k := &ov.Kustomizations[i]
		if a, ok := apps["flux:"+k.Namespace+"/"+k.Name]; ok {
			a.Kustomization = k
		}
	}
	out := make([]App, 0, len(order))
	for _, key := range order {
		a := apps[key]
		judge(a, ov)
		out = append(out, *a)
	}
	rank := map[string]int{HealthBad: 0, HealthWarn: 1, HealthOK: 2}
	slices.SortStableFunc(out, func(a, b App) int {
		if d := rank[a.Health] - rank[b.Health]; d != 0 {
			return d
		}
		return strings.Compare(a.Name, b.Name)
	})
	return out
}

// judge sets an app's health and problems from its rollouts, pods, services,
// Kustomization and warning events.
func judge(a *App, ov *Overview) {
	bad, warn := []string{}, []string{}
	if k := a.Kustomization; k != nil {
		switch {
		case k.Suspended:
			warn = append(warn, "Flux Kustomization is suspended")
		case k.Ready == "False":
			bad = append(bad, "Flux: "+firstNonEmpty(k.Message, k.Reason))
		case k.Ready != "True":
			warn = append(warn, "Flux: reconciling")
		}
	}
	for _, ref := range a.Workloads {
		kind, name, _ := strings.Cut(ref, "/")
		for _, w := range ov.Workloads {
			if w.Kind != kind || w.Name != name {
				continue
			}
			switch w.Rollout {
			case RolloutFailed:
				bad = append(bad, w.Name+": rollout failed: "+w.RolloutMessage)
			case RolloutProgressing:
				warn = append(warn, w.Name+": rolling out ("+w.RolloutMessage+")")
			case RolloutPaused:
				warn = append(warn, w.Name+": rollout paused")
			}
			for _, p := range ov.Pods {
				if p.Owner != w.Name || p.Healthy || p.Status == "Terminating" {
					continue
				}
				msg := p.Name + ": " + firstNonEmpty(p.Problem, p.Status)
				if crashing(p.Status) {
					bad = append(bad, msg)
				} else {
					warn = append(warn, msg)
				}
			}
		}
	}
	for _, name := range a.Services {
		for _, s := range ov.Services {
			if s.Name == name && len(s.selector) > 0 && s.Ready == 0 && s.Type != "ExternalName" {
				bad = append(bad, "service "+s.Name+" has no ready pods")
			}
		}
	}
	for _, e := range ov.Events {
		if a.owns(e.Kind, e.Name, ov) {
			warn = append(warn, e.Kind+" "+e.Name+": "+e.Reason+": "+truncate(e.Message, 200))
		}
	}
	a.Problems, a.Critical = append(bad, warn...), len(bad)
	switch {
	case len(bad) > 0:
		a.Health = HealthBad
	case len(warn) > 0:
		a.Health = HealthWarn
	default:
		a.Health = HealthOK
	}
}

// owns: the object (a workload, its pods or ReplicaSets, a service or ingress) is part of the app.
func (a *App) owns(kind, name string, ov *Overview) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		return slices.Contains(a.Workloads, kind+"/"+name)
	case "Service":
		return slices.Contains(a.Services, name)
	case "Ingress":
		return slices.Contains(a.Ingresses, name)
	case "ReplicaSet":
		for _, ref := range a.Workloads {
			if d, ok := strings.CutPrefix(ref, "Deployment/"); ok && strings.HasPrefix(name, d+"-") {
				return true
			}
		}
	case "Pod":
		for _, p := range ov.Pods {
			if p.Name == name {
				for _, ref := range a.Workloads {
					if strings.HasSuffix(ref, "/"+p.Owner) {
						return true
					}
				}
			}
		}
	}
	return false
}

// crashing statuses mean the pod won't recover by waiting.
func crashing(status string) bool {
	for _, s := range []string{"BackOff", "Err", "Error", "OOMKilled", "Unschedulable", "Invalid", "Failed", "Evicted"} {
		if strings.Contains(status, s) {
			return true
		}
	}
	return false
}

func firstNonEmpty(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// eventRaw is a core/v1 Event.
type eventRaw struct {
	InvolvedObject struct {
		Kind string `json:"kind"`
		Name string `json:"name"`
	} `json:"involvedObject"`
	Reason         string `json:"reason"`
	Message        string `json:"message"`
	Count          int    `json:"count"`
	LastTimestamp  string `json:"lastTimestamp"`
	EventTime      string `json:"eventTime"`
	FirstTimestamp string `json:"firstTimestamp"`
}

// eventWindow: older warnings are history, not the current problem.
const eventWindow = time.Hour

// warningEvents keeps the last hour's warnings, newest first, merged per
// object and reason.
func warningEvents(items []eventRaw, now time.Time) []Event {
	out := []Event{}
	for _, e := range items {
		seen := firstNonEmpty(e.LastTimestamp, e.EventTime, e.FirstTimestamp)
		t, err := time.Parse(time.RFC3339Nano, seen)
		if err != nil || now.Sub(t) > eventWindow {
			continue
		}
		out = append(out, Event{Kind: e.InvolvedObject.Kind, Name: e.InvolvedObject.Name, Reason: e.Reason, Message: truncate(e.Message, 500), Count: max(e.Count, 1), LastSeen: t.UTC().Format(time.RFC3339)})
	}
	slices.SortFunc(out, func(a, b Event) int { return strings.Compare(b.LastSeen, a.LastSeen) })
	merged := out[:0]
	seen := map[string]bool{}
	for _, e := range out {
		key := e.Kind + "/" + e.Name + "/" + e.Reason
		if !seen[key] {
			seen[key] = true
			merged = append(merged, e)
		}
	}
	return merged[:min(len(merged), maxEvents)]
}

func kustomization(o object) Kustomization {
	k := Kustomization{Name: o.Metadata.Name, Namespace: o.Metadata.Namespace, Ready: "Unknown", Revision: o.Status.LastAppliedRevision}
	if o.Spec.Suspend != nil {
		k.Suspended = *o.Spec.Suspend
	}
	if c, ok := conditionOf(o.Status.Conditions, "Ready"); ok {
		k.Ready, k.Reason, k.Message = c.Status, c.Reason, truncate(c.Message, 500)
	}
	return k
}
