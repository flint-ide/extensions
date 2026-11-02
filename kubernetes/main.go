// Command kubernetes-ext is Flint's Kubernetes extension: a panel (its own
// HTML, ui/panel.html) showing any context of the kubeconfig without
// switching the global one — apps with their health and problems, workloads,
// pods, services and jobs, with actions (rollout restart and undo, scale,
// delete pod, logs, a shell, use-context, a diagnosis for an agent) — and the
// agent tool kubernetes_status. kubectl does the work; the panel's page talks
// to this process with flint.request (webview/request), and the process opens
// terminal tabs (window/openTerminal) and asks the agent (agent/ask).
//
// Build it next to its manifest and install the folder from Flint
// (Install Extension from Folder…):
//
//	go build -o kubernetes-ext .
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/flint-ide/extensions/kubernetes/internal/awsprof"
	"github.com/flint-ide/extensions/kubernetes/internal/kube"
)

const (
	panelID   = "flint.kubernetes.panel"
	refreshID = "flint.kubernetes.refresh"
	toolName  = "status"
)

// Refresh interval bounds, as Flint's refresh.kubernetesSeconds.
const (
	defaultRefreshSeconds = 5
	minRefreshSeconds     = 2
	maxRefreshSeconds     = 600
)

// cluster is what the extension asks of kubectl (implemented by
// *kube.Kubectl; tests use a fake).
type cluster interface {
	Contexts(ctx context.Context) ([]kube.Context, error)
	Namespaces(ctx context.Context, kubeContext string) ([]string, error)
	Overview(ctx context.Context, kubeContext, namespace string) (kube.Overview, error)
	RolloutRestart(ctx context.Context, kubeContext, namespace, kind, name string) error
	RolloutUndo(ctx context.Context, kubeContext, namespace, kind, name string) error
	Scale(ctx context.Context, kubeContext, namespace, kind, name string, replicas int) error
	DeletePod(ctx context.Context, kubeContext, namespace, name string) error
	UseContext(ctx context.Context, name string) error
	Diagnose(ctx context.Context, kubeContext, namespace, appKey string) (string, error)
}

// peer is Flint, seen from the extension (implemented by *conn).
type peer interface {
	call(method string, params, result any) error
	notify(method string, params any)
}

// refresher renews AWS credentials without a terminal (*awsprof.Refresher).
type refresher interface {
	Refresh(ctx context.Context, p awsprof.Profile) (string, error)
}

type settings struct {
	RefreshSeconds int `json:"refreshSeconds"`
}

// refreshSeconds is the setting within its bounds.
func (s settings) refreshSeconds() int {
	if s.RefreshSeconds < minRefreshSeconds || s.RefreshSeconds > maxRefreshSeconds {
		return defaultRefreshSeconds
	}
	return s.RefreshSeconds
}

type ext struct {
	c        peer
	kubectl  func() (cluster, error)
	profiles func() ([]awsprof.Profile, error)
	aws      refresher
	data     string // FLINT_EXTENSION_DATA

	mu       sync.Mutex
	settings settings

	refreshing sync.Map // AWS profiles with a refresh running
}

func newExt(c peer, data string) *ext {
	return &ext{
		c: c,
		kubectl: func() (cluster, error) {
			k, err := kube.New() // looked up on each use: kubectl may be installed meanwhile
			if err != nil {
				return nil, err
			}
			return k, nil
		},
		profiles: func() ([]awsprof.Profile, error) {
			files, err := awsprof.DefaultFiles()
			if err != nil {
				return nil, err
			}
			return awsprof.Load(files, time.Now())
		},
		aws:      awsprof.NewRefresher(),
		data:     data,
		settings: settings{RefreshSeconds: defaultRefreshSeconds},
	}
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("kubernetes: ") // stderr: Flint's log of the extension
	data, err := dataDir()
	if err != nil {
		log.Fatal(err)
	}
	c := newConn(os.Stdout)
	e := newExt(c, data)
	if err := c.serve(os.Stdin, e.handle, e.notified); err != nil {
		log.Fatal(err)
	}
}

// handle answers Flint's requests.
func (e *ext) handle(method string, params json.RawMessage) (any, error) {
	switch method {
	case "initialize":
		var p struct {
			Settings settings `json:"settings"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		e.mu.Lock()
		e.settings = p.Settings
		e.mu.Unlock()
		return map[string]any{"contract": 1, "capabilities": map[string]any{}}, nil
	case "shutdown":
		return nil, nil
	case "command/execute":
		var p struct{ ID string }
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if p.ID != refreshID {
			return nil, &rpcError{Code: -32602, Message: "unknown command " + p.ID}
		}
		e.post(map[string]any{"type": "refresh"})
		return nil, nil
	case "webview/request":
		var p struct {
			View   string          `json:"view"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if p.View != panelID {
			return nil, &rpcError{Code: -32602, Message: "unknown view " + p.View}
		}
		res, err := e.request(context.Background(), p.Method, p.Params)
		if err != nil {
			return nil, err
		}
		return bounded(res)
	case "agentTool/call":
		var p struct {
			Name      string `json:"name"`
			Arguments struct {
				Namespace string `json:"namespace"`
			} `json:"arguments"`
		}
		if err := json.Unmarshal(params, &p); err != nil {
			return nil, err
		}
		if p.Name != toolName {
			return nil, &rpcError{Code: -32601, Message: "unknown tool " + p.Name}
		}
		k, err := e.kubectl()
		if err != nil {
			return nil, err
		}
		text, err := kubeStatus(context.Background(), k, p.Arguments.Namespace)
		if err != nil {
			return nil, err
		}
		return map[string]string{"text": text}, nil
	}
	return nil, &rpcError{Code: -32601, Message: "method not found: " + method}
}

// notified handles Flint's notifications; the page hears of them through webview/post.
func (e *ext) notified(method string, params json.RawMessage) {
	switch method {
	case "settings/changed":
		var p struct{ Settings settings }
		if json.Unmarshal(params, &p) == nil {
			e.mu.Lock()
			e.settings = p.Settings
			e.mu.Unlock()
			e.post(map[string]any{"type": "settings", "refreshSeconds": p.Settings.refreshSeconds()})
		}
	case "view/visible":
		var p struct {
			View    string `json:"view"`
			Visible bool   `json:"visible"`
		}
		if json.Unmarshal(params, &p) == nil && p.View == panelID {
			e.post(map[string]any{"type": "visible", "visible": p.Visible})
		}
	}
}

// post sends a message to every frame of the panel.
func (e *ext) post(message any) {
	e.c.notify("webview/post", map[string]any{"view": panelID, "message": message})
}

// maxAnswer keeps an answer under Flint's 4 MiB per message, with room for
// the envelope.
const maxAnswer = 4<<20 - 64<<10

// bounded refuses an answer Flint would refuse (overviews are cut before).
func bounded(v any) (any, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if len(data) > maxAnswer {
		return nil, fmt.Errorf("the answer is too large (%d MiB)", len(data)>>20)
	}
	return json.RawMessage(data), nil
}
