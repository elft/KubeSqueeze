package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/elft/KubeSqueeze/internal/options"
	"github.com/elft/KubeSqueeze/internal/output"
	"github.com/elft/KubeSqueeze/internal/selection"
	"github.com/spf13/cobra"
)

var ErrNoHandler = errors.New("Kubernetes operation handler is not configured")

type Handler func(context.Context, options.Validated) (output.Result, error)

type Command struct {
	Out     io.Writer
	ErrOut  io.Writer
	Handler Handler
	Version string
}

func (c Command) New() *cobra.Command {
	root := &cobra.Command{
		Use:           "kubesqueeze",
		Short:         "Temporarily scale Kubernetes workloads down and restore them",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       c.Version,
	}
	root.SetOut(c.Out)
	root.SetErr(c.ErrOut)
	root.AddCommand(c.operationCommand(options.OperationSqueeze), c.operationCommand(options.OperationRestore))
	return root
}

func (c Command) operationCommand(operation options.Operation) *cobra.Command {
	var opts options.Options
	opts.Operation = operation
	command := &cobra.Command{
		Use:   string(operation),
		Short: map[options.Operation]string{options.OperationSqueeze: "Save state and scale matching workloads down", options.OperationRestore: "Restore matching workloads to saved state"}[operation],
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			validated, err := opts.Validate()
			if err != nil {
				return err
			}
			if c.Handler == nil {
				return ErrNoHandler
			}
			result, err := c.Handler(cmd.Context(), validated)
			if err != nil {
				return err
			}
			return output.Write(c.Out, result)
		},
	}

	flags := command.Flags()
	flags.StringVar(&opts.Kubeconfig, "kubeconfig", "", "path to kubeconfig (uses standard loading rules when omitted)")
	flags.StringVar(&opts.Context, "context", "", "kubeconfig context to target (required)")
	flags.StringArrayVar(&opts.Namespaces, "namespace", nil, "namespace safety boundary (repeatable)")
	flags.BoolVar(&opts.AllNamespaces, "all-namespaces", false, "allow selection across every namespace")
	flags.BoolVar(&opts.DryRun, "dry-run", false, "print planned changes and annotations without modifying workloads")
	addRuleFlags(flags, "include", &opts.Include)
	addRuleFlags(flags, "ignore", &opts.Ignore)
	return command
}

type flagSet interface {
	StringArrayVar(*[]string, string, []string, string)
}

func addRuleFlags(flags flagSet, prefix string, rules *selection.RuleOptions) {
	flags.StringArrayVar(&rules.NameRegexes, prefix+"-name-regex", nil, fmt.Sprintf("anchored RE2 %s name expression (repeatable)", prefix))
	flags.StringArrayVar(&rules.NamespaceRegexes, prefix+"-namespace-regex", nil, fmt.Sprintf("anchored RE2 %s namespace expression (repeatable)", prefix))
	flags.StringArrayVar(&rules.LabelSelectors, prefix+"-label-selector", nil, fmt.Sprintf("Kubernetes %s label selector (repeatable)", prefix))
	flags.StringArrayVar(&rules.AnnotationSelectors, prefix+"-annotation-selector", nil, fmt.Sprintf("Kubernetes-style %s annotation selector (repeatable)", prefix))
	flags.StringArrayVar(&rules.Kinds, prefix+"-kind", nil, fmt.Sprintf("%s deployment, statefulset, or cronjob (repeatable)", prefix))
	flags.StringArrayVar(&rules.OwnerRegexes, prefix+"-owner-regex", nil, fmt.Sprintf("anchored RE2 %s apiVersion/kind/name owner expression (repeatable)", prefix))
}
