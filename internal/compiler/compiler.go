package compiler

import (
	"errors"
	"fmt"
	"iter"
	"log/slog"
	"maps"
	"reflect"
	"sort"
	"strconv"
)

const (
	KindScalar     = "scalar"
	KindMap        = "map"
	KindList       = "list"
	KindSet        = "set"
	KindScalarList = "scalarlist"
)

var ErrWrongFormat = errors.New("wrong format (not map)")

// MergingRule describes how a YAML path should be merged.
// For list rules:
// - IDAttr selects the key used to match entries across fragments.
// - IDOptional controls whether entries without IDAttr are kept as standalone items.
// - AllowMergeSameID controls whether entries with the same IDAttr are merged or rejected.
// - SupportLabels controls whether the type supports labels.
// - Children defines merge rules for nested list paths under each list element.
// - Priority defines object priority. Higher priority items are processed first in the admin API.
// - Tagger tags elements for use in downstream processing.
type MergingRule struct {
	Path             string
	Kind             string
	IDAttr           string
	IDOptional       bool
	AllowMergeSameID bool
	SupportsLabels   bool
	Children         map[string]MergingRule
	Priority         int
	Tagger           func(data map[string]any) map[string][]string
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
				SupportsLabels:   true,
				Priority:         10,
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
				Tagger: routeTags,
			},
			"services": {
				Path:             "/services",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       true,
				AllowMergeSameID: true,
				SupportsLabels:   true,
				Priority:         20,
				Children: basicRules("services", map[string]MergingRule{
					"hosts": {
						Path: "services/hosts",
						Kind: KindScalarList,
					},
				}),
			},
			"upstreams": {
				Path:             "/upstreams",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       true,
				AllowMergeSameID: false,
				SupportsLabels:   true,
				Priority:         30,
			},
			"ssls": {
				Path:             "/ssls",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       true,
				AllowMergeSameID: false,
				SupportsLabels:   true,
				Priority:         40,
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
				SupportsLabels:   false,
				Priority:         50,
			},
			"consumer_groups": {
				Path:             "/consumer_groups",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       false,
				AllowMergeSameID: true,
				SupportsLabels:   true,
				Children:         basicRules("consumer_groups", nil),
				Priority:         40,
			},
			"plugin_configs": {
				Path:             "/plugin_configs",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       false,
				AllowMergeSameID: true,
				SupportsLabels:   true,
				Priority:         30,
				Children:         basicRules("plugin_configs", nil),
			},
			"stream_routes": {
				Path:             "/stream_routes",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       true,
				AllowMergeSameID: true,
				SupportsLabels:   true,
				Priority:         10,
				Children: map[string]MergingRule{
					"labels": {
						Path: "stream_routes/labels",
						Kind: KindMap,
					},
				},
			},
			"protos": {
				Path:             "/protos",
				Kind:             KindList,
				IDAttr:           "id",
				IDOptional:       true,
				AllowMergeSameID: false,
				SupportsLabels:   true,
				Priority:         50,
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
				SupportsLabels:   true,
				Priority:         50,
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
				IDAttr:           "id",
				IDOptional:       false,
				AllowMergeSameID: false,
				SupportsLabels:   false,
				Priority:         50,
			},
		},
	}
}

// Merge reads YAML fragments from the provided filesystems and merges them.
func Merge(logger *slog.Logger, snippets iter.Seq[Snippet]) (map[string]any, error) {
	rootRule := DefaultMergingRules()
	var merged map[string]any
	for value := range snippets {
		if merged == nil {
			merged = value.Data
			continue
		}
		mergedAny, err := ApplyMergeRules(merged, value.Data, rootRule)
		if err != nil {
			return nil, fmt.Errorf("merging path %s: %w", value.Ref.Path, err)
		}
		var ok bool
		merged, ok = mergedAny.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("wrong format (not map) after merging %s", value.Ref.Path)
		}
	}
	return merged, nil
}

// ApplyMergeRules merges two values according to the provided merge rule.
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
		switch rule.Kind {
		case KindMap:
			return mergeMap(leftMap, rightMap, rule)
		case KindSet:
			return mergeSet(leftMap, rightMap, rule)
		default:
			return nil, fmt.Errorf("expected %s at %s but got map", rule.Kind, rule.Path)
		}
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
	leftOrder, leftItems, leftTail, err := indexListByID(left, rule)
	if err != nil {
		return nil, err
	}
	rightOrder, rightItems, rightTail, err := indexListByID(right, rule)
	if err != nil {
		return nil, err
	}
	mergedItems, err := mergeKeyedItems(leftItems, rightItems, rule)
	if err != nil {
		return nil, err
	}

	output := make([]any, 0, len(leftOrder)+len(rightOrder)+len(leftTail)+len(rightTail))
	seen := make(map[idKey]struct{}, len(leftOrder)+len(rightOrder))
	for _, id := range leftOrder {
		output = append(output, mergedItems[id])
		seen[id] = struct{}{}
	}
	for _, id := range rightOrder {
		if _, ok := seen[id]; ok {
			continue
		}
		output = append(output, mergedItems[id])
		seen[id] = struct{}{}
	}
	output = append(output, leftTail...)
	output = append(output, rightTail...)
	return output, nil
}

func mergeSet(left, right map[string]any, rule MergingRule) (any, error) {
	leftItems, err := indexSet(left, rule)
	if err != nil {
		return nil, err
	}
	rightItems, err := indexSet(right, rule)
	if err != nil {
		return nil, err
	}
	mergedItems, err := mergeKeyedItems(leftItems, rightItems, rule)
	if err != nil {
		return nil, err
	}
	output := make(map[string]any, len(mergedItems))
	for key, value := range mergedItems {
		output[string(key)] = value
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

func indexListByID(items []any, rule MergingRule) ([]idKey, map[idKey]map[string]any, []any, error) {
	order := make([]idKey, 0, len(items))
	indexed := make(map[idKey]map[string]any)
	tail := make([]any, 0, len(items))
	for _, item := range items {
		asMap, ok := item.(map[string]any)
		if !ok {
			return nil, nil, nil, fmt.Errorf("list item must be a map at %s", rule.Path)
		}
		id, hasID, err := extractID(asMap, rule)
		if err != nil {
			return nil, nil, nil, err
		}
		if !hasID {
			tail = append(tail, asMap)
			continue
		}
		if _, exists := indexed[id]; exists {
			return nil, nil, nil, fmt.Errorf("duplicate id %s in list at %s", id, rule.Path)
		}
		order = append(order, id)
		indexed[id] = asMap
	}
	return order, indexed, tail, nil
}

func indexSet(items map[string]any, rule MergingRule) (map[idKey]map[string]any, error) {
	indexed := make(map[idKey]map[string]any, len(items))
	for key, value := range items {
		asMap, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("set item must be a map at %s", joinPath(rule.Path, key))
		}
		indexed[idKey(key)] = asMap
	}
	return indexed, nil
}

func mergeKeyedItems(left, right map[idKey]map[string]any, rule MergingRule) (map[idKey]map[string]any, error) {
	merged := make(map[idKey]map[string]any, len(left)+len(right))
	for id, value := range left {
		merged[id] = value
	}
	for id, value := range right {
		existing, exists := merged[id]
		if !exists {
			merged[id] = value
			continue
		}
		if !rule.AllowMergeSameID {
			return nil, fmt.Errorf("duplicate id %s without merge rule at %s", id, rule.Path)
		}
		mapMergingRule := rule
		mapMergingRule.Kind = KindMap
		mergedValue, err := ApplyMergeRules(existing, value, mapMergingRule)
		if err != nil {
			return nil, fmt.Errorf("merge id %s at %s: %w", id, rule.Path, err)
		}
		mergedMap, ok := mergedValue.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("merged item is not a map for id %s at %s", id, rule.Path)
		}
		merged[id] = mergedMap
	}
	return merged, nil
}

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
