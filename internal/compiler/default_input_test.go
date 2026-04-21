package compiler_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/warpcomdev/cuesix/internal/compiler"
)

func TestNewDefaultInput(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	input, err := compiler.InputFromPaths([]string{dir})
	if err != nil {
		t.Fatalf("NewDefaultInput() error = %v", err)
	}

	// DefaultInput never return an error
	if namespaces, _ := input.Namespaces(); len(namespaces) != 1 || namespaces[0] != dir {
		t.Fatalf("unexpected namespaces %v", namespaces)
	}
	if _, err := compiler.InputFromPaths([]string{filepath.Join(dir, "missing")}); err == nil {
		t.Fatalf("expected error for missing path")
	}
}

func TestNewDefaultInputSkipsEmptyPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	input, err := compiler.InputFromPaths([]string{"", dir, ""})
	if err != nil {
		t.Fatalf("NewDefaultInput() error = %v", err)
	}
	if namespaces, _ := input.Namespaces(); len(namespaces) != 1 || namespaces[0] != dir {
		t.Fatalf("unexpected namespaces %v", namespaces)
	}
}

func TestNewDefaultInputProducesUsableFS(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("data"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	input, err := compiler.InputFromPaths([]string{dir})
	if err != nil {
		t.Fatalf("NewDefaultInput() error = %v", err)
	}
	fses := input.Filesystems()
	if len(fses) != 1 {
		t.Fatalf("len(filesystems) = %d", len(fses))
	}
	data, err := fs.ReadFile(fses[0], "file.txt")
	if err != nil {
		t.Fatalf("fs.ReadFile() error = %v", err)
	}
	if string(data) != "data" {
		t.Fatalf("data = %q", string(data))
	}
}

func TestNewDefaultInputPreservesInputOrder(t *testing.T) {
	t.Parallel()

	first := t.TempDir()
	second := t.TempDir()

	input, err := compiler.InputFromPaths([]string{second, first})
	if err != nil {
		t.Fatalf("NewDefaultInput() error = %v", err)
	}
	namespaces, _ := input.Namespaces()
	if len(namespaces) != 2 {
		t.Fatalf("len(order) = %d", len(namespaces))
	}
	if namespaces[0] != second || namespaces[1] != first {
		t.Fatalf("order = %v", namespaces)
	}
}

func TestNewDefaultInputFSUsesProvidedOrder(t *testing.T) {
	t.Parallel()

	input := compiler.InputFromFS(map[string]fs.FS{
		"b": fstest.MapFS{},
		"a": fstest.MapFS{},
	}, []string{"b", "a"})

	namespaces, _ := input.Namespaces()
	if len(namespaces) != 2 {
		t.Fatalf("len(order) = %d", len(namespaces))
	}
	if namespaces[0] != "b" || namespaces[1] != "a" {
		t.Fatalf("order = %v", namespaces)
	}
}

func TestNewDefaultInputFSAppendsRemainingSorted(t *testing.T) {
	t.Parallel()

	input := compiler.InputFromFS(map[string]fs.FS{
		"c": fstest.MapFS{},
		"a": fstest.MapFS{},
		"b": fstest.MapFS{},
	}, []string{"c"})

	namespaces, _ := input.Namespaces()
	want := []string{"c", "a", "b"}
	if len(namespaces) != len(want) {
		t.Fatalf("len(order) = %d", len(namespaces))
	}
	for i := range want {
		if namespaces[i] != want[i] {
			t.Fatalf("order = %v", namespaces)
		}
	}
}
