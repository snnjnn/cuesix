package factory_test

import (
	"context"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/warpcondev/cuesix/cmd/sixpack/config"
	"github.com/warpcondev/cuesix/cmd/sixpack/factory"
	"github.com/warpcondev/cuesix/internal/compiler"
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

func TestBuildFilesystems(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	fses, err := factory.BuildFilesystems([]string{dir})
	if err != nil {
		t.Fatalf("BuildFilesystems error: %v", err)
	}
	if len(fses) != 1 {
		t.Fatalf("expected one filesystem, got %d", len(fses))
	}
	if _, err := factory.BuildFilesystems([]string{filepath.Join(dir, "missing")}); err == nil {
		t.Fatalf("expected error for missing path")
	}
}

func TestBuildFilesystemsSkipsEmptyPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	fses, err := factory.BuildFilesystems([]string{"", dir, ""})
	if err != nil {
		t.Fatalf("BuildFilesystems() error = %v", err)
	}
	if len(fses) != 1 {
		t.Fatalf("len(filesystems) = %d", len(fses))
	}
}

func TestBuildFilesystemsProducesUsableFS(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	fses, err := factory.BuildFilesystems([]string{dir})
	if err != nil {
		t.Fatalf("BuildFilesystems() error = %v", err)
	}
	data, err := fs.ReadFile(fses[0].FS, "file.txt")
	if err != nil {
		t.Fatalf("fs.ReadFile() error = %v", err)
	}
	if string(data) != "data" {
		t.Fatalf("data = %q", string(data))
	}
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
			Ref:       compiler.SourceRef{Root: "test", Path: "routes.yaml"},
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
