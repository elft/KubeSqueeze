package kube

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
)

const (
	OriginalReplicasAnnotation = "kubesqueeze.io/original-replicas"
	OriginalSuspendAnnotation  = "kubesqueeze.io/original-suspend"
)

type Operation string

const (
	Squeeze Operation = "squeeze"
	Restore Operation = "restore"
)

// Change is a stable, JSON-ready account of one successfully processed
// workload. Previous and Current are int64 for replica workloads and bool for
// CronJobs.
type Change struct {
	Namespace   string            `json:"namespace"`
	Kind        Kind              `json:"kind"`
	Name        string            `json:"name"`
	Previous    any               `json:"previous"`
	Current     any               `json:"current"`
	Status      string            `json:"status"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type Engine struct {
	dynamic dynamic.Interface
}

func NewEngine(client *Client) *Engine { return &Engine{dynamic: client.Dynamic} }

func NewEngineForInterface(client dynamic.Interface) *Engine { return &Engine{dynamic: client} }

// Apply validates every workload before making the first change, then patches
// resources in deterministic input-independent order. API failures are
// fail-fast; successful earlier patches cannot be rolled back by Kubernetes.
func (e *Engine) Apply(ctx context.Context, operation Operation, workloads []Workload) ([]Change, error) {
	plans, err := planMutations(operation, workloads)
	if err != nil {
		return nil, err
	}

	changes := make([]Change, 0, len(plans))
	for _, plan := range plans {
		if plan.patch != nil {
			payload, err := json.Marshal(plan.patch)
			if err != nil {
				return nil, fmt.Errorf("encode patch for %s/%s: %w", plan.workload.Namespace(), plan.workload.Name(), err)
			}
			_, err = e.dynamic.Resource(plan.workload.GVR).Namespace(plan.workload.Namespace()).Patch(
				ctx, plan.workload.Name(), types.MergePatchType, payload, metav1.PatchOptions{},
			)
			if err != nil {
				return nil, fmt.Errorf("patch %s %s/%s: %w", plan.workload.Kind, plan.workload.Namespace(), plan.workload.Name(), err)
			}
		}
		changes = append(changes, plan.change)
	}
	return changes, nil
}

// Plan performs the same validation and returns the same changes as Apply,
// including annotation updates, without sending patches to Kubernetes.
func (e *Engine) Plan(operation Operation, workloads []Workload) ([]Change, error) {
	plans, err := planMutations(operation, workloads)
	if err != nil {
		return nil, err
	}
	changes := make([]Change, 0, len(plans))
	for _, plan := range plans {
		changes = append(changes, plan.change)
	}
	return changes, nil
}

func planMutations(operation Operation, workloads []Workload) ([]mutation, error) {
	if operation != Squeeze && operation != Restore {
		return nil, fmt.Errorf("unsupported operation %q", operation)
	}
	items := append([]Workload(nil), workloads...)
	sortWorkloads(items)
	plans := make([]mutation, 0, len(items))
	seen := make(map[string]bool, len(items))
	for _, workload := range items {
		key := workload.GVR.String() + "/" + workload.Namespace() + "/" + workload.Name()
		if seen[key] {
			return nil, fmt.Errorf("duplicate workload %s %s/%s", workload.Kind, workload.Namespace(), workload.Name())
		}
		seen[key] = true
		plan, err := planMutation(operation, workload)
		if err != nil {
			return nil, fmt.Errorf("%s %s/%s: %w", workload.Kind, workload.Namespace(), workload.Name(), err)
		}
		plans = append(plans, plan)
	}

	return plans, nil
}

type mutation struct {
	workload Workload
	patch    map[string]any
	change   Change
}

func planMutation(operation Operation, workload Workload) (mutation, error) {
	if workload.Object == nil {
		return mutation{}, fmt.Errorf("object is nil")
	}
	if workload.Name() == "" || workload.Namespace() == "" {
		return mutation{}, fmt.Errorf("object must have a name and namespace")
	}
	switch workload.Kind {
	case Deployment, StatefulSet:
		if (workload.Kind == Deployment && workload.GVR != deploymentsGVR) || (workload.Kind == StatefulSet && workload.GVR != statefulSetsGVR) {
			return mutation{}, fmt.Errorf("unexpected API resource %s", workload.GVR)
		}
		return planReplicas(operation, workload)
	case CronJob:
		if workload.GVR != cronJobsV1GVR && workload.GVR != cronJobsBetaGVR {
			return mutation{}, fmt.Errorf("unexpected API resource %s", workload.GVR)
		}
		return planCronJob(operation, workload)
	default:
		return mutation{}, fmt.Errorf("unsupported kind %q", workload.Kind)
	}
}

func planReplicas(operation Operation, workload Workload) (mutation, error) {
	current, found, err := nestedInt64(workload.Object.Object, "spec", "replicas")
	if err != nil {
		return mutation{}, err
	}
	if !found {
		current = 1
	}
	if current < 0 {
		return mutation{}, fmt.Errorf("spec.replicas cannot be negative")
	}
	annotations := workload.Annotations()
	snapshot, hasSnapshot := annotations[OriginalReplicasAnnotation]

	var desired int64
	var annotation string
	if operation == Squeeze {
		desired = 0
		if current == 0 && hasSnapshot {
			if _, err := parseReplicas(snapshot); err != nil {
				return mutation{}, err
			}
			annotation = snapshot
		} else {
			annotation = strconv.FormatInt(current, 10)
		}
	} else {
		if !hasSnapshot {
			return mutation{}, fmt.Errorf("missing annotation %q", OriginalReplicasAnnotation)
		}
		desired, err = parseReplicas(snapshot)
		if err != nil {
			return mutation{}, err
		}
		annotation = snapshot
	}

	changed := desired != current || !hasSnapshot || annotations[OriginalReplicasAnnotation] != annotation
	return buildMutation(workload, current, desired, changed, OriginalReplicasAnnotation, annotation, "replicas"), nil
}

func planCronJob(operation Operation, workload Workload) (mutation, error) {
	current, found, err := nestedBool(workload.Object.Object, "spec", "suspend")
	if err != nil {
		return mutation{}, err
	}
	if !found {
		current = false
	}
	annotations := workload.Annotations()
	snapshot, hasSnapshot := annotations[OriginalSuspendAnnotation]

	var desired bool
	var annotation string
	if operation == Squeeze {
		desired = true
		if current && hasSnapshot {
			if _, err := parseSuspend(snapshot); err != nil {
				return mutation{}, fmt.Errorf("invalid annotation %q: %w", OriginalSuspendAnnotation, err)
			}
			annotation = snapshot
		} else {
			annotation = strconv.FormatBool(current)
		}
	} else {
		if !hasSnapshot {
			return mutation{}, fmt.Errorf("missing annotation %q", OriginalSuspendAnnotation)
		}
		desired, err = parseSuspend(snapshot)
		if err != nil {
			return mutation{}, fmt.Errorf("invalid annotation %q: %w", OriginalSuspendAnnotation, err)
		}
		annotation = snapshot
	}

	changed := desired != current || !hasSnapshot || annotations[OriginalSuspendAnnotation] != annotation
	return buildMutation(workload, current, desired, changed, OriginalSuspendAnnotation, annotation, "suspend"), nil
}

func parseSuspend(value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("value %q must be true or false", value)
	}
}

func buildMutation(workload Workload, previous, current any, changed bool, annotationKey, annotationValue, specField string) mutation {
	status := "unchanged"
	var patch map[string]any
	var annotations map[string]string
	if changed {
		status = "updated"
		annotations = map[string]string{annotationKey: annotationValue}
		metadata := map[string]any{"annotations": map[string]any{annotationKey: annotationValue}}
		if rv := workload.Object.GetResourceVersion(); rv != "" {
			metadata["resourceVersion"] = rv
		}
		patch = map[string]any{
			"metadata": metadata,
			"spec":     map[string]any{specField: current},
		}
	}
	return mutation{workload: workload, patch: patch, change: Change{
		Namespace: workload.Namespace(), Kind: workload.Kind, Name: workload.Name(),
		Previous: previous, Current: current, Status: status,
		Annotations: annotations,
	}}
}

func parseReplicas(value string) (int64, error) {
	replicas, err := strconv.ParseInt(value, 10, 32)
	if err != nil || replicas < 0 {
		return 0, fmt.Errorf("invalid annotation %q value %q", OriginalReplicasAnnotation, value)
	}
	return replicas, nil
}

// Fake/unstructured clients sometimes retain json numbers as int, float64, or
// json.Number. Normalize those representations without weakening validation.
func nestedInt64(object map[string]any, fields ...string) (int64, bool, error) {
	value, found, err := nestedValue(object, fields...)
	if err != nil || !found {
		return 0, found, err
	}
	switch number := value.(type) {
	case int64:
		return number, true, nil
	case int32:
		return int64(number), true, nil
	case int:
		return int64(number), true, nil
	case float64:
		if number != float64(int64(number)) {
			return 0, true, fmt.Errorf("%s must be an integer", fields[len(fields)-1])
		}
		return int64(number), true, nil
	default:
		return 0, true, fmt.Errorf("%s must be an integer", fields[len(fields)-1])
	}
}

func nestedBool(object map[string]any, fields ...string) (bool, bool, error) {
	value, found, err := nestedValue(object, fields...)
	if err != nil || !found {
		return false, found, err
	}
	result, ok := value.(bool)
	if !ok {
		return false, true, fmt.Errorf("%s must be a boolean", fields[len(fields)-1])
	}
	return result, true, nil
}

func nestedValue(object map[string]any, fields ...string) (any, bool, error) {
	current := any(object)
	for _, field := range fields {
		mapping, ok := current.(map[string]any)
		if !ok {
			return nil, false, fmt.Errorf("field containing %q must be an object", field)
		}
		value, ok := mapping[field]
		if !ok || value == nil {
			return nil, false, nil
		}
		current = value
	}
	return current, true, nil
}
