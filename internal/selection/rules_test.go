package selection

import (
	"reflect"
	"testing"
)

func TestRulesORWithinCategoryANDAcrossCategories(t *testing.T) {
	rules, err := Compile(RuleOptions{NameRegexes: []string{"api-.*", "worker"}, LabelSelectors: []string{"environment in (staging,dev)", "tier=batch"}, Kinds: []string{"deployment"}})
	if err != nil {
		t.Fatal(err)
	}
	match := rules.Match(Resource{Kind: KindDeployment, Name: "api-web", Labels: map[string]string{"environment": "staging"}})
	if !match.Matched {
		t.Fatal("expected resource to match")
	}
	want := []Category{CategoryName, CategoryLabel, CategoryKind}
	if !reflect.DeepEqual(match.Categories, want) {
		t.Fatalf("categories = %#v, want %#v", match.Categories, want)
	}
	if rules.Match(Resource{Kind: KindCronJob, Name: "api-web", Labels: map[string]string{"environment": "staging"}}).Matched {
		t.Fatal("expected kind mismatch")
	}
}

func TestRegexesAreAnchored(t *testing.T) {
	rules, err := Compile(RuleOptions{NameRegexes: []string{"api"}})
	if err != nil {
		t.Fatal(err)
	}
	if !rules.Match(Resource{Name: "api"}).Matched {
		t.Fatal("exact name should match")
	}
	if rules.Match(Resource{Name: "my-api"}).Matched {
		t.Fatal("substring should not match")
	}
}

func TestAnnotationAndOwnerRules(t *testing.T) {
	rules, err := Compile(RuleOptions{AnnotationSelectors: []string{"scheduling.example.com/managed,!protected"}, OwnerRegexes: []string{`apps/v1/Deployment/api-.*`}})
	if err != nil {
		t.Fatal(err)
	}
	resource := Resource{Annotations: map[string]string{"scheduling.example.com/managed": "true"}, Owners: []Owner{{APIVersion: "apps/v1", Kind: "Deployment", Name: "api-main"}}}
	if !rules.Match(resource).Matched {
		t.Fatal("expected annotation and owner to match")
	}
	resource.Annotations["protected"] = "true"
	if rules.Match(resource).Matched {
		t.Fatal("non-existence requirement should fail")
	}
}

func TestCompileRejectsInvalidInput(t *testing.T) {
	tests := []RuleOptions{{NameRegexes: []string{"["}}, {LabelSelectors: []string{"key in ("}}, {Kinds: []string{"daemonset"}}, {OwnerRegexes: []string{""}}}
	for _, test := range tests {
		if _, err := Compile(test); err == nil {
			t.Fatalf("Compile(%#v) succeeded", test)
		}
	}
}

func TestApplyIncludesThenIgnoresAndSorts(t *testing.T) {
	include, err := Compile(RuleOptions{NameRegexes: []string{".*"}, Kinds: []string{"deployment", "cronjob"}})
	if err != nil {
		t.Fatal(err)
	}
	ignore, err := Compile(RuleOptions{NamespaceRegexes: []string{"system"}, LabelSelectors: []string{"protected=true"}})
	if err != nil {
		t.Fatal(err)
	}
	resources := []Resource{
		{Namespace: "team", Kind: KindDeployment, Name: "z"},
		{Namespace: "system", Kind: KindDeployment, Name: "ignored", Labels: map[string]string{"protected": "true"}},
		{Namespace: "system", Kind: KindDeployment, Name: "included", Labels: map[string]string{"protected": "false"}},
		{Namespace: "team", Kind: KindStatefulSet, Name: "not-selected"},
		{Namespace: "team", Kind: KindCronJob, Name: "a"},
	}
	got := Apply(resources, include, ignore)
	if got.Discovered != 5 || len(got.Included) != 3 || len(got.Ignored) != 1 {
		t.Fatalf("unexpected counts: %#v", got)
	}
	wantNames := []string{"included", "a", "z"}
	var gotNames []string
	for _, resource := range got.Included {
		gotNames = append(gotNames, resource.Name)
	}
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("names = %v, want %v", gotNames, wantNames)
	}
	if !reflect.DeepEqual(got.Ignored[0].Categories, []Category{CategoryNamespace, CategoryLabel}) {
		t.Fatalf("categories = %#v", got.Ignored[0].Categories)
	}
}

func TestEmptyIgnoreRulesDoNotIgnore(t *testing.T) {
	include, _ := Compile(RuleOptions{NameRegexes: []string{".*"}})
	ignore, _ := Compile(RuleOptions{})
	result := Apply([]Resource{{Name: "api"}}, include, ignore)
	if len(result.Included) != 1 || len(result.Ignored) != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
}
