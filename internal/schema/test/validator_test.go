package schema_test

import (
	"os"
	"testing"

	"github.com/warpcomdev/sixpack/internal/schema"
	"go.yaml.in/yaml/v4"
)

func TestApplyDefaults_AddsPolicyDefaultToLimitReqPlugin(t *testing.T) {
	normalized, err := os.ReadFile("processed_schema.json")
	if err != nil {
		t.Fatalf("read processed schema: %v", err)
	}
	defaults, _, err := schema.Compile(schema.NormalizedSchema{Normalized: normalized})
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}

	// Keep this snippet verbatim; it exercises plugin defaults under plugin_configs.
	payload := `# Plugins que aplican a todas las rutas de control
plugin_configs:
- id: plugins
  plugins:
    limit-req:
      rate: 10
      burst: 20
      key: remote_addr`
	var doc any
	if err := yaml.Unmarshal([]byte(payload), &doc); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}

	schema.ApplyDefaults(defaults, doc)

	root, ok := doc.(map[string]any)
	if !ok {
		t.Fatalf("expected root object, got %T", doc)
	}
	pluginConfigs, ok := root["plugin_configs"].([]any)
	if !ok || len(pluginConfigs) == 0 {
		t.Fatalf("expected non-empty plugin_configs list, got %T (%v)", root["plugin_configs"], root["plugin_configs"])
	}
	first, ok := pluginConfigs[0].(map[string]any)
	if !ok {
		t.Fatalf("expected plugin_configs[0] object, got %T", pluginConfigs[0])
	}
	plugins, ok := first["plugins"].(map[string]any)
	if !ok {
		t.Fatalf("expected plugin_configs[0].plugins object, got %T", first["plugins"])
	}
	limitReq, ok := plugins["limit-req"].(map[string]any)
	if !ok {
		t.Fatalf("expected plugin_configs[0].plugins[limit-req] object, got %T", plugins["limit-req"])
	}
	if got := limitReq["policy"]; got != "local" {
		t.Fatalf("expected limit-req.policy default to be %q, got %v", "local", got)
	}
}
