package kube

import (
	"fmt"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Kind is a supported top-level workload kind.
type Kind string

const (
	Deployment  Kind = "deployment"
	StatefulSet Kind = "statefulset"
	CronJob     Kind = "cronjob"
)

var (
	deploymentsGVR  = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	statefulSetsGVR = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	cronJobsV1GVR   = schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}
	cronJobsBetaGVR = schema.GroupVersionResource{Group: "batch", Version: "v1beta1", Resource: "cronjobs"}
)

// Workload is the version-neutral representation consumed by filters and the
// mutation engine. Object is a snapshot from discovery and is not mutated.
type Workload struct {
	Kind   Kind
	GVR    schema.GroupVersionResource
	Object *unstructured.Unstructured
}

func (w Workload) Namespace() string               { return w.Object.GetNamespace() }
func (w Workload) Name() string                    { return w.Object.GetName() }
func (w Workload) Labels() map[string]string       { return w.Object.GetLabels() }
func (w Workload) Annotations() map[string]string  { return w.Object.GetAnnotations() }
func (w Workload) Owners() []metav1.OwnerReference { return w.Object.GetOwnerReferences() }

// ParseKind converts the public CLI spelling to a supported kind.
func ParseKind(value string) (Kind, error) {
	switch Kind(strings.ToLower(value)) {
	case Deployment:
		return Deployment, nil
	case StatefulSet:
		return StatefulSet, nil
	case CronJob:
		return CronJob, nil
	default:
		return "", fmt.Errorf("unsupported kind %q", value)
	}
}

func sortWorkloads(items []Workload) {
	sort.Slice(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if left.Namespace() != right.Namespace() {
			return left.Namespace() < right.Namespace()
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.Name() < right.Name()
	})
}
