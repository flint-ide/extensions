package kube

import (
	"context"
	"fmt"
	"slices"
	"strconv"
)

// restartable kinds take `kubectl rollout restart`; scalable ones `kubectl scale`.
var (
	restartable = []string{"Deployment", "StatefulSet", "DaemonSet"}
	scalable    = []string{"Deployment", "StatefulSet"}
)

// MaxReplicas bounds Scale (a typo of 1000 instead of 10 is refused).
const MaxReplicas = 100

// RolloutRestart restarts a workload's pods one by one.
func (k *Kubectl) RolloutRestart(ctx context.Context, kubeContext, namespace, kind, name string) error {
	if !slices.Contains(restartable, kind) || !validName(name) {
		return fmt.Errorf("cannot restart %s %q", kind, name)
	}
	args, err := scope(kubeContext, namespace)
	if err != nil {
		return err
	}
	_, err = k.run(ctx, actionTimeout, append(args, "rollout", "restart", kind+"/"+name)...)
	return err
}

// Scale sets a workload's replica count.
func (k *Kubectl) Scale(ctx context.Context, kubeContext, namespace, kind, name string, replicas int) error {
	if !slices.Contains(scalable, kind) || !validName(name) || replicas < 0 || replicas > MaxReplicas {
		return fmt.Errorf("cannot scale %s %q to %d", kind, name, replicas)
	}
	args, err := scope(kubeContext, namespace)
	if err != nil {
		return err
	}
	_, err = k.run(ctx, actionTimeout, append(args, "scale", kind+"/"+name, "--replicas="+strconv.Itoa(replicas))...)
	return err
}

// DeletePod deletes a pod (its controller usually starts a new one).
func (k *Kubectl) DeletePod(ctx context.Context, kubeContext, namespace, name string) error {
	if !validName(name) {
		return fmt.Errorf("invalid pod name %q", name)
	}
	args, err := scope(kubeContext, namespace)
	if err != nil {
		return err
	}
	_, err = k.run(ctx, actionTimeout, append(args, "delete", "pod", name, "--wait=false")...)
	return err
}

// RolloutUndo rolls a workload back to its previous revision.
func (k *Kubectl) RolloutUndo(ctx context.Context, kubeContext, namespace, kind, name string) error {
	if !slices.Contains(restartable, kind) || !validName(name) {
		return fmt.Errorf("cannot undo %s %q", kind, name)
	}
	args, err := scope(kubeContext, namespace)
	if err != nil {
		return err
	}
	_, err = k.run(ctx, actionTimeout, append(args, "rollout", "undo", kind+"/"+name)...)
	return err
}
