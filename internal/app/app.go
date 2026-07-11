package app

import (
	"context"
	"fmt"

	"github.com/elft/KubeSqueeze/internal/kube"
	"github.com/elft/KubeSqueeze/internal/options"
	"github.com/elft/KubeSqueeze/internal/output"
	"github.com/elft/KubeSqueeze/internal/selection"
)

// ClusterError preserves the resolved target when an operation fails after
// kubeconfig resolution, allowing the CLI to emit an auditable JSON error.
type ClusterError struct {
	Cluster output.Cluster
	Err     error
}

func (e *ClusterError) Error() string { return e.Err.Error() }
func (e *ClusterError) Unwrap() error { return e.Err }

// Run connects the public CLI contracts to discovery, filtering, and mutation.
func Run(ctx context.Context, opts options.Validated) (output.Result, error) {
	client, err := kube.NewClient(kube.Config{Kubeconfig: opts.Kubeconfig, Context: opts.Context})
	if err != nil {
		return output.Result{}, err
	}
	cluster := output.Cluster{Context: client.Target.Context, Server: client.Target.Server}
	fail := func(err error) (output.Result, error) {
		return output.Result{}, &ClusterError{Cluster: cluster, Err: err}
	}
	if err := client.Preflight(ctx); err != nil {
		return fail(err)
	}

	kinds, err := discoveryKinds(opts.Include.Kinds)
	if err != nil {
		return fail(err)
	}
	namespaces := opts.Namespaces
	if opts.AllNamespaces {
		namespaces = nil
	}
	workloads, err := kube.NewDiscoverer(client).List(ctx, namespaces, kinds)
	if err != nil {
		return fail(err)
	}

	resources := make([]selection.Resource, 0, len(workloads))
	for _, workload := range workloads {
		resources = append(resources, selectionResource(workload))
	}
	selected := selection.Apply(resources, opts.IncludeRules, opts.IgnoreRules)

	ignored := make([]output.IgnoredResource, 0, len(selected.Ignored))
	for _, item := range selected.Ignored {
		ignored = append(ignored, output.IgnoredResource{
			Namespace:  item.Resource.Namespace,
			Kind:       item.Resource.Kind,
			Name:       item.Resource.Name,
			Categories: item.Categories,
		})
	}
	mutationSet := make([]kube.Workload, 0, len(selected.Included))
	for _, resource := range selected.Included {
		workload, ok := resource.Object.(kube.Workload)
		if !ok {
			return fail(fmt.Errorf("internal selection record %s/%s has no Kubernetes workload", resource.Namespace, resource.Name))
		}
		mutationSet = append(mutationSet, workload)
	}

	operation := kube.Squeeze
	if opts.Operation == options.OperationRestore {
		operation = kube.Restore
	}
	changes, err := kube.NewEngine(client).Apply(ctx, operation, mutationSet)
	if err != nil {
		return fail(err)
	}
	mutated := make([]output.MutatedResource, 0, len(changes))
	for _, change := range changes {
		previous, err := stateFor(change.Kind, change.Previous)
		if err != nil {
			return fail(err)
		}
		current, err := stateFor(change.Kind, change.Current)
		if err != nil {
			return fail(err)
		}
		mutated = append(mutated, output.MutatedResource{
			Namespace: change.Namespace,
			Kind:      selection.Kind(change.Kind),
			Name:      change.Name,
			Previous:  previous,
			Current:   current,
			Status:    change.Status,
		})
	}

	return output.Result{
		Operation:  string(opts.Operation),
		Cluster:    cluster,
		Discovered: selected.Discovered,
		Included:   len(selected.Included) + len(selected.Ignored),
		Ignored:    ignored,
		Mutated:    mutated,
	}, nil
}

func discoveryKinds(raw []string) ([]kube.Kind, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	result := make([]kube.Kind, 0, len(raw))
	seen := map[kube.Kind]bool{}
	for _, value := range raw {
		kind, err := selection.ParseKind(value)
		if err != nil {
			return nil, err
		}
		parsed, err := kube.ParseKind(string(kind))
		if err != nil {
			return nil, err
		}
		if !seen[parsed] {
			seen[parsed] = true
			result = append(result, parsed)
		}
	}
	return result, nil
}

func selectionResource(workload kube.Workload) selection.Resource {
	owners := make([]selection.Owner, 0, len(workload.Owners()))
	for _, owner := range workload.Owners() {
		owners = append(owners, selection.Owner{APIVersion: owner.APIVersion, Kind: owner.Kind, Name: owner.Name})
	}
	return selection.Resource{
		APIVersion:  workload.Object.GetAPIVersion(),
		Kind:        selection.Kind(workload.Kind),
		Namespace:   workload.Namespace(),
		Name:        workload.Name(),
		Labels:      workload.Labels(),
		Annotations: workload.Annotations(),
		Owners:      owners,
		Object:      workload,
	}
}

func stateFor(kind kube.Kind, value any) (output.State, error) {
	switch kind {
	case kube.Deployment, kube.StatefulSet:
		replicas, ok := value.(int64)
		if !ok || replicas < 0 || replicas > int64(^uint32(0)>>1) {
			return output.State{}, fmt.Errorf("invalid replica state %v for %s", value, kind)
		}
		converted := int32(replicas)
		return output.State{Replicas: &converted}, nil
	case kube.CronJob:
		suspend, ok := value.(bool)
		if !ok {
			return output.State{}, fmt.Errorf("invalid suspend state %v", value)
		}
		return output.State{Suspend: &suspend}, nil
	default:
		return output.State{}, fmt.Errorf("unsupported kind %q", kind)
	}
}
