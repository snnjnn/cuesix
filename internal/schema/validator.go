package schema

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/warpcondev/cuesix/internal/compiler"
	"go.yaml.in/yaml/v4"
)

// ValidationIssue describes a single validation problem reported by the schema engine.
// swagger:model ValidationIssue
type ValidationIssue struct {
	InstanceLocation        string `json:"instanceLocation,omitempty"`
	KeywordLocation         string `json:"keywordLocation,omitempty"`
	AbsoluteKeywordLocation string `json:"absoluteKeywordLocation,omitempty"`
	SchemaLocation          string `json:"schemaLocation,omitempty"`
	Message                 string `json:"message"`
	Detail                  string `json:"detail,omitempty"`
}

// ValidationResponse reports whether a document is valid and records any issues.
// swagger:model ValidationResponse
type ValidationResponse struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationIssue `json:"errors,omitempty"`
	Error  error             `json:"error,omitempty"`
}

type ValidationProbe struct {
	Payload json.RawMessage   `json:"payload"`
	IsYaml  bool              `json:"isYAML"`
	Env     map[string]string `json:"env"`
}

// Validate runs a JSON Schema validation against instance.
func Validate(schema *jsonschema.Schema, instance any) ValidationResponse {
	var issues []ValidationIssue
	err := schema.Validate(instance)
	return ValidationResponse{
		Valid:  err == nil && len(issues) == 0,
		Errors: collectIssues(err, issues),
		Error:  err,
	}
}

// Validate substitutes env vars, decodes payload, applies defaults, and validates.
func (p ValidationProbe) Validate(schema *jsonschema.Schema, defaults ParsedSchema) ValidationResponse {
	substituted := compiler.SubstituteAPISIX(string(p.Payload), p.Env)
	if len(substituted) == 0 {
		return ValidationResponse{
			Valid: false,
			Error: errors.New("schema payload is empty"),
		}
	}
	var doc any
	if p.IsYaml {
		if err := yaml.Unmarshal([]byte(substituted), &doc); err != nil {
			return ValidationResponse{
				Valid: false,
				Error: err,
			}
		}
	} else {
		if err := json.Unmarshal([]byte(substituted), &doc); err != nil {
			return ValidationResponse{
				Valid: false,
				Error: err,
			}
		}
	}
	if defaults.Parsed != nil {
		ApplyDefaults(defaults, doc)
	}
	return Validate(schema, doc)
}

// ApplyDefaults fills instance fields using schema default values and refs.
func ApplyDefaults(schema ParsedSchema, instance any) {
	applyDefaults(schema, instance, schema.Parsed, make(map[string]struct{}))
}

func applyDefaults(schema ParsedSchema, instance any, root any, seen map[string]struct{}) {
	sch, ok := schema.Parsed.(map[string]any)
	if !ok {
		return
	}

	// Handle $ref
	if ref, ok := sch["$ref"].(string); ok {
		if _, visited := seen[ref]; !visited {
			if resolved := resolveRef(root, ref); resolved != nil {
				seen[ref] = struct{}{}
				applyDefaults(ParsedSchema{Parsed: resolved}, instance, root, seen)
				delete(seen, ref)
			}
		}
		return
	}

	// Handle allOf
	if allOf, ok := sch["allOf"].([]any); ok {
		for _, entry := range allOf {
			applyDefaults(ParsedSchema{Parsed: entry}, instance, root, seen)
		}
	}

	// Handle object properties
	if obj, ok := instance.(map[string]any); ok {
		if props, ok := sch["properties"].(map[string]any); ok {
			for prop, propSchema := range props {
				if _, exists := obj[prop]; !exists {
					if def := getDefault(propSchema, root, seen); def != nil {
						obj[prop] = def
					}
				}
				if value, exists := obj[prop]; exists {
					applyDefaults(ParsedSchema{Parsed: propSchema}, value, root, seen)
				}
			}
		}
	}

	// Handle array items
	if arr, ok := instance.([]any); ok {
		if items, ok := sch["items"]; ok {
			switch typed := items.(type) {
			case []any:
				for i, itemSchema := range typed {
					if i < len(arr) {
						applyDefaults(ParsedSchema{Parsed: itemSchema}, arr[i], root, seen)
					}
				}
			default:
				for i := range arr {
					applyDefaults(ParsedSchema{Parsed: typed}, arr[i], root, seen)
				}
			}
		}
	}
}

func getDefault(schema, root any, seen map[string]struct{}) any {
	sch, ok := schema.(map[string]any)
	if !ok {
		return nil
	}

	if def, ok := sch["default"]; ok {
		// Simple clone via JSON round-trip
		if data, err := json.Marshal(def); err == nil {
			var clone any
			if json.Unmarshal(data, &clone) == nil {
				return clone
			}
		}
		return def
	}

	if ref, ok := sch["$ref"].(string); ok {
		if _, visited := seen[ref]; !visited {
			if resolved := resolveRef(root, ref); resolved != nil {
				seen[ref] = struct{}{}
				defer delete(seen, ref)
				return getDefault(resolved, root, seen)
			}
		}
	}

	return nil
}

func resolveRef(root any, ref string) any {
	if !strings.HasPrefix(ref, "#/") {
		return nil
	}

	cur := root
	for part := range strings.SplitSeq(ref[2:], "/") {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")

		switch node := cur.(type) {
		case map[string]any:
			cur = node[part]
		case []any:
			if idx, err := strconv.Atoi(part); err == nil && idx >= 0 && idx < len(node) {
				cur = node[idx]
			} else {
				return nil
			}
		default:
			return nil
		}
	}
	return cur
}

func collectIssues(err error, issues []ValidationIssue) []ValidationIssue {
	if err == nil {
		return issues
	}

	validation, ok := err.(*jsonschema.ValidationError)
	if !ok {
		return append(issues, ValidationIssue{Message: err.Error()})
	}

	output := validation.BasicOutput()
	if output == nil {
		return append(issues, ValidationIssue{Message: err.Error()})
	}

	return collectFromOutput(output, issues, validation.GoString())
}

func collectFromOutput(output *jsonschema.OutputUnit, issues []ValidationIssue, detail string) []ValidationIssue {
	if output == nil {
		return issues
	}

	if len(output.Errors) == 0 {
		message := "invalid"
		if output.Error != nil {
			message = output.Error.String()
		}

		schemaLocation := output.AbsoluteKeywordLocation
		if schemaLocation == "" {
			schemaLocation = output.KeywordLocation
		}

		// Improve generic validation messages
		if message == "validation failed" {
			if output.KeywordLocation != "" {
				message = "validation failed at " + output.KeywordLocation
			} else if schemaLocation != "" {
				message = "validation failed at " + schemaLocation
			}
			if detail != "" {
				if line := strings.TrimSpace(strings.Split(detail, "\n")[0]); line != "" {
					message = line
				}
			}
		}

		return append(issues, ValidationIssue{
			InstanceLocation:        output.InstanceLocation,
			KeywordLocation:         output.KeywordLocation,
			AbsoluteKeywordLocation: output.AbsoluteKeywordLocation,
			SchemaLocation:          schemaLocation,
			Message:                 message,
			Detail:                  detail,
		})
	}

	for i := range output.Errors {
		issues = collectFromOutput(&output.Errors[i], issues, detail)
	}
	return issues
}
