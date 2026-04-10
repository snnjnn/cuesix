package factory_test

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/warpcomdev/cuesix/cmd/sixpack/config"
	"github.com/warpcomdev/cuesix/cmd/sixpack/factory"
	"github.com/warpcomdev/cuesix/internal/compiler"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestSchedulerMustAndMight(t *testing.T) {
	t.Parallel()
	s := factory.NewScheduler()
	started := make(chan struct{})
	release := make(chan struct{})
	doneMust := make(chan struct{})
	go s.Must(context.Background(), func() {
		close(started)
		<-release
		close(doneMust)
	})
	<-started
	if executed := s.Might(context.Background(), func() {}); executed {
		t.Fatalf("expected Might to skip when locked")
	}
	close(release)
	<-doneMust
	doneMight := make(chan struct{})
	if executed := s.Might(context.Background(), func() { close(doneMight) }); !executed {
		t.Fatalf("expected Might to execute after unlock")
	}
	<-doneMight
}

func TestSerializerInstanceSerializeCachesCommittedHash(t *testing.T) {
	t.Parallel()

	sf, err := factory.NewSerializer(testLogger(), config.Plugins{}, config.Apisix{}, factory.SSLSetup{}, factory.NewScheduler())
	if err != nil {
		t.Fatalf("NewSerializer() error = %v", err)
	}

	inst := sf.Instance(compiler.DEFAULT_VIRTUALGW)
	inst.Reset()
	first, err := inst.Serialize(map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	if len(first) == 0 {
		t.Fatalf("expected serialized payload")
	}
	inst.Commit()

	inst.Reset()
	second, err := inst.Serialize(map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("Serialize() second error = %v", err)
	}
	if second != nil {
		t.Fatalf("expected cache hit, got payload %q", string(second))
	}
}

func TestSerializerInstanceSerializeYamlPlugin(t *testing.T) {
	t.Parallel()

	sf, err := factory.NewSerializer(testLogger(), config.Plugins{}, config.Apisix{OutputYAML: true}, factory.SSLSetup{}, factory.NewScheduler())
	if err != nil {
		t.Fatalf("NewSerializer() error = %v", err)
	}

	inst := sf.Instance(compiler.DEFAULT_VIRTUALGW)
	inst.Reset()
	out, err := inst.Serialize(map[string]any{"x": 1})
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	if len(out) == 0 {
		t.Fatalf("expected serialized payload")
	}
	if string(out)[:2] == "{\"" {
		t.Fatalf("expected YAML output, got %q", string(out))
	}
}

func TestSerializerInstanceSerializeAddsManagedLabels(t *testing.T) {
	t.Parallel()

	sf, err := factory.NewSerializer(testLogger(), config.Plugins{}, config.Apisix{EnableLabels: true}, factory.SSLSetup{}, factory.NewScheduler())
	if err != nil {
		t.Fatalf("NewSerializer() error = %v", err)
	}

	inst := sf.Instance("edge")
	inst.Reset()
	out, err := inst.Serialize(map[string]any{
		"routes": []any{
			map[string]any{"id": "r1"},
		},
		"plugin_metadata": []any{
			map[string]any{"id": "limit-count"},
		},
	})
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}

	if !strings.Contains(string(out), "\"managed-by\":\"sixpack\"") {
		t.Fatalf("missing managed-by label in %s", out)
	}
	if !strings.Contains(string(out), "\"sixpack-label\":\"edge\"") {
		t.Fatalf("missing sixpack-label in %s", out)
	}
	if strings.Contains(string(out), "\"plugin_metadata\":[{\"labels\"") {
		t.Fatalf("plugin_metadata unexpectedly labeled: %s", out)
	}
}

func TestSerializerInstanceSerializeSkipsManagedLabelsWhenDisabled(t *testing.T) {
	t.Parallel()

	sf, err := factory.NewSerializer(testLogger(), config.Plugins{}, config.Apisix{}, factory.SSLSetup{}, factory.NewScheduler())
	if err != nil {
		t.Fatalf("NewSerializer() error = %v", err)
	}

	inst := sf.Instance("edge")
	inst.Reset()
	out, err := inst.Serialize(map[string]any{
		"routes": []any{
			map[string]any{"id": "r1"},
		},
	})
	if err != nil {
		t.Fatalf("Serialize() error = %v", err)
	}
	if strings.Contains(string(out), "\"managed-by\":\"sixpack\"") || strings.Contains(string(out), "\"sixpack-label\":") {
		t.Fatalf("managed labels unexpectedly added: %s", out)
	}
}

func TestCompilerFactoryMergeDeepCopy(t *testing.T) {
	t.Parallel()

	source := map[string]any{
		"routes": []any{
			map[string]any{
				"id": "r1",
				"labels": map[string]any{
					"team": "api",
				},
			},
		},
	}

	factoryWithCopy := factory.CompilerFactory{Logger: testLogger(), DeepCopy: true}
	merged, err := factoryWithCopy.Merge(func(yield func(compiler.Snippet) bool) {
		yield(compiler.Snippet{
			Ref:       compiler.SourceRef{Namespace: "test", Path: "routes.yaml"},
			Virtualgw: compiler.FromKey(compiler.DEFAULT_VIRTUALGW),
			Data:      source,
		})
	})
	if err != nil {
		t.Fatalf("Merge() error = %v", err)
	}

	routes := merged["routes"].([]any)
	route := routes[0].(map[string]any)
	labels := route["labels"].(map[string]any)
	labels["managed-by"] = "sixpack"

	sourceRoute := source["routes"].([]any)[0].(map[string]any)
	sourceLabels := sourceRoute["labels"].(map[string]any)
	if _, ok := sourceLabels["managed-by"]; ok {
		t.Fatalf("source mutated after deep copy: %v", sourceLabels)
	}
}
