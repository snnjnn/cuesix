package compiler

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/google/go-cmp/cmp"
)

func TestMergeListOptionalIDKeepsItems(t *testing.T) {
	rule := MergingRule{
		Path:             "/routes",
		Kind:             KindList,
		IDAttr:           "id",
		IDOptional:       true,
		AllowMergeSameID: false,
	}

	left := []any{
		map[string]any{"uri": "/a"},
		map[string]any{"id": "1", "uri": "/b"},
	}
	right := []any{
		map[string]any{"uri": "/c"},
		map[string]any{"id": "2", "uri": "/d"},
	}

	merged, err := mergeList(left, right, rule)
	if err != nil {
		t.Fatalf("mergeList returned error: %v", err)
	}

	want := []any{
		map[string]any{"uri": "/a"},
		map[string]any{"id": "1", "uri": "/b"},
		map[string]any{"uri": "/c"},
		map[string]any{"id": "2", "uri": "/d"},
	}

	if diff := cmp.Diff(want, merged); diff != "" {
		t.Fatalf("unexpected merge result (-want +got):\n%s", diff)
	}
}

func TestMergeListSameIDWithChildList(t *testing.T) {
	rule := MergingRule{
		Path:             "/consumers",
		Kind:             KindList,
		IDAttr:           "username",
		IDOptional:       false,
		AllowMergeSameID: true,
		Children: map[string]MergingRule{
			"credentials": {
				Path:             "/consumers/credentials",
				Kind:             KindList,
				IDAttr:           "credential_id",
				IDOptional:       false,
				AllowMergeSameID: false,
			},
		},
	}

	left := []any{
		map[string]any{
			"username": "alice",
			"desc":     "primary",
			"credentials": []any{
				map[string]any{
					"credential_id": "c1",
					"plugins": map[string]any{
						"key-auth": map[string]any{"key": "one"},
					},
				},
			},
		},
	}

	right := []any{
		map[string]any{
			"username": "alice",
			"credentials": []any{
				map[string]any{
					"credential_id": "c2",
					"plugins": map[string]any{
						"key-auth": map[string]any{"key": "two"},
					},
				},
			},
		},
	}

	merged, err := mergeList(left, right, rule)
	if err != nil {
		t.Fatalf("mergeList returned error: %v", err)
	}

	want := []any{
		map[string]any{
			"username": "alice",
			"desc":     "primary",
			"credentials": []any{
				map[string]any{
					"credential_id": "c1",
					"plugins": map[string]any{
						"key-auth": map[string]any{"key": "one"},
					},
				},
				map[string]any{
					"credential_id": "c2",
					"plugins": map[string]any{
						"key-auth": map[string]any{"key": "two"},
					},
				},
			},
		},
	}

	if diff := cmp.Diff(want, merged); diff != "" {
		t.Fatalf("unexpected merge result (-want +got):\n%s", diff)
	}
}

func TestMergeListCastsIntegerIDType(t *testing.T) {
	rule := MergingRule{
		Path:             "/routes",
		Kind:             KindList,
		IDAttr:           "id",
		IDOptional:       false,
		AllowMergeSameID: true,
	}

	left := []any{
		map[string]any{"id": 1, "attrib1": "/a"},
	}
	right := []any{
		map[string]any{"id": "1", "attrib2": "/b"},
	}

	merged, err := mergeList(left, right, rule)
	if err != nil {
		t.Fatalf("mergeList returned error: %v", err)
	}

	want := []any{
		map[string]any{"id": "1", "attrib1": "/a", "attrib2": "/b"},
	}
	if diff := cmp.Diff(want, merged); diff != "" {
		t.Fatalf("unexpected merge result (-want +got):\n%s", diff)
	}
}

func TestCompileMergesFilesystems(t *testing.T) {
	fsOne := fstest.MapFS{
		"one.yaml": {
			Data: []byte(`
consumers:
  - username: alice
    desc: primary
    credentials:
      - credential_id: c1
        plugins:
          key-auth:
            key: one
`),
		},
	}
	fsTwo := fstest.MapFS{
		"two.yaml": {
			Data: []byte(`
consumers:
  - username: alice
    credentials:
      - credential_id: c2
        plugins:
          key-auth:
            key: two
`),
		},
	}

	merged, err := Compile(fsOne, fsTwo)
	if err != nil {
		t.Fatalf("Compile returned error: %v", err)
	}

	want := map[string]any{
		"consumers": []any{
			map[string]any{
				"username": "alice",
				"desc":     "primary",
				"credentials": []any{
					map[string]any{
						"credential_id": "c1",
						"plugins": map[string]any{
							"key-auth": map[string]any{"key": "one"},
						},
					},
					map[string]any{
						"credential_id": "c2",
						"plugins": map[string]any{
							"key-auth": map[string]any{"key": "two"},
						},
					},
				},
			},
		},
	}

	if diff := cmp.Diff(want, merged); diff != "" {
		t.Fatalf("unexpected compile result (-want +got):\n%s", diff)
	}
}

func TestMergeMapRejectsNestedMapWithoutRule(t *testing.T) {
	rule := MergingRule{
		Path: "/",
		Kind: KindMap,
	}
	left := map[string]any{
		"meta": map[string]any{"a": 1},
	}
	right := map[string]any{
		"meta": map[string]any{"b": 2},
	}

	_, err := mergeMap(left, right, rule)
	if err == nil {
		t.Fatalf("expected error for missing nested map rule")
	}
	if !strings.Contains(err.Error(), "expected scalar") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMergeMapMergesNestedMapWithRule(t *testing.T) {
	rule := MergingRule{
		Path: "/",
		Kind: KindMap,
		Children: map[string]MergingRule{
			"meta": {
				Path: "/meta",
				Kind: KindMap,
			},
		},
	}
	left := map[string]any{
		"meta": map[string]any{"a": 1},
	}
	right := map[string]any{
		"meta": map[string]any{"b": 2},
	}

	merged, err := mergeMap(left, right, rule)
	if err != nil {
		t.Fatalf("mergeMap returned error: %v", err)
	}
	want := map[string]any{
		"meta": map[string]any{"a": 1, "b": 2},
	}
	if diff := cmp.Diff(want, merged); diff != "" {
		t.Fatalf("unexpected merge result (-want +got):\n%s", diff)
	}
}

func TestMergeScalarList(t *testing.T) {
	rule := MergingRule{
		Path: "/ssls/snis",
		Kind: KindScalarList,
	}
	left := []any{"a.example.com", "b.example.com"}
	right := []any{"b.example.com", "c.example.com"}

	merged, err := mergeScalarList(left, right, rule)
	if err != nil {
		t.Fatalf("mergeScalarList returned error: %v", err)
	}
	want := []any{"a.example.com", "b.example.com", "c.example.com"}
	if diff := cmp.Diff(want, merged); diff != "" {
		t.Fatalf("unexpected merge result (-want +got):\n%s", diff)
	}
}

func TestMergeScalarListRejectsNonScalar(t *testing.T) {
	rule := MergingRule{
		Path: "/ssls/snis",
		Kind: KindScalarList,
	}
	left := []any{map[string]any{"bad": true}}
	right := []any{"ok"}

	_, err := mergeScalarList(left, right, rule)
	if err == nil {
		t.Fatalf("expected error for non-scalar list item")
	}
}

func TestExtractIDMissingRequired(t *testing.T) {
	rule := MergingRule{
		Path:             "/routes",
		Kind:             KindList,
		IDAttr:           "id",
		IDOptional:       false,
		AllowMergeSameID: false,
	}
	item := map[string]any{"uri": "/a"}
	_, _, err := extractID(item, rule)
	if err == nil {
		t.Fatalf("expected error for missing id")
	}
}

func TestApplyMergeRulesTypeMismatch(t *testing.T) {
	rule := MergingRule{Path: "/", Kind: KindScalar}
	_, err := ApplyMergeRules(map[string]any{"a": 1}, []any{"b"}, rule)
	if err == nil {
		t.Fatalf("expected type mismatch error")
	}
	if !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApplyMergeRulesScalarRuleMismatch(t *testing.T) {
	rule := MergingRule{Path: "/a", Kind: KindMap}
	_, err := ApplyMergeRules("left", "right", rule)
	if err == nil {
		t.Fatalf("expected missing scalar merge rule error")
	}
	if !strings.Contains(err.Error(), "missing scalar merge rule") {
		t.Fatalf("unexpected error: %v", err)
	}
}
