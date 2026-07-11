package app

import (
	"testing"

	"github.com/elft/KubeSqueeze/internal/kube"
	"github.com/elft/KubeSqueeze/internal/selection"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDiscoveryKindsNormalizesAndDeduplicates(t *testing.T) {
	kinds, err := discoveryKinds([]string{"deployments", "deployment", "CronJob"})
	if err != nil {
		t.Fatalf("discoveryKinds: %v", err)
	}
	if len(kinds) != 2 || kinds[0] != kube.Deployment || kinds[1] != kube.CronJob {
		t.Fatalf("unexpected kinds: %#v", kinds)
	}
	if _, err := discoveryKinds([]string{"daemonset"}); err == nil {
		t.Fatal("expected unsupported kind error")
	}
}

func TestSelectionResourcePreservesMetadataAndWorkload(t *testing.T) {
	object := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       "Deployment",
		"metadata": map[string]any{
			"namespace":   "team-a",
			"name":        "api",
			"labels":      map[string]any{"environment": "test"},
			"annotations": map[string]any{"managed": "true"},
			"ownerReferences": []any{map[string]any{
				"apiVersion": "example.io/v1", "kind": "Environment", "name": "test",
			}},
		},
	}}
	workload := kube.Workload{Kind: kube.Deployment, Object: object}
	resource := selectionResource(workload)
	if resource.APIVersion != "apps/v1" || resource.Kind != selection.KindDeployment ||
		resource.Namespace != "team-a" || resource.Name != "api" {
		t.Fatalf("unexpected resource identity: %#v", resource)
	}
	if resource.Labels["environment"] != "test" || resource.Annotations["managed"] != "true" {
		t.Fatalf("metadata was not preserved: %#v", resource)
	}
	if len(resource.Owners) != 1 || resource.Owners[0].Canonical() != "example.io/v1/Environment/test" {
		t.Fatalf("owner was not preserved: %#v", resource.Owners)
	}
	if _, ok := resource.Object.(kube.Workload); !ok {
		t.Fatalf("workload link was not preserved: %T", resource.Object)
	}
}

func TestStateFor(t *testing.T) {
	replicas, err := stateFor(kube.StatefulSet, int64(3))
	if err != nil || replicas.Replicas == nil || *replicas.Replicas != 3 || replicas.Suspend != nil {
		t.Fatalf("unexpected replica state: %#v, %v", replicas, err)
	}
	suspend, err := stateFor(kube.CronJob, true)
	if err != nil || suspend.Suspend == nil || !*suspend.Suspend || suspend.Replicas != nil {
		t.Fatalf("unexpected suspend state: %#v, %v", suspend, err)
	}
	if _, err := stateFor(kube.Deployment, "3"); err == nil {
		t.Fatal("expected invalid state error")
	}
}
