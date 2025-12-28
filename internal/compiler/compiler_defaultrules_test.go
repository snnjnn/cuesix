package compiler

import (
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestDefaultRulesAllowMergeSameID(t *testing.T) {
	root := DefaultMergingRules()
	cases := []struct {
		name  string
		path  []string
		allow bool
	}{
		{"routes", []string{"routes"}, true},
		{"services", []string{"services"}, false},
		{"upstreams", []string{"upstreams"}, false},
		{"ssls", []string{"ssls"}, false},
		{"global_rules", []string{"global_rules"}, false},
		{"consumer_groups", []string{"consumer_groups"}, false},
		{"plugin_configs", []string{"plugin_configs"}, false},
		{"stream_routes", []string{"stream_routes"}, false},
		{"protos", []string{"protos"}, false},
		{"consumers", []string{"consumers"}, true},
		{"plugin_metadata", []string{"plugin_metadata"}, false},
		{"jq", []string{"jq"}, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rule := findRule(t, root, tc.path...)
			left := []any{map[string]any{rule.IDAttr: "same", "a": 1}}
			right := []any{map[string]any{rule.IDAttr: "same", "b": 2}}

			_, err := mergeList(left, right, rule)
			if tc.allow {
				if err != nil {
					t.Fatalf("expected merge to succeed, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected merge to fail for duplicate id")
			}
		})
	}
}

func TestDefaultRulesRoutesOptionalIDDoesNotMerge(t *testing.T) {
	rule := findRule(t, DefaultMergingRules(), "routes")
	left := []any{map[string]any{"uri": "/a"}}
	right := []any{map[string]any{"uri": "/b"}}

	merged, err := mergeList(left, right, rule)
	if err != nil {
		t.Fatalf("mergeList returned error: %v", err)
	}
	if len(merged.([]any)) != 2 {
		t.Fatalf("expected two routes, got %d", len(merged.([]any)))
	}
}

func TestDefaultRulesConsumersMergeCredentials(t *testing.T) {
	root := DefaultMergingRules()
	left := map[string]any{
		"consumers": []any{
			map[string]any{
				"username": "alice",
				"credentials": []any{
					map[string]any{"credential_id": "c1"},
				},
			},
		},
	}
	right := map[string]any{
		"consumers": []any{
			map[string]any{
				"username": "alice",
				"credentials": []any{
					map[string]any{"credential_id": "c2"},
				},
			},
		},
	}

	merged, err := ApplyMergeRules(left, right, root)
	if err != nil {
		t.Fatalf("ApplyMergeRules returned error: %v", err)
	}
	want := map[string]any{
		"consumers": []any{
			map[string]any{
				"username": "alice",
				"credentials": []any{
					map[string]any{"credential_id": "c1"},
					map[string]any{"credential_id": "c2"},
				},
			},
		},
	}
	if diff := cmp.Diff(want, merged); diff != "" {
		t.Fatalf("unexpected merge result (-want +got):\n%s", diff)
	}
}

func TestDefaultRulesConsumersDuplicateCredentialIDFails(t *testing.T) {
	root := DefaultMergingRules()
	left := map[string]any{
		"consumers": []any{
			map[string]any{
				"username": "alice",
				"credentials": []any{
					map[string]any{"credential_id": "c1"},
				},
			},
		},
	}
	right := map[string]any{
		"consumers": []any{
			map[string]any{
				"username": "alice",
				"credentials": []any{
					map[string]any{"credential_id": "c1"},
				},
			},
		},
	}

	_, err := ApplyMergeRules(left, right, root)
	if err == nil {
		t.Fatalf("expected duplicate credential_id error")
	}
	if !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultRulesNumericIDMatchesString(t *testing.T) {
	rule := findRule(t, DefaultMergingRules(), "services")
	left := []any{map[string]any{rule.IDAttr: 1, "name": "left"}}
	right := []any{map[string]any{rule.IDAttr: "1", "name": "right"}}

	_, err := mergeList(left, right, rule)
	if err == nil {
		t.Fatalf("expected duplicate id error for numeric/string match")
	}
	if !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDefaultRulesSSLSNIsMerge(t *testing.T) {
	rule := findRule(t, DefaultMergingRules(), "ssls", "snis")
	left := []any{"a.example.com", "b.example.com"}
	right := []any{"b.example.com", "c.example.com"}

	merged, err := ApplyMergeRules(left, right, rule)
	if err != nil {
		t.Fatalf("ApplyMergeRules returned error: %v", err)
	}
	want := []any{"a.example.com", "b.example.com", "c.example.com"}
	if diff := cmp.Diff(want, merged); diff != "" {
		t.Fatalf("unexpected merge result (-want +got):\n%s", diff)
	}
}

func findRule(t *testing.T, root MergingRule, path ...string) MergingRule {
	t.Helper()
	current := root
	for _, segment := range path {
		child, ok := current.Children[segment]
		if !ok {
			t.Fatalf("missing rule for %s", strings.Join(path, "/"))
		}
		current = child
	}
	return current
}
