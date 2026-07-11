package kube

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/fake"
)

func TestEngineSqueezeAndRestoreReplicas(t *testing.T) {
	object := workloadObject("apps/v1", "Deployment", "web", "team-a", map[string]any{"replicas": int64(3)})
	client := fake.NewSimpleDynamicClient(runtime.NewScheme(), object)
	engine := NewEngineForInterface(client)
	workload := Workload{Kind: Deployment, GVR: deploymentsGVR, Object: object.DeepCopy()}

	changes, err := engine.Apply(context.Background(), Squeeze, []Workload{workload})
	if err != nil {
		t.Fatalf("squeeze: %v", err)
	}
	if got := changes[0].Current; got != int64(0) {
		t.Fatalf("current = %#v, want 0", got)
	}
	squeezed, err := client.Resource(deploymentsGVR).Namespace("team-a").Get(context.Background(), "web", getOptions)
	if err != nil {
		t.Fatal(err)
	}
	assertIntField(t, squeezed, 0, "spec", "replicas")
	if got := squeezed.GetAnnotations()[OriginalReplicasAnnotation]; got != "3" {
		t.Fatalf("snapshot = %q, want 3", got)
	}

	// Repeated squeeze must retain the original snapshot.
	if _, err := engine.Apply(context.Background(), Squeeze, []Workload{{Kind: Deployment, GVR: deploymentsGVR, Object: squeezed}}); err != nil {
		t.Fatalf("repeated squeeze: %v", err)
	}
	afterRepeat, _ := client.Resource(deploymentsGVR).Namespace("team-a").Get(context.Background(), "web", getOptions)
	if got := afterRepeat.GetAnnotations()[OriginalReplicasAnnotation]; got != "3" {
		t.Fatalf("snapshot after repeat = %q, want 3", got)
	}

	if _, err := engine.Apply(context.Background(), Restore, []Workload{{Kind: Deployment, GVR: deploymentsGVR, Object: afterRepeat}}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restored, _ := client.Resource(deploymentsGVR).Namespace("team-a").Get(context.Background(), "web", getOptions)
	assertIntField(t, restored, 3, "spec", "replicas")
	if got := restored.GetAnnotations()[OriginalReplicasAnnotation]; got != "3" {
		t.Fatalf("restore removed snapshot: %q", got)
	}
}

func TestEngineCronJobNilSuspendAndRepeatRestore(t *testing.T) {
	object := workloadObject("batch/v1beta1", "CronJob", "nightly", "team-a", map[string]any{})
	client := fake.NewSimpleDynamicClient(runtime.NewScheme(), object)
	engine := NewEngineForInterface(client)
	workload := Workload{Kind: CronJob, GVR: cronJobsBetaGVR, Object: object.DeepCopy()}

	if _, err := engine.Apply(context.Background(), Squeeze, []Workload{workload}); err != nil {
		t.Fatal(err)
	}
	squeezed, _ := client.Resource(cronJobsBetaGVR).Namespace("team-a").Get(context.Background(), "nightly", getOptions)
	assertBoolField(t, squeezed, true, "spec", "suspend")
	if got := squeezed.GetAnnotations()[OriginalSuspendAnnotation]; got != "false" {
		t.Fatalf("snapshot = %q, want false", got)
	}

	if _, err := engine.Apply(context.Background(), Restore, []Workload{{Kind: CronJob, GVR: cronJobsBetaGVR, Object: squeezed}}); err != nil {
		t.Fatal(err)
	}
	restored, _ := client.Resource(cronJobsBetaGVR).Namespace("team-a").Get(context.Background(), "nightly", getOptions)
	assertBoolField(t, restored, false, "spec", "suspend")
	changes, err := engine.Apply(context.Background(), Restore, []Workload{{Kind: CronJob, GVR: cronJobsBetaGVR, Object: restored}})
	if err != nil {
		t.Fatal(err)
	}
	if changes[0].Status != "unchanged" {
		t.Fatalf("status = %q, want unchanged", changes[0].Status)
	}
}

func TestEngineValidatesAllResourcesBeforePatching(t *testing.T) {
	valid := workloadObject("apps/v1", "Deployment", "a", "team-a", map[string]any{"replicas": int64(4)})
	invalid := workloadObject("apps/v1", "Deployment", "b", "team-a", map[string]any{"replicas": "many"})
	client := fake.NewSimpleDynamicClient(runtime.NewScheme(), valid, invalid)
	engine := NewEngineForInterface(client)
	_, err := engine.Apply(context.Background(), Squeeze, []Workload{
		{Kind: Deployment, GVR: deploymentsGVR, Object: valid},
		{Kind: Deployment, GVR: deploymentsGVR, Object: invalid},
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	unchanged, _ := client.Resource(deploymentsGVR).Namespace("team-a").Get(context.Background(), "a", getOptions)
	assertIntField(t, unchanged, 4, "spec", "replicas")
}

func workloadObject(apiVersion, kind, name, namespace string, spec map[string]any) *unstructured.Unstructured {
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata": map[string]any{
			"name": name, "namespace": namespace,
		},
		"spec": spec,
	}}
}

func assertIntField(t *testing.T, object *unstructured.Unstructured, want int64, fields ...string) {
	t.Helper()
	got, found, err := unstructured.NestedInt64(object.Object, fields...)
	if err != nil || !found || got != want {
		t.Fatalf("field %v = %d, found=%v, err=%v; want %d", fields, got, found, err, want)
	}
}

func assertBoolField(t *testing.T, object *unstructured.Unstructured, want bool, fields ...string) {
	t.Helper()
	got, found, err := unstructured.NestedBool(object.Object, fields...)
	if err != nil || !found || got != want {
		t.Fatalf("field %v = %v, found=%v, err=%v; want %v", fields, got, found, err, want)
	}
}
