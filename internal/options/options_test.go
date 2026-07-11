package options

import (
	"strings"
	"testing"

	"github.com/elft/KubeSqueeze/internal/selection"
)

func validOptions() Options {
	return Options{Operation: OperationSqueeze, Context: "dev", Namespaces: []string{"team-a"}, Include: selection.RuleOptions{NameRegexes: []string{"api-.*"}}}
}

func TestValidate(t *testing.T) {
	if _, err := validOptions().Validate(); err != nil {
		t.Fatalf("valid options failed: %v", err)
	}
}

func TestValidateRejectsUnsafeOrInvalidOptions(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Options)
		want string
	}{
		{"context", func(o *Options) { o.Context = "" }, "--context is required"},
		{"namespace scope missing", func(o *Options) { o.Namespaces = nil }, "exactly one"},
		{"both namespace scopes", func(o *Options) { o.AllNamespaces = true }, "exactly one"},
		{"no inclusion", func(o *Options) { o.Include = selection.RuleOptions{} }, "at least one --include-*"},
		{"duplicate namespace", func(o *Options) { o.Namespaces = []string{"a", "a"} }, "duplicate"},
		{"invalid ignore", func(o *Options) { o.Ignore.NameRegexes = []string{"["} }, "invalid ignore rules"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts := validOptions()
			test.edit(&opts)
			_, err := opts.Validate()
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}
