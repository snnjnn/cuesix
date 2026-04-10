package factory

import (
	"testing"

	"iter"

	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/schema"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

type stubFetcher struct {
	snippets []compiler.Snippet
}

func (s stubFetcher) Fetch() iter.Seq2[compiler.Snippet, error] {
	return func(yield func(compiler.Snippet, error) bool) {
		for _, snippet := range s.snippets {
			if !yield(snippet, nil) {
				return
			}
		}
	}
}

func TestDeepcopyClonesNestedMapsAndLists(t *testing.T) {
	t.Parallel()

	original := map[string]any{
		"routes": []any{
			map[string]any{
				"id": "r1",
				"labels": map[string]any{
					"team": "api",
				},
			},
		},
	}

	copiedAny, err := deepcopy(original)
	if err != nil {
		t.Fatalf("deepcopy() error = %v", err)
	}
	copied, ok := copiedAny.(map[string]any)
	if !ok {
		t.Fatalf("deepcopy() type = %T", copiedAny)
	}

	copiedRoutes := copied["routes"].([]any)
	copiedRoute := copiedRoutes[0].(map[string]any)
	copiedLabels := copiedRoute["labels"].(map[string]any)
	copiedLabels["managed-by"] = "sixpack"

	originalRoute := original["routes"].([]any)[0].(map[string]any)
	originalLabels := originalRoute["labels"].(map[string]any)
	if _, ok := originalLabels["managed-by"]; ok {
		t.Fatalf("original mutated after deepcopy: %v", originalLabels)
	}
}

func TestSchemaFetcherFetchDoesNotMutateSnippetDataWhenApplyingDefaults(t *testing.T) {
	t.Parallel()

	parsed, compiled, err := schema.Compile(schema.NormalizedSchema{
		Normalized: []byte(`{
			"type":"object",
			"properties":{
				"injected":{"type":"string","default":"yes"},
				"routes":{"type":"array","items":{"type":"object"}}
			}
		}`),
	})
	if err != nil {
		t.Fatalf("schema.Compile() error = %v", err)
	}

	source := map[string]any{
		"routes": []any{
			map[string]any{"id": "r1"},
		},
	}
	fetcher := &SchemaFetcher{
		Fetcher: stubFetcher{
			snippets: []compiler.Snippet{{
				Ref:       compiler.SourceRef{Namespace: "test", Path: "routes.yaml"},
				Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW),
				Data:      source,
			}},
		},
		parsed: parsed,
		schema: compiled,
		Logger: testutil.Logger(),
	}

	for _, err := range fetcher.Fetch() {
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}
	}

	if _, ok := source["injected"]; ok {
		t.Fatalf("source mutated by SchemaFetcher defaults: %v", source)
	}
}
