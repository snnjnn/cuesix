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
	output, err := schema.NormalizeSchema(schema.RawSchema{Raw:raw}, false)
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
