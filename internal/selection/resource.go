package selection

// Kind is a workload kind supported by KubeSqueeze.
type Kind string

const (
	KindDeployment  Kind = "deployment"
	KindStatefulSet Kind = "statefulset"
	KindCronJob     Kind = "cronjob"
)

var SupportedKinds = []Kind{KindDeployment, KindStatefulSet, KindCronJob}

// Owner identifies a Kubernetes ownerReference. UID and controller status do
// not participate in filtering because the public selector is intentionally
// based on the portable apiVersion/kind/name representation.
type Owner struct {
	APIVersion string
	Kind       string
	Name       string
}

func (o Owner) Canonical() string {
	return o.APIVersion + "/" + o.Kind + "/" + o.Name
}

// Resource is the version-neutral metadata required by selection. API-specific
// objects remain owned by the discovery and mutation layers.
type Resource struct {
	APIVersion  string
	Kind        Kind
	Namespace   string
	Name        string
	Labels      map[string]string
	Annotations map[string]string
	Owners      []Owner
	Object      any
}
