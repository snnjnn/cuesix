package compiler

import (
	"fmt"
	"io/fs"
	"iter"
	"log/slog"
	"maps"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"

	"go.yaml.in/yaml/v4"
)

const (
	KindScalar     = "scalar"
	KindMap        = "map"
	KindList       = "list"
	KindScalarList = "scalarlist"
)

// MergingRule describes how a YAML path should be merged.
// For list rules:
// - IDAttr selects the key used to match entries across fragments.
// - IDOptional controls whether entries without IDAttr are kept as standalone items.
// - AllowMergeSameID controls whether entries with the same IDAttr are merged or rejected.
// - Children defines merge rules for nested list paths under each list element.
type MergingRule struct {
	Path             string
	Kind             string
	IDAttr           string
	IDOptional       bool
	AllowMergeSameID bool
	Children         map[string]MergingRule
}

// DefaultMergingRules returns the APISIX-specific merge rules for top-level lists.
func DefaultMergingRules() MergingRule {

	// Many types share labels and plugins in their schema; we can usually merge
	// them as long as they contain disjoint keys.
	basicRules := func(prev string, other map[string]MergingRule) map[string]MergingRule {
		basic := map[string]MergingRule{
			"plugins": {
				Path: prev + "/plugins",
				Kind: KindMap,
			},
			"labels": {
				Path: prev + "/labels",
				Kind: KindMap,
			},
		}
		if other != nil {
			maps.Insert(basic, maps.All(other))
		}
		return basic
	}

	return MergingRule{
		Path: "/",
		Kind: KindMap,
		Children: map[string]MergingRule{
			"routes": {
				Path:             "/routes",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       true,
				AllowMergeSameID: true,
				Children: basicRules("routes", map[string]MergingRule{
					"uris": {
						Path: "routes/uris",
						Kind: KindScalarList,
					},
					"hosts": {
						Path: "routes/hosts",
						Kind: KindScalarList,
					},
				}),
			},
			"services": {
				Path:             "/services",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       true,
				AllowMergeSameID: false,
				Children:         basicRules("services", nil),
			},
			"upstreams": {
				Path:             "/upstreams",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       true,
				AllowMergeSameID: false,
			},
			"ssls": {
				Path:             "/ssls",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       true,
				AllowMergeSameID: false,
				Children: map[string]MergingRule{
					"snis": {
						Path: "ssls/snis",
						Kind: KindScalarList,
					},
				},
			},
			"global_rules": {
				Path:             "/global_rules",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       false,
				AllowMergeSameID: false,
			},
			"consumer_groups": {
				Path:             "/consumer_groups",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       false,
				AllowMergeSameID: false,
				Children:         basicRules("services", nil),
			},
			"plugin_configs": {
				Path:             "/plugin_configs",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       false,
				AllowMergeSameID: false,
			},
			"stream_routes": {
				Path:             "/stream_routes",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       true,
				AllowMergeSameID: false,
				Children:         basicRules("services", nil),
			},
			"protos": {
				Path:             "/protos",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       true,
				AllowMergeSameID: false,
				Children: map[string]MergingRule{
					"labels": {
						Kind:             KindMap,
						Path:             "/protos/labels",
						IDAttr:           "route_id",
						IDOptional:       false,
						AllowMergeSameID: false,
					},
				},
			},
			"consumers": {
				Path:             "/consumers",
				Kind:             KindList,
				IDAttr:           "username",
				IDOptional:       false,
				AllowMergeSameID: true,
				Children: basicRules("consumers", map[string]MergingRule{
					"credentials": {
						Path:             "/consumers/credentials",
						Kind:             KindList,
						IDAttr:           "credential_id",
						IDOptional:       false,
						AllowMergeSameID: false,
					},
				}),
			},
			"plugin_metadata": {
				Path:             "/plugin_metadata",
				Kind:             KindList,
				IDAttr:           "plugin_name",
				IDOptional:       false,
				AllowMergeSameID: false,
			},
			// Add support for the "jq" plugin
			"jq": {
				Path:             "/jq",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       true,
				AllowMergeSameID: false,
			},
		},
	}
}

type Snippet struct {
	Path string
	Data map[string]any
}

func Fetch(logger *slog.Logger, fses ...fs.FS) iter.Seq2[Snippet, error] {
	if logger == nil {
		logger = slog.Default()
	}
	return func(yield func(Snippet, error) bool) {
		for _, filesystem := range fses {
			paths, err := listYAMLFiles(filesystem)
			if err != nil {
				if !yield(Snippet{}, err) {
					return
				}
				continue
			}
			for _, path := range paths {
				logger.Info("compiler reading file", "path", path)
				content, err := fs.ReadFile(filesystem, path)
				if err != nil {
					if !yield(Snippet{}, fmt.Errorf("read %s: %w", path, err)) {
						return
					}
					continue
				}
				value, err := decodeYAML(content)
				if err != nil {
					if !yield(Snippet{}, fmt.Errorf("parse %s: %w", path, err)) {
						return
					}
					continue
				}
				if !yield(Snippet{Path: path, Data: value}, nil) {
					return
				}
			}
		}
	}
}

// Compile reads YAML fragments from the provided filesystems and merges them.
func Merge(logger *slog.Logger, snippets iter.Seq[Snippet]) (map[string]any, error) {
	if logger == nil {
		logger = slog.Default()
	}
	rootRule := DefaultMergingRules()
	var merged map[string]any
	for value := range snippets {
		if merged == nil {
			merged = value.Data
			continue
		}
		mergedAny, err := ApplyMergeRules(merged, value.Data, rootRule)
		if err != nil {
			return nil, fmt.Errorf("merging path %s: %w", value.Path, err)
		}
		var ok bool
		merged, ok = mergedAny.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("wrong format (not map) after merging %s", value.Path)
		}
	}
	return merged, nil
}

func Compile(logger *slog.Logger, fses ...fs.FS) (map[string]any, error) {
	if logger == nil {
		logger = slog.Default()
	}
	var fetchErr error
	untilError := func(items iter.Seq2[Snippet, error]) iter.Seq[Snippet] {
		return func(yield func(Snippet) bool) {
			for snippet, err := range items {
				if err != nil {
					fetchErr = err
					return
				}
				if !yield(snippet) {
					return
				}
			}
		}
	}
	if fetchErr != nil {
		return nil, fetchErr
	}
	merged, err := Merge(logger, untilError(Fetch(logger, fses...)))
	if err != nil {
		return nil, err
	}
	return merged, nil
}

func ApplyMergeRules(left, right any, rule MergingRule) (any, error) {
	if left == nil && right == nil {
		return nil, nil
	}
	if left == nil {
		return right, nil
	}
	if right == nil {
		return left, nil
	}

	leftMap, leftIsMap := left.(map[string]any)
	rightMap, rightIsMap := right.(map[string]any)
	if leftIsMap && rightIsMap {
		if rule.Kind != KindMap {
			return nil, fmt.Errorf("expected %s at %s but got map", rule.Kind, rule.Path)
		}
		return mergeMap(leftMap, rightMap, rule)
	}

	leftList, leftIsList := left.([]any)
	rightList, rightIsList := right.([]any)
	if leftIsList && rightIsList {
		switch rule.Kind {
		case KindList:
			return mergeList(leftList, rightList, rule)
		case KindScalarList:
			return mergeScalarList(leftList, rightList, rule)
		default:
			return nil, fmt.Errorf("missing list merge rule at %s", rulePath(rule))
		}
	}

	if !leftIsMap && !rightIsMap && !leftIsList && !rightIsList {
		if rule.Kind != KindScalar {
			return nil, fmt.Errorf("missing scalar merge rule at %s", rulePath(rule))
		}
		if reflect.DeepEqual(left, right) {
			return left, nil
		}
		return nil, fmt.Errorf("scalar conflict at %s", rulePath(rule))
	}

	return nil, fmt.Errorf("type mismatch at %s", rulePath(rule))
}

func mergeMap(left, right map[string]any, rule MergingRule) (any, error) {
	merged := make(map[string]any, len(left)+len(right))
	keys := make(map[string]struct{}, len(left)+len(right))
	for key := range left {
		keys[key] = struct{}{}
	}
	for key := range right {
		keys[key] = struct{}{}
	}

	keyList := make([]string, 0, len(keys))
	for key := range keys {
		keyList = append(keyList, key)
	}
	sort.Strings(keyList)

	for _, key := range keyList {
		leftValue, leftOk := left[key]
		rightValue, rightOk := right[key]
		if !leftOk {
			merged[key] = rightValue
			continue
		}
		if !rightOk {
			merged[key] = leftValue
			continue
		}
		// special case: id attributes might be defined as
		// integers or strings, but there are internally
		// noirmalized to strings by apisix.
		// So we allow merging id == 1 to id == "1"
		if key == rule.IDAttr {
			stringVal, ok := asString(leftValue)
			if ok {
				leftValue = stringVal
			}
			stringVal, ok = asString(rightValue)
			if ok {
				rightValue = stringVal
			}
		}
		mergedValue, err := ApplyMergeRules(leftValue, rightValue, ruleChild(rule, key))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", joinPath(rulePath(rule), key), err)
		}
		merged[key] = mergedValue
	}
	return merged, nil
}

func mergeList(left, right []any, rule MergingRule) (any, error) {
	type indexedItem struct {
		value map[string]any
		index int
	}
	seen := make(map[idKey]indexedItem)
	seenRight := make(map[idKey]struct{})
	output := make([]any, 0, len(left)+len(right))

	for _, item := range left {
		asMap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("list item must be a map at %s", rule.Path)
		}
		id, hasID, err := extractID(asMap, rule)
		if err != nil {
			return nil, err
		}
		if !hasID {
			output = append(output, asMap)
			continue
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate id %s in list at %s", id, rule.Path)
		}
		seen[id] = indexedItem{value: asMap, index: len(output)}
		output = append(output, asMap)
	}

	for _, item := range right {
		asMap, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("list item must be a map at %s", rule.Path)
		}
		id, hasID, err := extractID(asMap, rule)
		if err != nil {
			return nil, err
		}
		if !hasID {
			output = append(output, asMap)
			continue
		}
		if _, duplicate := seenRight[id]; duplicate {
			return nil, fmt.Errorf("duplicate id %s in list at %s", id, rule.Path)
		}
		seenRight[id] = struct{}{}
		existing, exists := seen[id]
		if !exists {
			seen[id] = indexedItem{value: asMap, index: len(output)}
			output = append(output, asMap)
			continue
		}
		if !rule.AllowMergeSameID {
			return nil, fmt.Errorf("duplicate id %s without merge rule at %s", id, rule.Path)
		}
		// Reuse the merging rule for maps
		mapMergingRule := rule
		mapMergingRule.Kind = KindMap
		mergedValue, err := ApplyMergeRules(existing.value, asMap, mapMergingRule)
		if err != nil {
			return nil, fmt.Errorf("merge id %s at %s: %w", id, rule.Path, err)
		}
		mergedMap, ok := mergedValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("merged item is not a map for id %s at %s", id, rule.Path)
		}
		seen[id] = indexedItem{value: mergedMap, index: existing.index}
		output[existing.index] = mergedMap
	}

	return output, nil
}

func mergeScalarList(left, right []any, rule MergingRule) (any, error) {
	output := make([]any, 0, len(left)+len(right))
	for _, item := range left {
		if !isScalarListItem(item) {
			return nil, fmt.Errorf("scalar list item must be a scalar at %s", rule.Path)
		}
		output = append(output, item)
	}
	for _, item := range right {
		if !isScalarListItem(item) {
			return nil, fmt.Errorf("scalar list item must be a scalar at %s", rule.Path)
		}
		if containsScalar(output, item) {
			continue
		}
		output = append(output, item)
	}
	return output, nil
}

func containsScalar(values []any, target any) bool {
	for _, value := range values {
		if reflect.DeepEqual(value, target) {
			return true
		}
	}
	return false
}

func isScalarListItem(value any) bool {
	switch value.(type) {
	case map[string]any, map[any]any, []any:
		return false
	default:
		return true
	}
}

type idKey string

func extractID(item map[string]any, rule MergingRule) (idKey, bool, error) {
	raw, ok := item[rule.IDAttr]
	if !ok {
		if rule.IDOptional {
			return "", false, nil
		}
		return "", false, fmt.Errorf("missing id attribute %q at %s", rule.IDAttr, rule.Path)
	}
	value, ok := asString(raw)
	if !ok {
		return "", false, fmt.Errorf("id attribute %q at %s must be string or number", rule.IDAttr, rule.Path)
	}
	return idKey(value), true, nil
}

func asString(raw any) (string, bool) {
	switch value := raw.(type) {
	case string:
		return value, true
	default:
		if val, ok := asInt64(raw); ok {
			return strconv.FormatInt(val, 10), true
		}
	}
	return "", false
}

func asInt64(value any) (int64, bool) {
	switch typed := value.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case float32:
		return int64(typed), true
	case float64:
		return int64(typed), true
	}
	return 0, false
}

func decodeYAML(content []byte) (map[string]any, error) {
	var value map[string]any
	if err := yaml.Unmarshal(content, &value); err != nil {
		return nil, err
	}
	return value, nil
}

func listYAMLFiles(filesystem fs.FS) ([]string, error) {
	var paths []string
	err := fs.WalkDir(filesystem, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext == ".yaml" || ext == ".yml" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}

func ruleChild(rule MergingRule, attrib string) MergingRule {
	if rule.Children != nil {
		child, ok := rule.Children[attrib]
		if ok {
			return child
		}
	}
	// If there is no merging rule defined, the merge
	// algorihtm must just allow scalars.
	return MergingRule{
		Path: attrib,
		Kind: KindScalar,
	}
}

func rulePath(rule MergingRule) string {
	if rule.Path == "" {
		return "/"
	}
	return rule.Path
}

func joinPath(base, key string) string {
	if base == "/" {
		return "/" + key
	}
	return base + "/" + key
}
