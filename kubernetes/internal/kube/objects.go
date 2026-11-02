package kube

import "encoding/json"

// object is the union of the fields Flint reads from every kind it lists.
type object struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name              string            `json:"name"`
		Namespace         string            `json:"namespace"`
		UID               string            `json:"uid"`
		Generation        int64             `json:"generation"`
		CreationTimestamp string            `json:"creationTimestamp"`
		DeletionTimestamp string            `json:"deletionTimestamp"`
		Labels            map[string]string `json:"labels"`
		Annotations       map[string]string `json:"annotations"`
		OwnerReferences   []struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
		} `json:"ownerReferences"`
	} `json:"metadata"`
	Spec struct {
		Replicas    *int            `json:"replicas"`
		Paused      bool            `json:"paused"`
		NodeName    string          `json:"nodeName"`
		Type        string          `json:"type"`
		ClusterIP   string          `json:"clusterIP"`
		Schedule    string          `json:"schedule"`
		Suspend     *bool           `json:"suspend"`
		Completions *int            `json:"completions"`
		Selector    json.RawMessage `json:"selector"` // Service: map; workloads: LabelSelector
		Template    struct {
			Metadata struct {
				Labels map[string]string `json:"labels"`
			} `json:"metadata"`
			Spec struct {
				Containers []container `json:"containers"`
			} `json:"spec"`
		} `json:"template"`
		Containers []container `json:"containers"`
		Ports      []struct {
			Port       int             `json:"port"`
			TargetPort json.RawMessage `json:"targetPort"`
			Protocol   string          `json:"protocol"`
			NodePort   int             `json:"nodePort"`
		} `json:"ports"`
		// Ingress
		IngressClassName string `json:"ingressClassName"`
		DefaultBackend   *struct {
			Service *ingressBackend `json:"service"`
		} `json:"defaultBackend"`
		Rules []struct {
			Host string `json:"host"`
			HTTP *struct {
				Paths []struct {
					Path    string `json:"path"`
					Backend struct {
						Service *ingressBackend `json:"service"`
					} `json:"backend"`
				} `json:"paths"`
			} `json:"http"`
		} `json:"rules"`
		TLS []struct {
			Hosts []string `json:"hosts"`
		} `json:"tls"`
	} `json:"spec"`
	Status struct {
		Phase                  string      `json:"phase"`
		Replicas               int         `json:"replicas"`
		ReadyReplicas          int         `json:"readyReplicas"`
		UpdatedReplicas        int         `json:"updatedReplicas"`
		AvailableReplicas      int         `json:"availableReplicas"`
		ObservedGeneration     int64       `json:"observedGeneration"`
		CurrentRevision        string      `json:"currentRevision"`
		UpdateRevision         string      `json:"updateRevision"`
		DesiredNumberScheduled int         `json:"desiredNumberScheduled"`
		NumberReady            int         `json:"numberReady"`
		NumberAvailable        int         `json:"numberAvailable"`
		UpdatedNumberScheduled int         `json:"updatedNumberScheduled"`
		Active                 int         `json:"active"`
		Succeeded              int         `json:"succeeded"`
		Failed                 int         `json:"failed"`
		Conditions             []condition `json:"conditions"`
		LastAppliedRevision    string      `json:"lastAppliedRevision"`
		LoadBalancer           struct {
			Ingress []struct {
				IP       string `json:"ip"`
				Hostname string `json:"hostname"`
			} `json:"ingress"`
		} `json:"loadBalancer"`
		ContainerStatuses []struct {
			Name         string `json:"name"`
			Image        string `json:"image"`
			Ready        bool   `json:"ready"`
			RestartCount int    `json:"restartCount"`
			State        struct {
				Waiting    *containerState `json:"waiting"`
				Terminated *containerState `json:"terminated"`
			} `json:"state"`
			LastState struct {
				Terminated *containerState `json:"terminated"`
			} `json:"lastState"`
		} `json:"containerStatuses"`
	} `json:"status"`
}

type container struct {
	Name  string `json:"name"`
	Image string `json:"image"`
	Ports []struct {
		ContainerPort int `json:"containerPort"`
	} `json:"ports"`
}

type containerState struct {
	Reason   string `json:"reason"`
	Message  string `json:"message"`
	ExitCode int    `json:"exitCode"`
}

type condition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

type ingressBackend struct {
	Name string `json:"name"`
	Port struct {
		Number int    `json:"number"`
		Name   string `json:"name"`
	} `json:"port"`
}

// selectorLabels reads a Service selector (a map) or a workload's
// LabelSelector (matchLabels); matchExpressions are ignored.
func selectorLabels(raw json.RawMessage, kind string) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	if kind == "Service" {
		var m map[string]string
		_ = json.Unmarshal(raw, &m)
		return m
	}
	var sel struct {
		MatchLabels map[string]string `json:"matchLabels"`
	}
	_ = json.Unmarshal(raw, &sel)
	return sel.MatchLabels
}

// matchesSelector: every selector label is on the object (an empty selector matches nothing).
func matchesSelector(selector, labels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if labels[k] != v {
			return false
		}
	}
	return true
}

func conditionOf(conds []condition, typ string) (condition, bool) {
	for _, c := range conds {
		if c.Type == typ {
			return c, true
		}
	}
	return condition{}, false
}
