package plugin_test

import (
	"testing"

	"github.com/warpcomdev/cuesix/internal/plugin"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

func TestManagedLabelsPluginUpdate(t *testing.T) {
	t.Parallel()

	p := &plugin.ManagedLabelsPlugin{VirtualGateway: "edge"}
	value := map[string]any{
		"routes": []any{
			map[string]any{
				"id": "r1",
				"labels": map[string]any{
					"team": "api",
				},
			},
		},
		"plugin_metadata": []any{
			map[string]any{
				"id": "limit-count",
			},
		},
	}

	out, err := p.Update(testutil.Logger(), value)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	routes := out["routes"].([]any)
	route := routes[0].(map[string]any)
	labels := route["labels"].(map[string]any)
	if labels["team"] != "api" {
		t.Fatalf("existing label lost: %v", labels)
	}
	if labels[plugin.ManagedByLabelKey] != plugin.ManagedByLabelValue {
		t.Fatalf("missing managed-by label: %v", labels)
	}
	if labels[plugin.SixpackLabelKey] != "edge" {
		t.Fatalf("missing sixpack-label: %v", labels)
	}

	metadata := out["plugin_metadata"].([]any)
	if _, ok := metadata[0].(map[string]any)["labels"]; ok {
		t.Fatalf("plugin_metadata must not receive labels")
	}
}

func TestManagedLabelsPluginUpdateInvalidLabels(t *testing.T) {
	t.Parallel()

	p := &plugin.ManagedLabelsPlugin{VirtualGateway: "edge"}
	_, err := p.Update(testutil.Logger(), map[string]any{
		"routes": []any{
			map[string]any{
				"id":     "r1",
				"labels": "bad",
			},
		},
	})
	if err == nil {
		t.Fatalf("expected error for invalid labels")
	}
}
