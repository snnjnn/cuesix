package compiler

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"reflect"
	"sort"

	"go.yaml.in/yaml/v3"
)

const (
	KindMap  = "map"
	KindList = "list"
)

type MergingRule struct {
	Path             string
	Kind             string
	IDAttr           string
	IDOptional       bool
	AllowMergeSameID bool
	Children         map[string]MergingRule
}

func DefaultMergingRules() MergingRule {
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
			},
			"services": {
				Path:             "/services",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       true,
				AllowMergeSameID: false,
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
			},
			"protos": {
				Path:             "/protos",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       true,
				AllowMergeSameID: false,
			},
			"consumers": {
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
			},
			"plugin_metadata": {
				Path:             "/plugin_metadata",
				Kind:             KindList,
				IDAttr:           "plugin_name",
				IDOptional:       false,
				AllowMergeSameID: false,
			},
		},
	}
}

func Compile(fses ...fs.FS) (map[string]any, error) {
	if len(fses) == 0 {
		return nil, errors.New("no filesystems provided")
	}

	rootRule := DefaultMergingRules()
	var merged map[string]any

	for _, filesystem := range fses {
		paths, err := listYAMLFiles(filesystem)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			content, err := fs.ReadFile(filesystem, path)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", path, err)
			}
			value, err := decodeYAML(content)
			if err != nil {
				return nil, fmt.Errorf("parse %s: %w", path, err)
			}
			asMap, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("root document in %s must be a map", path)
			}
			if merged == nil {
				merged = asMap
				continue
			}
			mergedAny, err := ApplyMergeRules(merged, asMap, rootRule)
			if err != nil {
				return nil, fmt.Errorf("merge %s: %w", path, err)
			}
			merged, ok = mergedAny.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("merged root is not a map after %s", path)
			}
		}
	}

	if merged == nil {
		return map[string]any{}, nil
	}

	return merged, nil
}

func ApplyMergeRules(left, right any, rule MergingRule) (any, error) {
	return mergeAny(left, right, &rule)
}

func mergeAny(left, right any, rule *MergingRule) (any, error) {
	if left == nil && right == nil {
		return nil, nil
	}
	if left == nil {
		return cloneAny(right), nil
	}
	if right == nil {
		return cloneAny(left), nil
	}

	leftMap, leftIsMap := left.(map[string]any)
	rightMap, rightIsMap := right.(map[string]any)
	if leftIsMap && rightIsMap {
		if rule != nil && rule.Kind != "" && rule.Kind != KindMap {
			return nil, fmt.Errorf("expected %s at %s but got map", rule.Kind, rule.Path)
		}
		return mergeMap(leftMap, rightMap, rule)
	}

	leftList, leftIsList := left.([]any)
	rightList, rightIsList := right.([]any)
	if leftIsList && rightIsList {
		if rule == nil || rule.Kind != KindList {
			return nil, fmt.Errorf("missing list merge rule at %s", rulePath(rule))
		}
		return mergeList(leftList, rightList, *rule)
	}

	if !leftIsMap && !rightIsMap && !leftIsList && !rightIsList {
		if reflect.DeepEqual(left, right) {
			return cloneAny(left), nil
		}
		return nil, fmt.Errorf("scalar conflict at %s", rulePath(rule))
	}

	return nil, fmt.Errorf("type mismatch at %s", rulePath(rule))
}

func mergeMap(left, right map[string]any, rule *MergingRule) (any, error) {
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
			merged[key] = cloneAny(rightValue)
			continue
		}
		if !rightOk {
			merged[key] = cloneAny(leftValue)
			continue
		}
		childRule, hasRule := ruleChild(rule, key)
		var useRule *MergingRule
		if hasRule {
			useRule = &childRule
		} else {
			useRule = nil
		}
		mergedValue, err := mergeAny(leftValue, rightValue, useRule)
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
	seen := make(map[string]indexedItem)
	seenRight := make(map[string]struct{})
	output := make([]any, 0, len(left)+len(right))

	for _, item := range left {
		normalized, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("list item must be a map at %s", rule.Path)
		}
		id, hasID, err := extractID(normalized, rule)
		if err != nil {
			return nil, err
		}
		if !hasID {
			output = append(output, cloneAny(normalized))
			continue
		}
		if _, exists := seen[id]; exists {
			return nil, fmt.Errorf("duplicate id %q in list at %s", id, rule.Path)
		}
		seen[id] = indexedItem{value: normalized, index: len(output)}
		output = append(output, cloneAny(normalized))
	}

	for _, item := range right {
		normalized, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("list item must be a map at %s", rule.Path)
		}
		id, hasID, err := extractID(normalized, rule)
		if err != nil {
			return nil, err
		}
		if !hasID {
			output = append(output, cloneAny(normalized))
			continue
		}
		if _, duplicate := seenRight[id]; duplicate {
			return nil, fmt.Errorf("duplicate id %q in list at %s", id, rule.Path)
		}
		seenRight[id] = struct{}{}
		existing, exists := seen[id]
		if !exists {
			seen[id] = indexedItem{value: normalized, index: len(output)}
			output = append(output, cloneAny(normalized))
			continue
		}
		if !rule.AllowMergeSameID {
			return nil, fmt.Errorf("duplicate id %q without merge rule at %s", id, rule.Path)
		}
		mergedValue, err := mergeAny(existing.value, normalized, &MergingRule{
			Path:     rule.Path,
			Kind:     KindMap,
			Children: rule.Children,
		})
		if err != nil {
			return nil, fmt.Errorf("merge id %q at %s: %w", id, rule.Path, err)
		}
		mergedMap, ok := mergedValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("merged item is not a map for id %q at %s", id, rule.Path)
		}
		seen[id] = indexedItem{value: mergedMap, index: existing.index}
		output[existing.index] = mergedMap
	}

	return output, nil
}

func extractID(item map[string]any, rule MergingRule) (string, bool, error) {
	raw, ok := item[rule.IDAttr]
	if !ok {
		if rule.IDOptional {
			return "", false, nil
		}
		return "", false, fmt.Errorf("missing id attribute %q at %s", rule.IDAttr, rule.Path)
	}
	switch value := raw.(type) {
	case string:
		if value == "" && !rule.IDOptional {
			return "", false, fmt.Errorf("empty id attribute %q at %s", rule.IDAttr, rule.Path)
		}
		return value, true, nil
	default:
		return "", false, fmt.Errorf("id attribute %q must be string at %s", rule.IDAttr, rule.Path)
	}
}

func decodeYAML(content []byte) (any, error) {
	var value any
	if err := yaml.Unmarshal(content, &value); err != nil {
		return nil, err
	}
	return normalizeYAML(value)
}

func normalizeYAML(value any) (any, error) {
	switch typed := value.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(typed))
		for key, nested := range typed {
			converted, err := normalizeYAML(nested)
			if err != nil {
				return nil, err
			}
			normalized[key] = converted
		}
		return normalized, nil
	case map[any]any:
		normalized := make(map[string]any, len(typed))
		for key, nested := range typed {
			stringKey, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("non-string map key: %v", key)
			}
			converted, err := normalizeYAML(nested)
			if err != nil {
				return nil, err
			}
			normalized[stringKey] = converted
		}
		return normalized, nil
	case []any:
		normalized := make([]any, 0, len(typed))
		for _, item := range typed {
			converted, err := normalizeYAML(item)
			if err != nil {
				return nil, err
			}
			normalized = append(normalized, converted)
		}
		return normalized, nil
	default:
		return typed, nil
	}
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

func ruleChild(rule *MergingRule, key string) (MergingRule, bool) {
	if rule == nil || rule.Children == nil {
		return MergingRule{}, false
	}
	child, ok := rule.Children[key]
	return child, ok
}

func rulePath(rule *MergingRule) string {
	if rule == nil || rule.Path == "" {
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

func cloneAny(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		copied := make(map[string]any, len(typed))
		for key, nested := range typed {
			copied[key] = cloneAny(nested)
		}
		return copied
	case []any:
		copied := make([]any, len(typed))
		for i, item := range typed {
			copied[i] = cloneAny(item)
		}
		return copied
	default:
		return typed
	}
}
