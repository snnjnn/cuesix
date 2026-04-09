package compiler_test

import (
	"reflect"
	"testing"

	"github.com/warpcondev/cuesix/internal/compiler"
)

func TestMergingRuleAsTree(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		rule    compiler.MergingRule
		snippet compiler.Snippet
		want    map[string]map[string]map[string]any
	}{
		{
			name: "collects objects by id for list children",
			rule: compiler.MergingRule{
				Children: map[string]compiler.MergingRule{
					"routes": {Kind: compiler.KindList, IDAttr: "id"},
				},
			},
			snippet: compiler.Snippet{
				Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW),
				Data: map[string]any{
					"routes": []any{
						map[string]any{"id": "r1"},
						map[string]any{"id": "r2"},
					},
				}},
			want: map[string]map[string]map[string]any{
				"routes": {
					"r1": {"id": "r1"},
					"r2": {"id": "r2"},
				},
			},
		},
		{
			name: "ignores non list children and malformed items",
			rule: compiler.MergingRule{
				Children: map[string]compiler.MergingRule{
					"routes":   {Kind: compiler.KindList, IDAttr: "id"},
					"services": {Kind: compiler.KindMap, IDAttr: "id"},
				},
			},
			snippet: compiler.Snippet{
				Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW),
				Data: map[string]any{
					"routes": []any{
						"not-a-map",
						map[string]any{"name": "missing-id"},
						map[string]any{"id": 10},
						map[string]any{"id": "ok"},
					},
					"services": []any{map[string]any{"id": "svc1"}},
					"unknown":  []any{map[string]any{"id": "x"}},
				}},
			want: map[string]map[string]map[string]any{
				"routes": {
					"10": {"id": 10},
					"ok": {"id": "ok"},
				},
			},
		},
		{
			name: "returns empty map when child value is not list",
			rule: compiler.MergingRule{
				Children: map[string]compiler.MergingRule{
					"routes": {Kind: compiler.KindList, IDAttr: "id"},
				},
			},
			snippet: compiler.Snippet{
				Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: map[string]any{
					"routes": map[string]any{"id": "r1"},
				}},
			want: map[string]map[string]map[string]any{},
		},
		{
			name: "collects plugin_metadata by id for list children",
			rule: compiler.MergingRule{
				Children: map[string]compiler.MergingRule{
					"plugin_metadata": {Kind: compiler.KindList, IDAttr: "id"},
				},
			},
			snippet: compiler.Snippet{
				Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW),
				Data: map[string]any{
					"plugin_metadata": []any{
						map[string]any{"id": "limit-count", "policy": "local"},
						"not-a-map",
						map[string]any{"policy": "missing-id"},
					},
				}},
			want: map[string]map[string]map[string]any{
				"plugin_metadata": {
					"limit-count": {"id": "limit-count", "policy": "local"},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := tt.rule.AsTree(tt.snippet)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("AsTree() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
