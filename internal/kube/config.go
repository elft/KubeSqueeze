package kube

import (
	"context"
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Target identifies the exact kubeconfig context and API server selected for an
// operation. Commands intentionally never fall back to current-context.
type Target struct {
	Context string `json:"context"`
	Server  string `json:"server"`
}

// Preflight verifies that the selected API server is reachable and responds to
// discovery before callers list or mutate workloads.
func (c *Client) Preflight(ctx context.Context) error {
	if _, err := c.Discovery.Discovery().ServerVersion(); err != nil {
		return fmt.Errorf("connect to context %q (%s): %w", c.Target.Context, c.Target.Server, err)
	}
	return nil
}

// Config contains the user-controlled cluster selection inputs.
type Config struct {
	Kubeconfig string
	Context    string
}

// Client holds the clients needed by discovery and mutation operations.
type Client struct {
	Dynamic   dynamic.Interface
	Discovery kubernetes.Interface
	Target    Target
}

// RESTConfig resolves an explicitly named context using normal kubeconfig
// loading rules, optionally restricted to one kubeconfig file.
func RESTConfig(input Config) (*rest.Config, Target, error) {
	if input.Context == "" {
		return nil, Target{}, fmt.Errorf("context is required")
	}

	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	if input.Kubeconfig != "" {
		rules.ExplicitPath = input.Kubeconfig
	}
	raw, err := rules.Load()
	if err != nil {
		return nil, Target{}, fmt.Errorf("load kubeconfig: %w", err)
	}
	context, ok := raw.Contexts[input.Context]
	if !ok || context == nil {
		return nil, Target{}, fmt.Errorf("context %q does not exist", input.Context)
	}
	cluster, ok := raw.Clusters[context.Cluster]
	if !ok || cluster == nil {
		return nil, Target{}, fmt.Errorf("context %q references missing cluster %q", input.Context, context.Cluster)
	}
	if cluster.Server == "" {
		return nil, Target{}, fmt.Errorf("cluster %q referenced by context %q has no API server", context.Cluster, input.Context)
	}

	overrides := &clientcmd.ConfigOverrides{CurrentContext: input.Context}
	restConfig, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides).ClientConfig()
	if err != nil {
		return nil, Target{}, fmt.Errorf("resolve context %q: %w", input.Context, err)
	}
	return restConfig, Target{Context: input.Context, Server: cluster.Server}, nil
}

// NewClient creates clients for an explicitly selected target.
func NewClient(input Config) (*Client, error) {
	restConfig, target, err := RESTConfig(input)
	if err != nil {
		return nil, err
	}
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("create discovery client: %w", err)
	}
	return &Client{Dynamic: dynamicClient, Discovery: clientset, Target: target}, nil
}
