package main

import (
	"encoding/json"
	"fmt"

	"github.com/flint-ide/extensions/kubernetes/internal/kube"
)

// boundOverview keeps an overview under limit bytes of JSON (Flint's 4 MiB
// per message): a huge namespace loses its longest lists from the end,
// events first, and Partial says what was cut. Ordinary namespaces are far
// below the limit and come back unchanged.
func boundOverview(ov kube.Overview, limit int) kube.Overview {
	size := func() int {
		data, _ := json.Marshal(ov)
		return len(data)
	}
	if size() <= limit {
		return ov
	}
	type list struct {
		name string
		n    func() int
		cut  func(n int)
	}
	lists := []list{
		{"events", func() int { return len(ov.Events) }, func(n int) { ov.Events = ov.Events[:n] }},
		{"pods", func() int { return len(ov.Pods) }, func(n int) { ov.Pods = ov.Pods[:n] }},
		{"jobs", func() int { return len(ov.Jobs) }, func(n int) { ov.Jobs = ov.Jobs[:n] }},
		{"services", func() int { return len(ov.Services) }, func(n int) { ov.Services = ov.Services[:n] }},
		{"ingresses", func() int { return len(ov.Ingresses) }, func(n int) { ov.Ingresses = ov.Ingresses[:n] }},
		{"apps", func() int { return len(ov.Apps) }, func(n int) { ov.Apps = ov.Apps[:n] }},
		{"workloads", func() int { return len(ov.Workloads) }, func(n int) { ov.Workloads = ov.Workloads[:n] }},
	}
	ov.Partial = append([]string(nil), ov.Partial...)
	for _, l := range lists {
		total := l.n()
		for size() > limit && l.n() > 0 {
			l.cut(l.n() / 2)
		}
		if l.n() < total {
			ov.Partial = append(ov.Partial, fmt.Sprintf("%s (first %d of %d)", l.name, l.n(), total))
		}
		if size() <= limit {
			break
		}
	}
	return ov
}
