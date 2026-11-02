package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Overview is what the Kubernetes panel shows for one namespace.
type Overview struct {
	Workloads      []Workload      `json:"workloads"`
	Pods           []Pod           `json:"pods"`
	Services       []Service       `json:"services"`
	Ingresses      []Ingress       `json:"ingresses"`
	Jobs           []Job           `json:"jobs"`
	Events         []Event         `json:"events"`
	Kustomizations []Kustomization `json:"kustomizations"`
	Apps           []App           `json:"apps"`
	// Partial lists what couldn't be read (RBAC, missing CRDs) besides the core kinds.
	Partial []string `json:"partial"`
}

// Rollout phases, as `kubectl rollout status` would report them.
const (
	RolloutComplete    = "complete"
	RolloutProgressing = "progressing"
	RolloutFailed      = "failed"
	RolloutPaused      = "paused"
	RolloutScaledDown  = "scaled-down"
)

// Workload is a Deployment, StatefulSet or DaemonSet with its rollout state.
type Workload struct {
	Kind      string   `json:"kind"`
	Name      string   `json:"name"`
	Ready     int      `json:"ready"`
	Desired   int      `json:"desired"`
	Updated   int      `json:"updated"`
	Available int      `json:"available"`
	Created   string   `json:"created"`
	Images    []string `json:"images"`
	Revision  string   `json:"revision,omitempty"`
	Rollout   string   `json:"rollout"`
	// RolloutMessage explains a rollout that isn't complete.
	RolloutMessage string `json:"rolloutMessage,omitempty"`
	App            string `json:"app"`

	labels         map[string]string
	selector       map[string]string
	templateLabels map[string]string
}

// Pod is a pod with the status kubectl get pods shows and what's wrong with it.
type Pod struct {
	Name       string   `json:"name"`
	Status     string   `json:"status"` // Running, Pending, CrashLoopBackOff, Completed, Terminating…
	Healthy    bool     `json:"healthy"`
	Ready      int      `json:"ready"`
	Total      int      `json:"total"`
	Restarts   int      `json:"restarts"`
	Node       string   `json:"node,omitempty"`
	Owner      string   `json:"owner,omitempty"` // workload name (Deployment for ReplicaSet pods)
	OwnerKind  string   `json:"ownerKind,omitempty"`
	Containers []string `json:"containers"`
	Ports      []int    `json:"ports"`
	Created    string   `json:"created"`
	// Problem explains an unhealthy pod: "OOMKilled (exit 137) in app", the
	// image pull error, a container not ready…
	Problem string `json:"problem,omitempty"`
	// Old: from a previous revision of its Deployment (a rollout is replacing it).
	Old bool `json:"old,omitempty"`

	labels map[string]string
}

// Service is a Service with its ports and how many ready pods back it.
type Service struct {
	Name      string        `json:"name"`
	Type      string        `json:"type"`
	ClusterIP string        `json:"clusterIP"`
	Ports     []ServicePort `json:"ports"`
	// Ready / Backends: pods matching the selector, and those ready.
	Ready    int    `json:"ready"`
	Backends int    `json:"backends"`
	App      string `json:"app"`

	labels   map[string]string
	selector map[string]string
}

// ServicePort is one port of a Service.
type ServicePort struct {
	Port     int    `json:"port"`
	Target   string `json:"target"`
	Protocol string `json:"protocol"`
	NodePort int    `json:"nodePort,omitempty"`
}

// Ingress routes hosts and paths to Services.
type Ingress struct {
	Name    string         `json:"name"`
	Class   string         `json:"class,omitempty"`
	Routes  []IngressRoute `json:"routes"`
	TLS     bool           `json:"tls"`
	Address string         `json:"address,omitempty"`
	App     string         `json:"app"`

	labels map[string]string
}

// IngressRoute is one host/path → service:port rule.
type IngressRoute struct {
	Host    string `json:"host"`
	Path    string `json:"path"`
	Service string `json:"service"`
	Port    string `json:"port"`
}

// Job is a Job or a CronJob.
type Job struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Schedule string `json:"schedule,omitempty"`
	Created  string `json:"created"`
}

// Event is a recent Warning event about an object of the namespace.
type Event struct {
	Kind     string `json:"kind"`
	Name     string `json:"name"`
	Reason   string `json:"reason"`
	Message  string `json:"message"`
	Count    int    `json:"count"`
	LastSeen string `json:"lastSeen"`
}

// Kustomization is a Flux kustomize.toolkit.fluxcd.io Kustomization.
type Kustomization struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     string `json:"ready"` // True, False, Unknown
	Reason    string `json:"reason,omitempty"`
	Message   string `json:"message,omitempty"`
	Revision  string `json:"revision,omitempty"`
	Suspended bool   `json:"suspended"`
}

// Kinds listed in one call (a namespace's usual RBAC allows all of them);
// ingresses, events and Flux Kustomizations are optional extras.
const (
	coreKinds = "deployments,statefulsets,daemonsets,replicasets,pods,services,jobs,cronjobs"
	maxEvents = 200
)

// Overview lists a namespace's objects, recent warnings and Flux state, and
// groups them into apps. Optional parts that fail are named in Partial.
func (k *Kubectl) Overview(ctx context.Context, kubeContext, namespace string) (Overview, error) {
	args, err := scope(kubeContext, namespace)
	if err != nil {
		return Overview{}, err
	}
	allArgs, _ := scope(kubeContext, "")
	type result struct {
		out []byte
		err error
	}
	var wg sync.WaitGroup
	results := make([]result, 4)
	calls := [][]string{
		append(slices.Clone(args), "get", coreKinds, "-o", "json"),
		append(slices.Clone(args), "get", "ingresses", "-o", "json"),
		append(slices.Clone(args), "get", "events", "--field-selector=type=Warning", "-o", "json"),
		append(slices.Clone(allArgs), "get", "kustomizations.kustomize.toolkit.fluxcd.io", "--all-namespaces", "-o", "json"),
	}
	for i, call := range calls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, err := k.run(ctx, readTimeout, call...)
			results[i] = result{out, err}
		}()
	}
	wg.Wait()
	if results[0].err != nil {
		return Overview{}, results[0].err
	}
	core, err := parseItems(results[0].out)
	if err != nil {
		return Overview{}, err
	}
	var partial []string
	optional := func(i int, name string) []object {
		if results[i].err == nil {
			if items, err := parseItems(results[i].out); err == nil {
				return items
			}
		}
		// Ingresses or Flux may not exist or be forbidden; that only hides them.
		if !(i == 3 && strings.Contains(fmt.Sprint(results[i].err), "the server doesn't have a resource type")) {
			partial = append(partial, name)
		}
		return nil
	}
	ingresses := optional(1, "ingresses")
	kustomizations := optional(3, "Flux Kustomizations")
	var events struct {
		Items []eventRaw `json:"items"`
	}
	if results[2].err != nil || json.Unmarshal(results[2].out, &events) != nil {
		partial = append(partial, "events")
	}
	ov := buildOverview(core, ingresses, events.Items, kustomizations, time.Now())
	if ov.Partial = partial; ov.Partial == nil {
		ov.Partial = []string{}
	}
	return ov, nil
}

func parseItems(data []byte) ([]object, error) {
	var list struct {
		Items []object `json:"items"`
	}
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("unexpected kubectl output: %w", err)
	}
	return list.Items, nil
}

func parseOverview(data []byte) (Overview, error) {
	items, err := parseItems(data)
	if err != nil {
		return Overview{}, err
	}
	return buildOverview(items, nil, nil, nil, time.Now()), nil
}

// buildOverview turns raw objects into the panel's model and groups apps.
func buildOverview(core, ingresses []object, events []eventRaw, kustomizations []object, now time.Time) Overview {
	ov := Overview{
		Workloads: []Workload{}, Pods: []Pod{}, Services: []Service{}, Ingresses: []Ingress{}, Jobs: []Job{},
		Events: []Event{}, Kustomizations: []Kustomization{}, Apps: []App{}, Partial: []string{},
	}
	// ReplicaSet → (Deployment, revision), to place pods and spot old ones.
	type rsInfo struct{ owner, revision string }
	replicaSets := map[string]rsInfo{}
	for _, o := range core {
		if o.Kind == "ReplicaSet" && len(o.Metadata.OwnerReferences) > 0 && o.Metadata.OwnerReferences[0].Kind == "Deployment" {
			replicaSets[o.Metadata.Name] = rsInfo{o.Metadata.OwnerReferences[0].Name, o.Metadata.Annotations["deployment.kubernetes.io/revision"]}
		}
	}
	revisions := map[string]string{}
	for _, o := range core {
		switch o.Kind {
		case "Deployment", "StatefulSet", "DaemonSet":
			w := workload(o)
			revisions[w.Name] = w.Revision
			ov.Workloads = append(ov.Workloads, w)
		case "Service":
			ov.Services = append(ov.Services, service(o))
		case "Job":
			if len(o.Metadata.OwnerReferences) > 0 && o.Metadata.OwnerReferences[0].Kind == "CronJob" {
				continue // runs of a CronJob: shown as the CronJob
			}
			ov.Jobs = append(ov.Jobs, Job{Kind: "Job", Name: o.Metadata.Name, Status: jobStatus(o), Created: o.Metadata.CreationTimestamp})
		case "CronJob":
			status := "scheduled"
			if o.Spec.Suspend != nil && *o.Spec.Suspend {
				status = "suspended"
			}
			ov.Jobs = append(ov.Jobs, Job{Kind: "CronJob", Name: o.Metadata.Name, Status: status, Schedule: o.Spec.Schedule, Created: o.Metadata.CreationTimestamp})
		}
	}
	for _, o := range core {
		if o.Kind != "Pod" {
			continue
		}
		p := pod(o)
		if len(o.Metadata.OwnerReferences) > 0 && o.Metadata.OwnerReferences[0].Kind == "ReplicaSet" {
			if rs, ok := replicaSets[o.Metadata.OwnerReferences[0].Name]; ok {
				p.Owner, p.OwnerKind = rs.owner, "Deployment"
				p.Old = rs.revision != "" && revisions[rs.owner] != "" && rs.revision != revisions[rs.owner]
			} else if hash := o.Metadata.Labels["pod-template-hash"]; hash != "" {
				// ReplicaSet not listed (RBAC): its name is <deployment>-<hash>.
				p.Owner, p.OwnerKind = strings.TrimSuffix(p.Owner, "-"+hash), "Deployment"
			}
		}
		ov.Pods = append(ov.Pods, p)
	}
	for i := range ov.Services {
		s := &ov.Services[i]
		for _, p := range ov.Pods {
			if matchesSelector(s.selector, p.labels) && p.Status != "Terminating" {
				s.Backends++
				if p.Ready == p.Total && p.Total > 0 {
					s.Ready++
				}
			}
		}
	}
	for _, o := range ingresses {
		ov.Ingresses = append(ov.Ingresses, ingress(o))
	}
	for _, o := range kustomizations {
		ov.Kustomizations = append(ov.Kustomizations, kustomization(o))
	}
	ov.Events = warningEvents(events, now)
	slices.SortFunc(ov.Workloads, func(a, b Workload) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(ov.Pods, func(a, b Pod) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(ov.Services, func(a, b Service) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(ov.Ingresses, func(a, b Ingress) int { return strings.Compare(a.Name, b.Name) })
	slices.SortFunc(ov.Jobs, func(a, b Job) int { return strings.Compare(a.Name, b.Name) })
	ov.Apps = groupApps(&ov)
	return ov
}

func workload(o object) Workload {
	w := Workload{
		Kind: o.Kind, Name: o.Metadata.Name, Created: o.Metadata.CreationTimestamp, Images: []string{},
		labels: o.Metadata.Labels, selector: selectorLabels(o.Spec.Selector, o.Kind), templateLabels: o.Spec.Template.Metadata.Labels,
	}
	for _, c := range o.Spec.Template.Spec.Containers {
		w.Images = append(w.Images, c.Image)
	}
	st := o.Status
	switch o.Kind {
	case "DaemonSet":
		w.Desired, w.Ready, w.Updated, w.Available = st.DesiredNumberScheduled, st.NumberReady, st.UpdatedNumberScheduled, st.NumberAvailable
	default:
		w.Desired = 1
		if o.Spec.Replicas != nil {
			w.Desired = *o.Spec.Replicas
		}
		w.Ready, w.Updated, w.Available = st.ReadyReplicas, st.UpdatedReplicas, st.AvailableReplicas
	}
	w.Revision = o.Metadata.Annotations["deployment.kubernetes.io/revision"]
	if o.Kind == "StatefulSet" && st.UpdateRevision != "" {
		w.Revision = st.UpdateRevision[strings.LastIndexByte(st.UpdateRevision, '-')+1:]
	}
	w.Rollout, w.RolloutMessage = rollout(o, w)
	return w
}

// rollout mirrors kubectl rollout status for the three workload kinds.
func rollout(o object, w Workload) (string, string) {
	st := o.Status
	if o.Spec.Paused {
		return RolloutPaused, "rollout paused"
	}
	if o.Metadata.Generation > st.ObservedGeneration {
		return RolloutProgressing, "waiting for the controller to see the new spec"
	}
	if c, ok := conditionOf(st.Conditions, "Progressing"); ok && c.Reason == "ProgressDeadlineExceeded" {
		return RolloutFailed, c.Message
	}
	if w.Desired == 0 && o.Kind != "DaemonSet" {
		return RolloutScaledDown, ""
	}
	switch o.Kind {
	case "Deployment":
		switch {
		case w.Updated < w.Desired:
			return RolloutProgressing, fmt.Sprintf("%d of %d new replicas updated", w.Updated, w.Desired)
		case st.Replicas > w.Updated:
			return RolloutProgressing, fmt.Sprintf("%d old replicas pending termination", st.Replicas-w.Updated)
		case w.Available < w.Updated:
			return RolloutProgressing, fmt.Sprintf("%d of %d updated replicas available", w.Available, w.Updated)
		}
	case "StatefulSet":
		switch {
		case st.UpdateRevision != "" && st.CurrentRevision != st.UpdateRevision:
			return RolloutProgressing, fmt.Sprintf("%d of %d pods updated", w.Updated, w.Desired)
		case w.Ready < w.Desired:
			return RolloutProgressing, fmt.Sprintf("%d of %d pods ready", w.Ready, w.Desired)
		}
	case "DaemonSet":
		switch {
		case w.Updated < w.Desired:
			return RolloutProgressing, fmt.Sprintf("%d of %d updated pods scheduled", w.Updated, w.Desired)
		case w.Available < w.Desired:
			return RolloutProgressing, fmt.Sprintf("%d of %d updated pods available", w.Available, w.Desired)
		}
	}
	return RolloutComplete, ""
}

// pod derives the status the way kubectl get pods prints it, and the problem.
func pod(o object) Pod {
	p := Pod{Name: o.Metadata.Name, Status: o.Status.Phase, Node: o.Spec.NodeName, Created: o.Metadata.CreationTimestamp, Containers: []string{}, Ports: []int{}, labels: o.Metadata.Labels}
	for _, c := range o.Spec.Containers {
		p.Containers = append(p.Containers, c.Name)
		for _, port := range c.Ports {
			p.Ports = append(p.Ports, port.ContainerPort)
		}
	}
	p.Total = len(o.Spec.Containers)
	var problems []string
	for _, cs := range o.Status.ContainerStatuses {
		p.Restarts += cs.RestartCount
		if cs.Ready {
			p.Ready++
		}
		switch {
		case cs.State.Waiting != nil && cs.State.Waiting.Reason != "":
			p.Status = cs.State.Waiting.Reason
			if cs.State.Waiting.Reason != "ContainerCreating" && cs.State.Waiting.Reason != "PodInitializing" {
				problems = append(problems, describeState(cs.Name, cs.State.Waiting, cs.LastState.Terminated))
			}
		case cs.State.Terminated != nil && cs.State.Terminated.Reason != "" && o.Status.Phase != "Running":
			p.Status = cs.State.Terminated.Reason
			if cs.State.Terminated.ExitCode != 0 {
				problems = append(problems, describeState(cs.Name, cs.State.Terminated, nil))
			}
		case !cs.Ready && o.Status.Phase == "Running":
			problems = append(problems, cs.Name+" is not ready (readiness probe)")
		}
	}
	if o.Status.Phase == "Pending" && len(o.Status.ContainerStatuses) == 0 {
		if c, ok := conditionOf(o.Status.Conditions, "PodScheduled"); ok && c.Status == "False" {
			p.Status = "Unschedulable"
			problems = append(problems, c.Message)
		}
	}
	if o.Metadata.DeletionTimestamp != "" {
		p.Status, problems = "Terminating", nil
	}
	if p.Status == "" {
		p.Status = "Unknown"
	}
	p.Healthy = (p.Status == "Running" && p.Ready == p.Total) || p.Status == "Completed" || o.Status.Phase == "Succeeded"
	if !p.Healthy {
		p.Problem = strings.Join(problems, "; ")
	}
	if len(o.Metadata.OwnerReferences) > 0 {
		p.Owner, p.OwnerKind = o.Metadata.OwnerReferences[0].Name, o.Metadata.OwnerReferences[0].Kind
	}
	return p
}

// describeState: "app: CrashLoopBackOff, last OOMKilled (exit 137)", "app: ErrImagePull: …".
func describeState(name string, now, last *containerState) string {
	s := name + ": " + now.Reason
	if now.ExitCode != 0 {
		s += " (exit " + strconv.Itoa(now.ExitCode) + ")"
	}
	if last != nil && last.Reason != "" {
		s += ", last " + last.Reason
		if last.ExitCode != 0 {
			s += " (exit " + strconv.Itoa(last.ExitCode) + ")"
		}
	}
	if msg := strings.TrimSpace(now.Message); msg != "" {
		s += ": " + truncate(msg, 300)
	}
	return s
}

func service(o object) Service {
	s := Service{Name: o.Metadata.Name, Type: o.Spec.Type, ClusterIP: o.Spec.ClusterIP, Ports: []ServicePort{}, labels: o.Metadata.Labels, selector: selectorLabels(o.Spec.Selector, "Service")}
	for _, p := range o.Spec.Ports {
		s.Ports = append(s.Ports, ServicePort{Port: p.Port, Target: strings.Trim(string(p.TargetPort), `"`), Protocol: p.Protocol, NodePort: p.NodePort})
	}
	return s
}

func ingress(o object) Ingress {
	in := Ingress{Name: o.Metadata.Name, Class: o.Spec.IngressClassName, Routes: []IngressRoute{}, TLS: len(o.Spec.TLS) > 0, labels: o.Metadata.Labels}
	port := func(b *ingressBackend) string {
		if b.Port.Name != "" {
			return b.Port.Name
		}
		return strconv.Itoa(b.Port.Number)
	}
	if d := o.Spec.DefaultBackend; d != nil && d.Service != nil {
		in.Routes = append(in.Routes, IngressRoute{Host: "*", Path: "/", Service: d.Service.Name, Port: port(d.Service)})
	}
	for _, r := range o.Spec.Rules {
		if r.HTTP == nil {
			continue
		}
		for _, path := range r.HTTP.Paths {
			if b := path.Backend.Service; b != nil {
				in.Routes = append(in.Routes, IngressRoute{Host: r.Host, Path: path.Path, Service: b.Name, Port: port(b)})
			}
		}
	}
	if lb := o.Status.LoadBalancer.Ingress; len(lb) > 0 {
		in.Address = lb[0].Hostname
		if in.Address == "" {
			in.Address = lb[0].IP
		}
	}
	return in
}

func jobStatus(o object) string {
	completions := 1
	if o.Spec.Completions != nil {
		completions = *o.Spec.Completions
	}
	switch {
	case o.Status.Active > 0:
		return "running"
	case o.Status.Succeeded >= completions:
		return "complete"
	case o.Status.Failed > 0:
		return "failed (" + strconv.Itoa(o.Status.Failed) + ")"
	}
	return "pending"
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}
