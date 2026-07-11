package selection

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/labels"
)

// Category names are part of the stable JSON explanation for ignored objects.
type Category string

const (
	CategoryName       Category = "name"
	CategoryNamespace  Category = "namespace"
	CategoryLabel      Category = "label"
	CategoryAnnotation Category = "annotation"
	CategoryKind       Category = "kind"
	CategoryOwner      Category = "owner"
)

// RuleOptions contains the repeatable public flags for either an inclusion or
// ignore rule set. Entries within a category use OR; populated categories use
// AND across categories.
type RuleOptions struct {
	NameRegexes         []string
	NamespaceRegexes    []string
	LabelSelectors      []string
	AnnotationSelectors []string
	Kinds               []string
	OwnerRegexes        []string
}

func (o RuleOptions) Empty() bool {
	return len(o.NameRegexes) == 0 && len(o.NamespaceRegexes) == 0 &&
		len(o.LabelSelectors) == 0 && len(o.AnnotationSelectors) == 0 &&
		len(o.Kinds) == 0 && len(o.OwnerRegexes) == 0
}

type Rules struct {
	names       []*regexp.Regexp
	namespaces  []*regexp.Regexp
	labels      []labels.Selector
	annotations []labels.Selector
	kinds       map[Kind]struct{}
	owners      []*regexp.Regexp
}

// Compile validates all expressions up front, before a caller can mutate a
// resource. Regexes match the complete field value.
func Compile(o RuleOptions) (Rules, error) {
	var out Rules
	var err error
	if out.names, err = compileRegexes("name", o.NameRegexes); err != nil {
		return Rules{}, err
	}
	if out.namespaces, err = compileRegexes("namespace", o.NamespaceRegexes); err != nil {
		return Rules{}, err
	}
	if out.owners, err = compileRegexes("owner", o.OwnerRegexes); err != nil {
		return Rules{}, err
	}
	if out.labels, err = compileSelectors("label", o.LabelSelectors); err != nil {
		return Rules{}, err
	}
	if out.annotations, err = compileSelectors("annotation", o.AnnotationSelectors); err != nil {
		return Rules{}, err
	}
	if len(o.Kinds) > 0 {
		out.kinds = make(map[Kind]struct{}, len(o.Kinds))
		for _, raw := range o.Kinds {
			kind, kindErr := ParseKind(raw)
			if kindErr != nil {
				return Rules{}, kindErr
			}
			out.kinds[kind] = struct{}{}
		}
	}
	return out, nil
}

func ParseKind(raw string) (Kind, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(KindDeployment), "deployments":
		return KindDeployment, nil
	case string(KindStatefulSet), "statefulsets":
		return KindStatefulSet, nil
	case string(KindCronJob), "cronjobs":
		return KindCronJob, nil
	default:
		return "", fmt.Errorf("unsupported kind %q (expected deployment, statefulset, or cronjob)", raw)
	}
}

func compileRegexes(category string, expressions []string) ([]*regexp.Regexp, error) {
	result := make([]*regexp.Regexp, 0, len(expressions))
	for _, expression := range expressions {
		if expression == "" {
			return nil, fmt.Errorf("%s regex cannot be empty", category)
		}
		compiled, err := regexp.Compile("^(?:" + expression + ")$")
		if err != nil {
			return nil, fmt.Errorf("invalid %s regex %q: %w", category, expression, err)
		}
		result = append(result, compiled)
	}
	return result, nil
}

func compileSelectors(category string, expressions []string) ([]labels.Selector, error) {
	result := make([]labels.Selector, 0, len(expressions))
	for _, expression := range expressions {
		if expression == "" {
			return nil, fmt.Errorf("%s selector cannot be empty", category)
		}
		selector, err := labels.Parse(expression)
		if err != nil {
			return nil, fmt.Errorf("invalid %s selector %q: %w", category, expression, err)
		}
		result = append(result, selector)
	}
	return result, nil
}

func (r Rules) Empty() bool {
	return len(r.names) == 0 && len(r.namespaces) == 0 && len(r.labels) == 0 &&
		len(r.annotations) == 0 && len(r.kinds) == 0 && len(r.owners) == 0
}

// Match returns whether all populated categories match and the categories that
// did match. For empty rules Matched is false, which makes an empty ignore set
// naturally include everything it receives.
func (r Rules) Match(resource Resource) Match {
	if r.Empty() {
		return Match{}
	}
	checks := []struct {
		active   bool
		category Category
		matched  bool
	}{
		{len(r.names) > 0, CategoryName, matchRegex(r.names, resource.Name)},
		{len(r.namespaces) > 0, CategoryNamespace, matchRegex(r.namespaces, resource.Namespace)},
		{len(r.labels) > 0, CategoryLabel, matchSelector(r.labels, resource.Labels)},
		{len(r.annotations) > 0, CategoryAnnotation, matchSelector(r.annotations, resource.Annotations)},
		{len(r.kinds) > 0, CategoryKind, matchKind(r.kinds, resource.Kind)},
		{len(r.owners) > 0, CategoryOwner, matchOwners(r.owners, resource.Owners)},
	}

	categories := make([]Category, 0, len(checks))
	for _, check := range checks {
		if !check.active {
			continue
		}
		if !check.matched {
			return Match{}
		}
		categories = append(categories, check.category)
	}
	return Match{Matched: true, Categories: categories}
}

type Match struct {
	Matched    bool
	Categories []Category
}

func matchRegex(regexes []*regexp.Regexp, value string) bool {
	for _, expression := range regexes {
		if expression.MatchString(value) {
			return true
		}
	}
	return false
}

func matchSelector(selectors []labels.Selector, values map[string]string) bool {
	set := labels.Set(values)
	for _, selector := range selectors {
		if selector.Matches(set) {
			return true
		}
	}
	return false
}

func matchKind(kinds map[Kind]struct{}, value Kind) bool {
	_, ok := kinds[value]
	return ok
}

func matchOwners(regexes []*regexp.Regexp, owners []Owner) bool {
	for _, owner := range owners {
		if matchRegex(regexes, owner.Canonical()) {
			return true
		}
	}
	return false
}

type Ignored struct {
	Resource   Resource
	Categories []Category
}

type Result struct {
	Discovered int
	Included   []Resource
	Ignored    []Ignored
}

// Apply sorts its input deterministically, then evaluates inclusion before
// exclusion. Ignore rules always win.
func Apply(resources []Resource, include, ignore Rules) Result {
	ordered := append([]Resource(nil), resources...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].Namespace != ordered[j].Namespace {
			return ordered[i].Namespace < ordered[j].Namespace
		}
		if ordered[i].Kind != ordered[j].Kind {
			return ordered[i].Kind < ordered[j].Kind
		}
		return ordered[i].Name < ordered[j].Name
	})

	result := Result{Discovered: len(ordered)}
	for _, resource := range ordered {
		if !include.Match(resource).Matched {
			continue
		}
		ignored := ignore.Match(resource)
		if ignored.Matched {
			result.Ignored = append(result.Ignored, Ignored{Resource: resource, Categories: ignored.Categories})
			continue
		}
		result.Included = append(result.Included, resource)
	}
	return result
}
