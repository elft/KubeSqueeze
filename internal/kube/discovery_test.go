package kube

import (
	"context"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	clienttesting "k8s.io/client-go/testing"
)

var getOptions = metav1.GetOptions{}

func TestDiscovererNegotiatesBetaCronJobsAndSorts(t *testing.T) {
	one := workloadObject("batch/v1beta1", "CronJob", "z-job", "b", map[string]any{})
	two := workloadObject("batch/v1beta1", "CronJob", "a-job", "a", map[string]any{})
	listKinds := map[schema.GroupVersionResource]string{cronJobsBetaGVR: "CronJobList"}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, one, two)
	discoveryClient := &fake.FakeDiscovery{Fake: &clienttesting.Fake{Resources: []*metav1.APIResourceList{{
		GroupVersion: "batch/v1beta1", APIResources: []metav1.APIResource{{Name: "cronjobs"}},
	}}}}

	items, err := NewDiscovererForInterfaces(dynamicClient, discoveryClient).List(context.Background(), nil, []Kind{CronJob})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Namespace() != "a" || items[1].Namespace() != "b" {
		t.Fatalf("unexpected order: %#v", items)
	}
	if items[0].GVR != cronJobsBetaGVR {
		t.Fatalf("GVR = %s, want %s", items[0].GVR, cronJobsBetaGVR)
	}
}

func TestDiscovererErrorsWhenNoCronJobAPIIsServed(t *testing.T) {
	dynamicClient := dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	discoveryClient := &fake.FakeDiscovery{Fake: &clienttesting.Fake{}}
	_, err := NewDiscovererForInterfaces(dynamicClient, discoveryClient).List(context.Background(), nil, []Kind{CronJob})
	if err == nil {
		t.Fatal("expected CronJob discovery error")
	}
}

func TestDiscovererPaginatesLists(t *testing.T) {
	one := workloadObject("apps/v1", "Deployment", "one", "team-a", map[string]any{})
	two := workloadObject("apps/v1", "Deployment", "two", "team-a", map[string]any{})
	listKinds := map[schema.GroupVersionResource]string{deploymentsGVR: "DeploymentList"}
	dynamicClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds)
	var options []metav1.ListOptions
	dynamicClient.PrependReactor("list", "deployments", func(action clienttesting.Action) (bool, runtime.Object, error) {
		listAction := action.(clienttesting.ListActionImpl)
		opts := listAction.GetListOptions()
		options = append(options, opts)
		if opts.Continue == "" {
			return true, &unstructured.UnstructuredList{
				Object: map[string]any{"metadata": map[string]any{"continue": "next-page"}},
				Items:  []unstructured.Unstructured{*one},
			}, nil
		}
		return true, &unstructured.UnstructuredList{Items: []unstructured.Unstructured{*two}}, nil
	})

	items, err := NewDiscovererForInterfaces(dynamicClient, &fake.FakeDiscovery{Fake: &clienttesting.Fake{}}).List(
		context.Background(), []string{"team-a"}, []Kind{Deployment},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if len(options) != 2 || options[0].Limit != discoveryPageSize || options[0].Continue != "" || options[1].Limit != discoveryPageSize || options[1].Continue != "next-page" {
		t.Fatalf("list options = %#v", options)
	}
}
