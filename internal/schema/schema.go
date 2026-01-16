package schema

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const schemaVersion = "https://json-schema.org/draft/2020-12/schema"
const schemaMaxBytes = 16 << 20
const schemaRetryDelay = time.Second

type ResourceKind string

const (
	KindList ResourceKind = "list"
	KindMap  ResourceKind = "map"
)

// ResourceSpec describes how a top-level APISIX resource is modeled in a declarative file.
type ResourceSpec struct {
	Key     string
	MainKey string
	IDField string
	Kind    ResourceKind
}

// DefaultResourceSpecs mirrors APISIX standalone resource layout used by ADC.
var DefaultResourceSpecs = []ResourceSpec{
	{Key: "routes", MainKey: "route", IDField: "id", Kind: KindList},
	{Key: "services", MainKey: "service", IDField: "id", Kind: KindList},
	{Key: "upstreams", MainKey: "upstream", IDField: "id", Kind: KindList},
	{Key: "consumers", MainKey: "consumer", IDField: "username", Kind: KindList},
	{Key: "consumer_groups", MainKey: "consumer_group", IDField: "id", Kind: KindList},
	{Key: "plugin_configs", MainKey: "plugin_config", IDField: "id", Kind: KindList},
	{Key: "global_rules", MainKey: "global_rule", IDField: "id", Kind: KindList},
	{Key: "plugin_metadata", MainKey: "plugin_metadata", IDField: "plugin_name", Kind: KindList},
	{Key: "ssls", MainKey: "ssl", IDField: "id", Kind: KindList},
	{Key: "stream_routes", MainKey: "stream_route", IDField: "id", Kind: KindList},
}

type RawSchema struct {
	Raw []byte
}

type NormalizedSchema struct {
	Normalized []byte
}

type ParsedSchema struct {
	Parsed any
}

// FetchSchemaUntil keeps trying to download the APISIX schema until the context ends
// or the backoff retries expire.
func FetchSchemaUntil(ctx context.Context, logger *slog.Logger, client *http.Client, baseURL, apiKey string, backoffCfg backoff.BackOff) (RawSchema, error) {
	if client == nil {
		return RawSchema{}, errors.New("missing http client")
	}
	schemaURL, err := buildSchemaURL(baseURL)
	if err != nil {
		return RawSchema{}, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, schemaURL, nil)
	if err != nil {
		return RawSchema{}, err
	}
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("X-API-KEY", apiKey)
	}
	var payload []byte
	operation := func() error {
		resp, err := client.Do(req)
		if err != nil {
			return err
		}
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, schemaMaxBytes))
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return readErr
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("schema request failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
		}
		payload = body
		return nil
	}
	notify := func(err error, next time.Duration) {
		logger.Warn("schema fetch failed; retrying", "error", err, "nextDelay", next)
	}
	if err := backoff.RetryNotify(operation, backoff.WithContext(backoffCfg, ctx), notify); err != nil {
		return RawSchema{}, err
	}
	return RawSchema{Raw: payload}, nil
}

// NormalizeSchema normalizes a raw APISIX schema payload into a JSON Schema.
// When strict is false, APISIX ID flexibility is preserved.
func NormalizeSchema(raw RawSchema, strict bool) (NormalizedSchema, error) {
	var doc map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw.Raw))
	dec.UseNumber()
	if err := dec.Decode(&doc); err != nil {
		return NormalizedSchema{}, fmt.Errorf("decode schema: %w", err)
	}
	normalized, err := normalizeSchema(doc, strict)
	if err != nil {
		return NormalizedSchema{}, err
	}
	sanitizeJSONSchema(normalized)
	payload, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		return NormalizedSchema{}, err
	}
	return NormalizedSchema{Normalized: payload}, nil
}

func Compile(payload NormalizedSchema) (ParsedSchema, *jsonschema.Schema, error) {
	if len(payload.Normalized) == 0 {
		return ParsedSchema{}, nil, errors.New("schema payload is empty")
	}
	var doc any
	if err := json.Unmarshal(payload.Normalized, &doc); err != nil {
		return ParsedSchema{}, nil, err
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("schema.json", doc); err != nil {
		return ParsedSchema{}, nil, err
	}
	compiled, err := compiler.Compile("schema.json")
	return ParsedSchema{Parsed: doc}, compiled, err
}

func buildSchemaURL(baseURL string) (string, error) {
	trimmed := strings.TrimSpace(baseURL)
	if trimmed == "" {
		return "", errors.New("missing apisix base url")
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("invalid apisix base url: %s", baseURL)
	}
	parsed.Path = "/v1/schema"
	parsed.RawQuery = ""
	return parsed.String(), nil
}

func normalizeSchema(doc map[string]any, strict bool) (map[string]any, error) {
	main, ok := doc["main"].(map[string]any)
	if !ok {
		return nil, errors.New("missing main section in schema")
	}
	pluginDefs := extractPluginDefs(doc["plugins"])
	streamPluginDefs := extractPluginDefs(doc["stream_plugins"])
	pluginMetadataDefs := extractPluginMetadataDefs(doc["plugins"], doc["stream_plugins"])

	updatePluginSchema(main, "route", "plugins", "#/$defs/plugins/properties", pluginDefs)
	updatePluginSchema(main, "plugin_config", "plugins", "#/$defs/plugins/properties", pluginDefs)
	updatePluginSchema(main, "global_rule", "plugins", "#/$defs/plugins/properties", pluginDefs)
	updatePluginSchema(main, "stream_route", "plugins", "#/$defs/stream_plugins/properties", streamPluginDefs)

	properties := buildTopLevelSchema(main, DefaultResourceSpecs, strict)
	if len(pluginMetadataDefs) > 0 {
		properties["properties"].(map[string]any)["plugin_metadata"] = wrapResourceSchema(
			buildPluginMetadataSchema(pluginMetadataDefs, strict),
			KindList,
		)
	}
	properties["$schema"] = schemaVersion
	properties["$defs"] = map[string]any{
		"plugins":        buildDefsSchema(pluginDefs),
		"stream_plugins": buildDefsSchema(streamPluginDefs),
	}
	return properties, nil
}

func extractPluginDefs(section any) map[string]any {
	result := map[string]any{}
	plugins, ok := section.(map[string]any)
	if !ok {
		return result
	}
	for name, descriptor := range plugins {
		entry, ok := descriptor.(map[string]any)
		if !ok {
			continue
		}
		schema, ok := entry["schema"]
		if !ok {
			continue
		}
		result[name] = schema
	}
	return result
}

func extractPluginMetadataDefs(sections ...any) map[string]any {
	result := map[string]any{}
	for _, section := range sections {
		plugins, ok := section.(map[string]any)
		if !ok {
			continue
		}
		for name, descriptor := range plugins {
			entry, ok := descriptor.(map[string]any)
			if !ok {
				continue
			}
			schema, ok := entry["metadata_schema"]
			if !ok {
				continue
			}
			result[name] = schema
		}
	}
	return result
}

func updatePluginSchema(main map[string]any, mainKey, propertyKey, refPrefix string, defs map[string]any) {
	if len(defs) == 0 {
		return
	}
	section, ok := main[mainKey].(map[string]any)
	if !ok {
		return
	}
	props, ok := section["properties"].(map[string]any)
	if !ok {
		return
	}
	props[propertyKey] = buildPluginsSchema(defs, refPrefix)
}

func buildPluginsSchema(defs map[string]any, refPrefix string) map[string]any {
	properties := map[string]any{}
	for name := range defs {
		properties[name] = map[string]any{
			"$ref": refPrefix + "/" + escapeJSONPointer(name),
		}
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
}

func buildDefsSchema(defs map[string]any) map[string]any {
	if len(defs) == 0 {
		return map[string]any{"type": "object"}
	}
	properties := map[string]any{}
	for name, schema := range defs {
		properties[name] = schema
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
}

func buildPluginMetadataSchema(defs map[string]any, strict bool) map[string]any {
	var options []any
	names := make([]string, 0, len(defs))
	for name := range defs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		schema := defs[name]
		properties := map[string]any{
			"plugin_name": map[string]any{
				"const": name,
			},
		}
		if schemaMap, ok := schema.(map[string]any); ok {
			if props, ok := schemaMap["properties"].(map[string]any); ok {
				for key, value := range props {
					properties[key] = value
				}
			}
			if required, ok := schemaMap["required"].([]any); ok {
				options = append(options, map[string]any{
					"type":                 "object",
					"properties":           properties,
					"required":             append([]any{"plugin_name"}, required...),
					"additionalProperties": false,
				})
				continue
			}
		}
		options = append(options, map[string]any{
			"type":                 "object",
			"properties":           properties,
			"required":             []any{"plugin_name"},
			"additionalProperties": false,
		})
	}
	if len(options) == 0 {
		return map[string]any{"type": "object"}
	}
	if strict {
		return map[string]any{
			"oneOf": options,
		}
	}
	return map[string]any{
		"anyOf": options,
	}
}

func escapeJSONPointer(input string) string {
	input = strings.ReplaceAll(input, "~", "~0")
	return strings.ReplaceAll(input, "/", "~1")
}

func buildTopLevelSchema(main map[string]any, specs []ResourceSpec, strict bool) map[string]any {
	properties := map[string]any{}
	for _, spec := range specs {
		resource, ok := main[spec.MainKey].(map[string]any)
		if !ok {
			continue
		}
		enforceIDSchema(resource, spec.IDField, strict)
		properties[spec.Key] = wrapResourceSchema(resource, spec.Kind)
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
}

func wrapResourceSchema(resource map[string]any, kind ResourceKind) map[string]any {
	switch kind {
	case KindMap:
		return map[string]any{
			"type":                 "object",
			"additionalProperties": resource,
		}
	default:
		return map[string]any{
			"type":  "array",
			"items": resource,
		}
	}
}

func enforceIDSchema(resource map[string]any, idField string, strict bool) {
	if idField == "" {
		return
	}
	properties, ok := resource["properties"].(map[string]any)
	if !ok {
		return
	}
	idSchema := map[string]any{}
	if current, ok := properties[idField].(map[string]any); ok {
		for key, value := range current {
			idSchema[key] = value
		}
	}
	if strict {
		idSchema["type"] = "string"
		if _, ok := idSchema["minLength"]; !ok {
			idSchema["minLength"] = 1
		}
		stripAnyOfTypes(idSchema)
	}
	properties[idField] = idSchema
	resource["properties"] = properties
	if strict {
		required := ensureRequired(resource["required"], idField)
		resource["required"] = required
	}
}

func stripAnyOfTypes(schema map[string]any) {
	raw, ok := schema["anyOf"].([]any)
	if !ok {
		return
	}
	var filtered []any
	for _, entry := range raw {
		asMap, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if asMap["type"] == "string" {
			filtered = append(filtered, asMap)
		}
	}
	if len(filtered) > 0 {
		schema["anyOf"] = filtered
	} else {
		delete(schema, "anyOf")
	}
}

func ensureRequired(raw any, field string) []any {
	var required []any
	if list, ok := raw.([]any); ok {
		required = append(required, list...)
	}
	for _, value := range required {
		if value == field {
			return required
		}
	}
	return append(required, field)
}

// Fix para algunas partes del schema de apisix, como el plugin JWT,
// usando tipos incorrectos para algunos atributos del schema
var schemaIntegerKeywords = map[string]struct{}{
	"minLength":     {},
	"maxLength":     {},
	"minItems":      {},
	"maxItems":      {},
	"minProperties": {},
	"maxProperties": {},
	"minContains":   {},
	"maxContains":   {},
}

var schemaNumberKeywords = map[string]struct{}{
	"multipleOf":       {},
	"minimum":          {},
	"maximum":          {},
	"exclusiveMinimum": {},
	"exclusiveMaximum": {},
}

// sanitizeJSONSchema coerces common APISIX schema deviations into metaschema-valid JSON Schema.
// In particular, it fixes known cases where numeric JSON Schema keywords are returned as strings.
func sanitizeJSONSchema(node any) {
	switch typed := node.(type) {
	case map[string]any:
		for key, value := range typed {
			if _, ok := schemaIntegerKeywords[key]; ok {
				if asString, ok := value.(string); ok {
					if parsed, err := strconv.ParseInt(strings.TrimSpace(asString), 10, 64); err == nil {
						typed[key] = parsed
						value = parsed
					}
				}
			} else if _, ok := schemaNumberKeywords[key]; ok {
				if asString, ok := value.(string); ok {
					asString = strings.TrimSpace(asString)
					if asString != "" {
						if _, err := strconv.ParseFloat(asString, 64); err == nil {
							typed[key] = json.Number(asString)
							value = typed[key]
						}
					}
				}
			}
			sanitizeJSONSchema(value)
		}
	case []any:
		for i := range typed {
			sanitizeJSONSchema(typed[i])
		}
	}
}
