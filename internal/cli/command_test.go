package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/elft/KubeSqueeze/internal/options"
	"github.com/elft/KubeSqueeze/internal/output"
)

func TestCommandParsesSymmetricSelectors(t *testing.T) {
	var got options.Validated
	handler := func(_ context.Context, validated options.Validated) (output.Result, error) {
		got = validated
		return output.Result{Operation: "squeeze", Ignored: []output.IgnoredResource{}, Mutated: []output.MutatedResource{}}, nil
	}
	var stdout bytes.Buffer
	command := Command{Out: &stdout, ErrOut: &bytes.Buffer{}, Handler: handler}.New()
	command.SetArgs([]string{"squeeze", "--context", "dev", "--all-namespaces", "--include-label-selector", "environment in (staging,dev)", "--include-kind", "deployment", "--ignore-annotation-selector", "protected=true", "--ignore-name-regex", "system-.*"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.Context != "dev" || !got.AllNamespaces {
		t.Fatalf("unexpected options: %#v", got.Options)
	}
	if len(got.Include.LabelSelectors) != 1 || len(got.Ignore.AnnotationSelectors) != 1 {
		t.Fatalf("selectors not parsed: %#v", got.Options)
	}
	if got.Include.LabelSelectors[0] != "environment in (staging,dev)" {
		t.Fatalf("selector containing comma was split: %#v", got.Include.LabelSelectors)
	}
	want := `{"operation":"squeeze","cluster":{"context":"","server":""},"discovered":0,"included":0,"ignored":[],"mutated":[]}` + "\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
}

func TestCommandValidatesBeforeHandler(t *testing.T) {
	called := false
	command := Command{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}, Handler: func(context.Context, options.Validated) (output.Result, error) {
		called = true
		return output.Result{}, nil
	}}.New()
	command.SetArgs([]string{"restore", "--context", "dev", "--all-namespaces"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "at least one --include-*") {
		t.Fatalf("error = %v", err)
	}
	if called {
		t.Fatal("handler called before validation")
	}
}

func TestCommandParsesDryRun(t *testing.T) {
	var got options.Validated
	command := Command{Out: &bytes.Buffer{}, ErrOut: &bytes.Buffer{}, Handler: func(_ context.Context, validated options.Validated) (output.Result, error) {
		got = validated
		return output.Result{}, nil
	}}.New()
	command.SetArgs([]string{"squeeze", "--context", "dev", "--namespace", "team-a", "--include-kind", "deployment", "--dry-run"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !got.DryRun {
		t.Fatal("--dry-run was not passed to the handler")
	}
}
