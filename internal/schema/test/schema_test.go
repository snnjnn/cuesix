package schema_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/warpcomdev/cuesix/internal/schema"
)

func TestNormalizeSchemaStrictMatchesFixture(t *testing.T) {
	raw, err := os.ReadFile("apisix_schema.json")
	if err != nil {
		t.Fatalf("read apisix_schema.json: %v", err)
	}
	expected, err := os.ReadFile("processed_schema.json")
	if err != nil {
		t.Fatalf("read processed_schema.json: %v", err)
	}
	output, err := schema.NormalizeSchema(schema.RawSchema{Raw: raw}, true)
	if err != nil {
		t.Fatalf("generate schema: %v", err)
	}

	got, err := decodeJSON(output.Normalized)
	if err != nil {
		t.Fatalf("decode generated schema: %v", err)
	}
	want, err := decodeJSON(expected)
	if err != nil {
		t.Fatalf("decode expected schema: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		path, left, right, ok := findMismatch(got, want)
		if ok {
			t.Fatalf("processed schema does not match fixture at %s: got=%v want=%v", path, left, right)
		}
		t.Fatalf("processed schema does not match fixture")
	}
}

func TestNormalizeSchemaLooseMatchesFixture(t *testing.T) {
	raw, err := os.ReadFile("apisix_schema.json")
	if err != nil {
		t.Fatalf("read apisix_schema.json: %v", err)
	}
	expected, err := os.ReadFile("loose_processed_schema.json")
	if err != nil {
		t.Fatalf("read loose_processed_schema.json: %v", err)
	}
	output, err := schema.NormalizeSchema(schema.RawSchema{Raw: raw}, false)
	if err != nil {
		t.Fatalf("generate schema: %v", err)
	}

	got, err := decodeJSON(output.Normalized)
	if err != nil {
		t.Fatalf("decode generated schema: %v", err)
	}
	want, err := decodeJSON(expected)
	if err != nil {
		t.Fatalf("decode expected schema: %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		path, left, right, ok := findMismatch(got, want)
		if ok {
			t.Fatalf("processed loose schema does not match fixture at %s: got=%v want=%v", path, left, right)
		}
		t.Fatalf("processed loose schema does not match fixture")
	}
}

func TestNormalizeSchemaDropsUnsupportedPatternsAndCoercesSchemaMapEntries(t *testing.T) {
	raw := []byte(`{
		"main": {
			"route": {
				"type": "object",
				"properties": {
					"id": {
						"type": "string"
					},
					"plugins": {
						"type": "object"
					}
				}
			}
		},
		"plugins": {
			"ai-rag": {
				"schema": {
					"type": "object",
					"properties": {
						"type": "object"
					}
				}
			},
			"datadog": {
				"schema": {
					"type": "object",
					"properties": {
						"constant_tags": {
							"type": "array",
							"items": {
								"type": "string",
								"pattern": "^[\\\\p{L}][\\\\p{L}\\\\p{N}_.:/-]*(?<!:)$"
							}
						}
					}
				},
				"metadata_schema": {
					"type": "object",
					"properties": {
						"constant_tags": {
							"type": "array",
							"items": {
								"type": "string",
								"pattern": "^[\\\\p{L}][\\\\p{L}\\\\p{N}_.:/-]*(?<!:)$"
							}
						}
					}
				}
			},
			"loggly": {
				"schema": {
					"type": "object",
					"properties": {
						"tags": {
							"type": "array",
							"items": {
								"type": "string",
								"pattern": "^(?!tag=)[ -~]*"
							}
						}
					}
				}
			},
			"opentelemetry": {
				"metadata_schema": {
					"type": "object",
					"properties": {
						"resource": {
							"type": "object",
							"additionalProperties": [
								{
									"type": "boolean"
								},
								{
									"type": "number"
								},
								{
									"type": "string"
								}
							]
						}
					}
				}
			}
		}
	}`)

	output, err := schema.NormalizeSchema(schema.RawSchema{Raw: raw}, true)
	if err != nil {
		t.Fatalf("NormalizeSchema() error = %v", err)
	}
	parsed, compiled, err := schema.Compile(output)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if compiled == nil {
		t.Fatal("Compile() returned nil schema")
	}

	doc, err := decodeJSON(output.Normalized)
	if err != nil {
		t.Fatalf("decode normalized schema: %v", err)
	}
	root := doc.(map[string]any)
	plugins := root["$defs"].(map[string]any)["plugins"].(map[string]any)["properties"].(map[string]any)

	aiRagType := plugins["ai-rag"].(map[string]any)["properties"].(map[string]any)["type"]
	aiRagTypeSchema, ok := aiRagType.(map[string]any)
	if !ok || aiRagTypeSchema["type"] != "object" {
		t.Fatalf("ai-rag type property not coerced into schema object: %#v", aiRagType)
	}

	datadogItems := plugins["datadog"].(map[string]any)["properties"].(map[string]any)["constant_tags"].(map[string]any)["items"].(map[string]any)
	if _, ok := datadogItems["pattern"]; ok {
		t.Fatalf("datadog constant_tags pattern was not removed: %#v", datadogItems["pattern"])
	}

	logglyItems := plugins["loggly"].(map[string]any)["properties"].(map[string]any)["tags"].(map[string]any)["items"].(map[string]any)
	if _, ok := logglyItems["pattern"]; ok {
		t.Fatalf("loggly tags pattern was not removed: %#v", logglyItems["pattern"])
	}

	pluginMetadata := root["properties"].(map[string]any)["plugin_metadata"].(map[string]any)
	if pluginMetadata["type"] != "array" {
		t.Fatalf("plugin_metadata should be modeled as array, got %#v", pluginMetadata["type"])
	}
	pluginMetadataItems := pluginMetadata["items"].(map[string]any)["oneOf"].([]any)

	foundDatadog := false
	for _, entry := range pluginMetadataItems {
		option := entry.(map[string]any)
		properties := option["properties"].(map[string]any)
		id, ok := properties["id"].(map[string]any)
		if !ok || id["const"] != "datadog" {
			continue
		}
		foundDatadog = true
		items := properties["constant_tags"].(map[string]any)["items"].(map[string]any)
		if _, ok := items["pattern"]; ok {
			t.Fatalf("plugin_metadata datadog constant_tags pattern was not removed: %#v", items["pattern"])
		}
	}
	if !foundDatadog {
		t.Fatal("datadog plugin_metadata schema not found")
	}

	foundOpenTelemetry := false
	for _, entry := range pluginMetadataItems {
		option := entry.(map[string]any)
		properties := option["properties"].(map[string]any)
		id, ok := properties["id"].(map[string]any)
		if !ok || id["const"] != "opentelemetry" {
			continue
		}
		foundOpenTelemetry = true
		resource := properties["resource"].(map[string]any)
		if _, ok := resource["additionalProperties"]; ok {
			t.Fatalf("plugin_metadata opentelemetry resource additionalProperties was not removed: %#v", resource["additionalProperties"])
		}
	}
	if !foundOpenTelemetry {
		t.Fatal("opentelemetry plugin_metadata schema not found")
	}

	if parsed.Parsed == nil {
		t.Fatal("parsed schema must not be nil")
	}
}

func decodeJSON(input []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(input))
	dec.UseNumber()
	var value any
	if err := dec.Decode(&value); err != nil {
		return nil, err
	}
	return value, nil
}

func findMismatch(left, right any) (string, any, any, bool) {
	return findMismatchAt("$", left, right)
}

func findMismatchAt(path string, left, right any) (string, any, any, bool) {
	if reflect.DeepEqual(left, right) {
		return "", nil, nil, false
	}
	switch l := left.(type) {
	case map[string]any:
		r, ok := right.(map[string]any)
		if !ok {
			return path, left, right, true
		}
		seen := map[string]struct{}{}
		for key := range l {
			seen[key] = struct{}{}
			path2, lv, rv, ok := findMismatchAt(path+"."+key, l[key], r[key])
			if ok {
				return path2, lv, rv, true
			}
		}
		for key := range r {
			if _, ok := seen[key]; ok {
				continue
			}
			return path + "." + key, nil, r[key], true
		}
	case []any:
		r, ok := right.([]any)
		if !ok {
			return path, left, right, true
		}
		if len(l) != len(r) {
			return path + ".length", len(l), len(r), true
		}
		for i := range l {
			path2, lv, rv, ok := findMismatchAt(path+fmt.Sprintf("[%d]", i), l[i], r[i])
			if ok {
				return path2, lv, rv, true
			}
		}
	default:
		return path, left, right, true
	}
	return path, left, right, true
}
