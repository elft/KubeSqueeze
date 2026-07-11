package kube

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
)

// Discoverer lists supported resources and negotiates the CronJob API served
// by older clusters.
type Discoverer struct {
	dynamic   dynamic.Interface
	discovery discovery.DiscoveryInterface
}

// discoveryPageSize bounds each response returned by the API server. The
// continue token keeps all pages on the same consistent list snapshot.
const discoveryPageSize int64 = 500

func NewDiscoverer(client *Client) *Discoverer {
	return &Discoverer{dynamic: client.Dynamic, discovery: client.Discovery.Discovery()}
}

// NewDiscovererForInterfaces is primarily useful to embedders and tests.
func NewDiscovererForInterfaces(dynamicClient dynamic.Interface, discoveryClient discovery.DiscoveryInterface) *Discoverer {
	return &Discoverer{dynamic: dynamicClient, discovery: discoveryClient}
}

// List lists kinds within explicit namespaces. An empty namespaces slice means
// all namespaces. An empty kinds slice means all supported kinds.
func (d *Discoverer) List(ctx context.Context, namespaces []string, kinds []Kind) ([]Workload, error) {
	requested := normalizeKinds(kinds)
	gvrs := make(map[Kind]schema.GroupVersionResource, len(requested))
	for _, kind := range requested {
		switch kind {
		case Deployment:
			gvrs[kind] = deploymentsGVR
		case StatefulSet:
			gvrs[kind] = statefulSetsGVR
		case CronJob:
			gvr, err := d.cronJobGVR()
			if err != nil {
				return nil, err
			}
			gvrs[kind] = gvr
		default:
			return nil, fmt.Errorf("unsupported kind %q", kind)
		}
	}

	if len(namespaces) == 0 {
		namespaces = []string{metav1.NamespaceAll}
	} else {
		namespaces = uniqueSorted(namespaces)
	}
	var result []Workload
	for _, kind := range requested {
		gvr := gvrs[kind]
		for _, namespace := range namespaces {
			continueToken := ""
			for {
				list, err := d.dynamic.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{
					Limit:    discoveryPageSize,
					Continue: continueToken,
				})
				if err != nil {
					return nil, fmt.Errorf("list %s in namespace %q: %w", kind, namespace, err)
				}
				for i := range list.Items {
					object := list.Items[i].DeepCopy()
					result = append(result, Workload{Kind: kind, GVR: gvr, Object: object})
				}
				continueToken = list.GetContinue()
				if continueToken == "" {
					break
				}
			}
		}
	}
	sortWorkloads(result)
	return result, nil
}

func (d *Discoverer) cronJobGVR() (schema.GroupVersionResource, error) {
	if hasResource(d.discovery, "batch/v1", "cronjobs") {
		return cronJobsV1GVR, nil
	}
	if hasResource(d.discovery, "batch/v1beta1", "cronjobs") {
		return cronJobsBetaGVR, nil
	}
	return schema.GroupVersionResource{}, fmt.Errorf("cluster serves neither batch/v1 nor batch/v1beta1 CronJobs")
}

func hasResource(client discovery.DiscoveryInterface, groupVersion, name string) bool {
	resources, err := client.ServerResourcesForGroupVersion(groupVersion)
	if err != nil || resources == nil {
		return false
	}
	for _, resource := range resources.APIResources {
		if resource.Name == name {
			return true
		}
	}
	return false
}

func normalizeKinds(kinds []Kind) []Kind {
	if len(kinds) == 0 {
		return []Kind{CronJob, Deployment, StatefulSet}
	}
	seen := map[Kind]bool{}
	result := make([]Kind, 0, len(kinds))
	for _, kind := range kinds {
		if !seen[kind] {
			seen[kind] = true
			result = append(result, kind)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
