package compiler

import (
	"reflect"
	"testing"
)

func TestRouteTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		route map[string]any
		want  map[string][]string
	}{
		{
			name: "collects scalar and list tags",
			route: map[string]any{
				"host":  "api.example.com",
				"hosts": []any{"a.example.com", "b.example.com"},
				"uri":   "/v1",
				"uris":  []any{"/foo", "/bar"},
			},
			want: map[string][]string{
				"hosts": {"a.example.com", "b.example.com", "api.example.com"},
				"uris":  {"/foo", "/bar", "/v1"},
			},
		},
		{
			name: "skips malformed values instead of panicking",
			route: map[string]any{
				"host":  123,
				"hosts": []any{"ok.example.com", 99, true},
				"uri":   []any{"/wrong"},
				"uris":  "not-a-list",
			},
			want: map[string][]string{
				"hosts": {"ok.example.com"},
			},
		},
		{
			name: "returns empty map when no tags are usable",
			route: map[string]any{
				"hosts": map[string]any{"bad": "shape"},
				"uris":  []any{1, false},
			},
			want: map[string][]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := routeTags(tt.route); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("routeTags() = %#v, want %#v", got, tt.want)
			}
		})
	}
}
