package compiler_test

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"iter"

	"github.com/warpcondev/cuesix/internal/compiler"
	"github.com/warpcondev/cuesix/internal/testutil"
)

func snippetRef(path string) compiler.SourceRef {
	return compiler.SourceRef{Root: "test", Path: path}
}

func snippetSeq(snips []compiler.Snippet) iter.Seq[compiler.Snippet] {
	return func(yield func(compiler.Snippet) bool) {
		for _, s := range snips {
			if !yield(s) {
				return
			}
		}
	}
}

func TestMergeRoutesWithDefaultRules(t *testing.T) {
	t.Parallel()
	left := map[string]any{
		"routes": []any{
			map[string]any{
				"id":    "1",
				"hosts": []any{"a.example.com"},
				"labels": map[string]any{
					"env": "dev",
				},
			},
		},
	}
	right := map[string]any{
		"routes": []any{
			map[string]any{
				"id":    "1",
				"hosts": []any{"b.example.com"},
				"labels": map[string]any{
					"team": "api",
				},
			},
			map[string]any{
				"uris": []any{"/new"},
			},
		},
	}
	merged, err := compiler.Merge(testutil.Logger(), snippetSeq([]compiler.Snippet{
		{Ref: snippetRef("left.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: left},
		{Ref: snippetRef("right.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: right},
	}))
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}
	routes, ok := merged["routes"].([]any)
	if !ok || len(routes) != 2 {
		t.Fatalf("expected two routes, got %v", merged["routes"])
	}
	var mergedRoute map[string]any
	for _, item := range routes {
		if m, ok := item.(map[string]any); ok && m["id"] == "1" {
			mergedRoute = m
			break
		}
	}
	if mergedRoute == nil {
		t.Fatalf("merged route not found")
	}
	hosts, ok := mergedRoute["hosts"].([]any)
	if !ok || len(hosts) != 2 {
		t.Fatalf("expected merged hosts, got %v", mergedRoute["hosts"])
	}
	if !slices.Contains(hosts, "a.example.com") || !slices.Contains(hosts, "b.example.com") {
		t.Fatalf("expected both hosts in merged result")
	}
	labels, ok := mergedRoute["labels"].(map[string]any)
	if !ok || labels["env"] != "dev" || labels["team"] != "api" {
		t.Fatalf("expected merged labels, got %v", mergedRoute["labels"])
	}
}

func TestMergeServicesConflictOnScalarField(t *testing.T) {
	t.Parallel()
	left := map[string]any{
		"services": []any{
			map[string]any{"id": "svc", "name": "left"},
		},
	}
	right := map[string]any{
		"services": []any{
			map[string]any{"id": "svc", "name": "right"},
		},
	}
	_, err := compiler.Merge(testutil.Logger(), snippetSeq([]compiler.Snippet{
		{Ref: snippetRef("left.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: left},
		{Ref: snippetRef("right.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: right},
	}))
	if err == nil || !strings.Contains(err.Error(), "scalar conflict at name") {
		t.Fatalf("expected scalar conflict error, got %v", err)
	}
}

func TestMergeGlobalRulesRequireID(t *testing.T) {
	t.Parallel()
	left := map[string]any{
		"global_rules": []any{
			map[string]any{"id": "ok", "desc": "first"},
		},
	}
	right := map[string]any{
		"global_rules": []any{
			map[string]any{"desc": "missing id"},
		},
	}
	_, err := compiler.Merge(testutil.Logger(), snippetSeq([]compiler.Snippet{
		{Ref: snippetRef("left.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: left},
		{Ref: snippetRef("right.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: right},
	}))
	if err == nil || !strings.Contains(err.Error(), "missing id attribute") {
		t.Fatalf("expected missing id error, got %v", err)
	}
}

func TestMergePluginConfigsWithSameIDMergesPluginsAndLabels(t *testing.T) {
	t.Parallel()

	left := map[string]any{
		"plugin_configs": []any{
			map[string]any{
				"id": "pc1",
				"plugins": map[string]any{
					"limit-count": map[string]any{"count": 10},
				},
				"labels": map[string]any{
					"team": "api",
				},
			},
		},
	}
	right := map[string]any{
		"plugin_configs": []any{
			map[string]any{
				"id": "pc1",
				"plugins": map[string]any{
					"proxy-rewrite": map[string]any{"uri": "/v2"},
				},
				"labels": map[string]any{
					"env": "prod",
				},
			},
		},
	}

	merged, err := compiler.Merge(testutil.Logger(), snippetSeq([]compiler.Snippet{
		{Ref: snippetRef("left.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: left},
		{Ref: snippetRef("right.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: right},
	}))
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	configs, ok := merged["plugin_configs"].([]any)
	if !ok || len(configs) != 1 {
		t.Fatalf("expected one merged plugin_config, got %v", merged["plugin_configs"])
	}

	config, ok := configs[0].(map[string]any)
	if !ok {
		t.Fatalf("expected plugin_config object, got %T", configs[0])
	}

	plugins, ok := config["plugins"].(map[string]any)
	if !ok {
		t.Fatalf("expected merged plugins map, got %T", config["plugins"])
	}
	if _, ok := plugins["limit-count"]; !ok {
		t.Fatalf("missing left plugin after merge: %v", plugins)
	}
	if _, ok := plugins["proxy-rewrite"]; !ok {
		t.Fatalf("missing right plugin after merge: %v", plugins)
	}

	labels, ok := config["labels"].(map[string]any)
	if !ok {
		t.Fatalf("expected merged labels map, got %T", config["labels"])
	}
	if labels["team"] != "api" || labels["env"] != "prod" {
		t.Fatalf("unexpected merged labels: %v", labels)
	}
}

func TestMergeConsumerGroupsWithSameIDMergesPluginsAndLabels(t *testing.T) {
	t.Parallel()

	left := map[string]any{
		"consumer_groups": []any{
			map[string]any{
				"id": "cg1",
				"plugins": map[string]any{
					"limit-count": map[string]any{"count": 10},
				},
				"labels": map[string]any{
					"team": "api",
				},
			},
		},
	}
	right := map[string]any{
		"consumer_groups": []any{
			map[string]any{
				"id": "cg1",
				"plugins": map[string]any{
					"proxy-rewrite": map[string]any{"uri": "/v2"},
				},
				"labels": map[string]any{
					"env": "prod",
				},
			},
		},
	}

	merged, err := compiler.Merge(testutil.Logger(), snippetSeq([]compiler.Snippet{
		{Ref: snippetRef("left.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: left},
		{Ref: snippetRef("right.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: right},
	}))
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	groups, ok := merged["consumer_groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("expected one merged consumer_group, got %v", merged["consumer_groups"])
	}

	group, ok := groups[0].(map[string]any)
	if !ok {
		t.Fatalf("expected consumer_group object, got %T", groups[0])
	}

	plugins, ok := group["plugins"].(map[string]any)
	if !ok {
		t.Fatalf("expected merged plugins map, got %T", group["plugins"])
	}
	if _, ok := plugins["limit-count"]; !ok {
		t.Fatalf("missing left plugin after merge: %v", plugins)
	}
	if _, ok := plugins["proxy-rewrite"]; !ok {
		t.Fatalf("missing right plugin after merge: %v", plugins)
	}

	labels, ok := group["labels"].(map[string]any)
	if !ok {
		t.Fatalf("expected merged labels map, got %T", group["labels"])
	}
	if labels["team"] != "api" || labels["env"] != "prod" {
		t.Fatalf("unexpected merged labels: %v", labels)
	}
}

func TestMergeStreamRoutesWithSameIDMergesLabels(t *testing.T) {
	t.Parallel()

	left := map[string]any{
		"stream_routes": []any{
			map[string]any{
				"id": "st1",
				"labels": map[string]any{
					"team": "api",
				},
				"server_port": 9000,
			},
		},
	}
	right := map[string]any{
		"stream_routes": []any{
			map[string]any{
				"id": "st1",
				"labels": map[string]any{
					"env": "prod",
				},
				"server_port": 9000,
			},
		},
	}

	merged, err := compiler.Merge(testutil.Logger(), snippetSeq([]compiler.Snippet{
		{Ref: snippetRef("left.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: left},
		{Ref: snippetRef("right.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: right},
	}))
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	routes, ok := merged["stream_routes"].([]any)
	if !ok || len(routes) != 1 {
		t.Fatalf("expected one merged stream_route, got %v", merged["stream_routes"])
	}

	route, ok := routes[0].(map[string]any)
	if !ok {
		t.Fatalf("expected stream_route object, got %T", routes[0])
	}

	labels, ok := route["labels"].(map[string]any)
	if !ok {
		t.Fatalf("expected merged labels map, got %T", route["labels"])
	}
	if labels["team"] != "api" || labels["env"] != "prod" {
		t.Fatalf("unexpected merged labels: %v", labels)
	}
	if route["server_port"] != 9000 {
		t.Fatalf("expected scalar field preserved, got %v", route["server_port"])
	}
}

func TestMergePluginMetadataDistinctKeys(t *testing.T) {
	t.Parallel()

	left := map[string]any{
		"plugin_metadata": []any{
			map[string]any{
				"id":     "limit-count",
				"policy": "local",
			},
		},
	}
	right := map[string]any{
		"plugin_metadata": []any{
			map[string]any{
				"id":        "proxy-rewrite",
				"regex_uri": []any{"^/v1/(.*)", "/$1"},
			},
		},
	}

	merged, err := compiler.Merge(testutil.Logger(), snippetSeq([]compiler.Snippet{
		{Ref: snippetRef("left.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: left},
		{Ref: snippetRef("right.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: right},
	}))
	if err != nil {
		t.Fatalf("Merge returned error: %v", err)
	}

	metadata, ok := merged["plugin_metadata"].([]any)
	if !ok {
		t.Fatalf("expected plugin_metadata list, got %T", merged["plugin_metadata"])
	}
	if len(metadata) != 2 {
		t.Fatalf("expected two plugin_metadata entries, got %v", metadata)
	}
	first, ok := metadata[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first plugin_metadata object, got %T", metadata[0])
	}
	second, ok := metadata[1].(map[string]any)
	if !ok {
		t.Fatalf("expected second plugin_metadata object, got %T", metadata[1])
	}
	if first["id"] != "limit-count" || first["policy"] != "local" {
		t.Fatalf("unexpected first plugin_metadata entry: %v", first)
	}
	if second["id"] != "proxy-rewrite" {
		t.Fatalf("unexpected second plugin_metadata id: %v", second)
	}
}

func TestMergePluginMetadataDuplicateKeyRejected(t *testing.T) {
	t.Parallel()

	left := map[string]any{
		"plugin_metadata": []any{
			map[string]any{
				"id":     "limit-count",
				"policy": "local",
			},
		},
	}
	right := map[string]any{
		"plugin_metadata": []any{
			map[string]any{
				"id":     "limit-count",
				"policy": "local",
			},
		},
	}

	_, err := compiler.Merge(testutil.Logger(), snippetSeq([]compiler.Snippet{
		{Ref: snippetRef("left.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: left},
		{Ref: snippetRef("right.yaml"), Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW), Data: right},
	}))
	if err == nil || !strings.Contains(err.Error(), "duplicate id limit-count without merge rule at /plugin_metadata") {
		t.Fatalf("expected plugin_metadata duplicate key error, got %v", err)
	}
}

func TestFetchReadsYAMLFiles(t *testing.T) {
	t.Parallel()
	fs1 := fstest.MapFS{
		"a.yaml": {Data: []byte("routes:\n- id: a")},
		"b.yml":  {Data: []byte("services:\n- id: b")},
		"c.txt":  {Data: []byte("ignore")},
	}
	var paths []string
	resolver := compiler.DefaultResolver{
		VirtualGateway: compiler.FromKey(compiler.DEFAULT_VIRTUALGW),
	}
	logger := testutil.Logger()
	root := compiler.InputRoot{Name: "fs1", FS: fs1}
	for snippet, err := range compiler.Fetch(logger, compiler.Enumerate(logger, resolver, root)) {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		paths = append(paths, snippet.Ref.Path)
		if snippet.Data == nil {
			t.Fatalf("expected data for %s", snippet.Ref.Path)
		}
	}
	if len(paths) != 2 || paths[0] != "a.yaml" || paths[1] != "b.yml" {
		t.Fatalf("unexpected paths %v", paths)
	}
}

func TestFetchPropagatesErrors(t *testing.T) {
	t.Parallel()
	badFS := fstest.MapFS{
		"bad.yaml": {Data: []byte("{not yaml")},
	}
	var gotErr error
	resolver := compiler.DefaultResolver{
		VirtualGateway: compiler.FromKey(compiler.DEFAULT_VIRTUALGW),
	}
	logger := testutil.Logger()
	root := compiler.InputRoot{Name: "bad", FS: badFS}
	for _, err := range compiler.Fetch(logger, compiler.Enumerate(logger, resolver, root)) {
		gotErr = err
		break
	}
	if gotErr == nil {
		t.Fatalf("expected parse error")
	}
}

func TestApplyMergeRulesScalarConflict(t *testing.T) {
	t.Parallel()
	_, err := compiler.ApplyMergeRules("a", "b", compiler.MergingRule{Path: "/root", Kind: compiler.KindScalar})
	if err == nil || !strings.Contains(err.Error(), "scalar conflict") {
		t.Fatalf("expected scalar conflict, got %v", err)
	}
}

func TestApplyMergeRulesTypeMismatch(t *testing.T) {
	t.Parallel()
	_, err := compiler.ApplyMergeRules(map[string]any{}, []any{}, compiler.MergingRule{Path: "/root", Kind: compiler.KindMap})
	if err == nil || !strings.Contains(err.Error(), "type mismatch") {
		t.Fatalf("expected type mismatch, got %v", err)
	}
}

func TestApplyMergeRulesListValidation(t *testing.T) {
	t.Parallel()
	_, err := compiler.ApplyMergeRules([]any{"a"}, []any{"b"}, compiler.MergingRule{Path: "/list", Kind: compiler.KindScalarList})
	if err != nil {
		t.Fatalf("expected scalar list merge success, got %v", err)
	}
	_, err = compiler.ApplyMergeRules([]any{map[string]any{}}, []any{}, compiler.MergingRule{Path: "/list", Kind: compiler.KindScalarList})
	if err == nil || !strings.Contains(err.Error(), "scalar list item must be a scalar") {
		t.Fatalf("expected scalar list error, got %v", err)
	}
	_, err = compiler.ApplyMergeRules([]any{"a"}, []any{1}, compiler.MergingRule{Path: "/list", Kind: compiler.KindList, IDAttr: "id", IDOptional: true})
	if err == nil {
		t.Fatalf("expected list item map error, got nil")
	}
}
