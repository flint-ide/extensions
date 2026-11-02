package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/flint-ide/extensions/kubernetes/internal/kube"
)

// args are the params of the page's requests (each uses some of them).
type args struct {
	Context   string `json:"context"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Target    string `json:"target"`
	Pod       string `json:"pod"`
	Container string `json:"container"`
	App       string `json:"app"`
	Profile   string `json:"profile"`
	Replicas  *int   `json:"replicas"`
	Port      int    `json:"port"`
	// State is saveState's value.
	State json.RawMessage `json:"state"`
}

// request answers the panel's flint.request(method, params).
func (e *ext) request(ctx context.Context, method string, params json.RawMessage) (any, error) {
	var a args
	if len(params) > 0 && string(params) != "null" {
		if err := json.Unmarshal(params, &a); err != nil {
			return nil, &rpcError{Code: -32602, Message: "invalid params: " + err.Error()}
		}
	}
	switch method {
	case "settings":
		e.mu.Lock()
		defer e.mu.Unlock()
		return map[string]any{"refreshSeconds": e.settings.refreshSeconds()}, nil
	case "state":
		return loadState(e.data), nil
	case "saveState":
		return nil, saveState(e.data, a.State)
	case "awsProfile":
		return e.awsProfile(a.Profile)
	case "awsLogin":
		return nil, e.awsLogin(a.Profile)
	case "awsRefresh":
		return nil, e.awsRefresh(a.Profile)
	}
	if handler, ok := terminalRequests[method]; ok {
		title, argv, err := handler(a)
		if err != nil {
			return nil, err
		}
		return nil, e.openTerminal(map[string]any{"title": title, "command": argv})
	}
	k, err := e.kubectl()
	if err != nil {
		return nil, err
	}
	return e.clusterRequest(ctx, k, method, a)
}

// clusterRequest answers the requests that run kubectl in this process.
func (e *ext) clusterRequest(ctx context.Context, k cluster, method string, a args) (any, error) {
	switch method {
	case "contexts":
		return k.Contexts(ctx)
	case "namespaces":
		return k.Namespaces(ctx, a.Context)
	case "overview":
		ov, err := k.Overview(ctx, a.Context, a.Namespace)
		if err != nil {
			return nil, err
		}
		return boundOverview(ov, maxAnswer), nil
	case "restart":
		return nil, k.RolloutRestart(ctx, a.Context, a.Namespace, a.Kind, a.Name)
	case "undo":
		return nil, k.RolloutUndo(ctx, a.Context, a.Namespace, a.Kind, a.Name)
	case "scale":
		if a.Replicas == nil {
			return nil, &rpcError{Code: -32602, Message: "scale needs replicas"}
		}
		return nil, k.Scale(ctx, a.Context, a.Namespace, a.Kind, a.Name, *a.Replicas)
	case "deletePod":
		return nil, k.DeletePod(ctx, a.Context, a.Namespace, a.Name)
	case "useContext":
		return nil, k.UseContext(ctx, a.Context)
	case "shell":
		if err := checkScope(a.Context, a.Namespace); err != nil {
			return nil, err
		}
		env, err := terminalEnv(ctx, k, e.data, a.Context, a.Namespace)
		if err != nil {
			return nil, err
		}
		short, _, _ := strings.Cut(shortContext(a.Context), " ")
		return nil, e.openTerminal(map[string]any{"env": env, "tag": "k8s:" + short + "/" + a.Namespace})
	case "diagnose":
		return nil, e.diagnose(ctx, k, a)
	}
	return nil, &rpcError{Code: -32601, Message: "unknown request " + method}
}

// terminalRequests open a terminal tab running kubectl (or flux): each
// checks its values and returns the tab's title and the argv.
var terminalRequests = map[string]func(a args) (string, []string, error){
	"logs": func(a args) (string, []string, error) {
		if err := checkAll(checkScope(a.Context, a.Namespace), checkTarget(a.Target)); err != nil {
			return "", nil, err
		}
		return "logs: " + lastPart(a.Target), logsCommand(a.Context, a.Namespace, a.Target), nil
	},
	"exec": func(a args) (string, []string, error) {
		if err := checkAll(checkScope(a.Context, a.Namespace), checkName("pod", a.Pod)); err != nil {
			return "", nil, err
		}
		if a.Container != "" && !kube.ValidName(a.Container) {
			return "", nil, fmt.Errorf("invalid container %q", a.Container)
		}
		return "sh: " + a.Pod, execCommand(a.Context, a.Namespace, a.Pod, a.Container), nil
	},
	"describe": func(a args) (string, []string, error) {
		target := resourceRef(a.Kind, a.Name)
		if err := checkAll(checkScope(a.Context, a.Namespace), checkTarget(target)); err != nil {
			return "", nil, err
		}
		return "describe: " + a.Name, describeCommand(a.Context, a.Namespace, target), nil
	},
	"portForward": func(a args) (string, []string, error) {
		if err := checkAll(checkScope(a.Context, a.Namespace), checkTarget(a.Target)); err != nil {
			return "", nil, err
		}
		if a.Port < 1 || a.Port > 65535 {
			return "", nil, fmt.Errorf("invalid port %d", a.Port)
		}
		return fmt.Sprintf(":%d → %s", a.Port, lastPart(a.Target)), portForwardCommand(a.Context, a.Namespace, a.Target, a.Port), nil
	},
	"rolloutStatus": func(a args) (string, []string, error) {
		target := resourceRef(a.Kind, a.Name)
		if err := checkAll(checkScope(a.Context, a.Namespace), checkTarget(target)); err != nil {
			return "", nil, err
		}
		return "rollout: " + a.Name, rolloutStatusCommand(a.Context, a.Namespace, target), nil
	},
	"rolloutHistory": func(a args) (string, []string, error) {
		target := resourceRef(a.Kind, a.Name)
		if err := checkAll(checkScope(a.Context, a.Namespace), checkTarget(target)); err != nil {
			return "", nil, err
		}
		return "history: " + a.Name, rolloutHistoryCommand(a.Context, a.Namespace, target), nil
	},
	// reconcile: namespace is the Kustomization's own (often flux-system).
	"reconcile": func(a args) (string, []string, error) {
		if err := checkAll(checkScope(a.Context, a.Namespace), checkName("Kustomization", a.Name)); err != nil {
			return "", nil, err
		}
		if a.Namespace == "" {
			return "", nil, fmt.Errorf("reconcile needs the Kustomization's namespace")
		}
		return "flux: " + a.Name, fluxReconcileCommand(a.Context, a.Name, a.Namespace), nil
	},
}

func checkName(what, name string) error {
	if !kube.ValidName(name) {
		return fmt.Errorf("invalid %s name %q", what, name)
	}
	return nil
}

// checkAll is the first error.
func checkAll(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

// openTerminal asks Flint for a terminal tab (permission terminal).
func (e *ext) openTerminal(params map[string]any) error {
	return e.c.call("window/openTerminal", params, nil)
}

// diagnose gathers an unhealthy app's state and failing pods' logs (redacted)
// and puts the diagnosis prompt in the agent chat's draft (agent/ask never
// sends: the user reviews it first).
func (e *ext) diagnose(ctx context.Context, k cluster, a args) error {
	if err := checkScope(a.Context, a.Namespace); err != nil {
		return err
	}
	if a.App == "" {
		return &rpcError{Code: -32602, Message: "diagnose needs the app's key"}
	}
	diagnostics, err := k.Diagnose(ctx, a.Context, a.Namespace, a.App)
	if err != nil {
		return err
	}
	name := a.Name
	if name == "" {
		name = a.App
	}
	return e.c.call("agent/ask", map[string]any{"prompt": diagnosePrompt(a.Context, a.Namespace, name, diagnostics)}, nil)
}
