package options

import (
	"fmt"
	"strings"

	"github.com/elft/KubeSqueeze/internal/selection"
)

type Operation string

const (
	OperationSqueeze Operation = "squeeze"
	OperationRestore Operation = "restore"
)

type Options struct {
	Operation     Operation
	Kubeconfig    string
	Context       string
	Namespaces    []string
	AllNamespaces bool
	DryRun        bool
	Include       selection.RuleOptions
	Ignore        selection.RuleOptions
}

type Validated struct {
	Options
	IncludeRules selection.Rules
	IgnoreRules  selection.Rules
}

// Validate performs all local validation. Cluster/context existence is checked
// by the Kubernetes client layer after kubeconfig loading.
func (o Options) Validate() (Validated, error) {
	if o.Operation != OperationSqueeze && o.Operation != OperationRestore {
		return Validated{}, fmt.Errorf("unsupported operation %q", o.Operation)
	}
	if strings.TrimSpace(o.Context) == "" {
		return Validated{}, fmt.Errorf("--context is required")
	}
	if o.AllNamespaces == (len(o.Namespaces) > 0) {
		return Validated{}, fmt.Errorf("exactly one of --namespace or --all-namespaces is required")
	}
	seen := make(map[string]struct{}, len(o.Namespaces))
	for _, namespace := range o.Namespaces {
		if strings.TrimSpace(namespace) == "" {
			return Validated{}, fmt.Errorf("--namespace cannot be empty")
		}
		if _, exists := seen[namespace]; exists {
			return Validated{}, fmt.Errorf("duplicate --namespace %q", namespace)
		}
		seen[namespace] = struct{}{}
	}
	if o.Include.Empty() {
		return Validated{}, fmt.Errorf("at least one --include-* selector is required")
	}
	include, err := selection.Compile(o.Include)
	if err != nil {
		return Validated{}, fmt.Errorf("invalid inclusion rules: %w", err)
	}
	ignore, err := selection.Compile(o.Ignore)
	if err != nil {
		return Validated{}, fmt.Errorf("invalid ignore rules: %w", err)
	}
	return Validated{Options: o, IncludeRules: include, IgnoreRules: ignore}, nil
}
