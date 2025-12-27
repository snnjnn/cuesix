package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSplitComma(t *testing.T) {
	got := splitComma("a, b,,c ")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("unexpected split result: %#v", got)
	}
}

func TestEnvBool(t *testing.T) {
	t.Setenv("CUESIX_BOOL", "true")
	if !envBool("CUESIX_BOOL", false) {
		t.Fatalf("expected true")
	}
	t.Setenv("CUESIX_BOOL", "0")
	if envBool("CUESIX_BOOL", true) {
		t.Fatalf("expected false")
	}
}

func TestEnvDuration(t *testing.T) {
	t.Setenv("CUESIX_DUR", "150ms")
	if envDuration("CUESIX_DUR", 0) != 150*time.Millisecond {
		t.Fatalf("unexpected duration")
	}
}

func TestBuildFilesystems(t *testing.T) {
	dir := t.TempDir()
	fses, err := buildFilesystems([]string{dir})
	if err != nil {
		t.Fatalf("buildFilesystems returned error: %v", err)
	}
	if len(fses) != 1 {
		t.Fatalf("expected one filesystem")
	}
}

func TestBuildPluginsInvalidPath(t *testing.T) {
	_, err := buildPlugins([]string{"/path/does/not/exist"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestBuildPluginsValidPath(t *testing.T) {
	dir := t.TempDir()
	_, err := buildPlugins([]string{filepath.Clean(dir)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEnvStringDefault(t *testing.T) {
	t.Setenv("CUESIX_STR", "")
	if envStringDefault("CUESIX_STR", "fallback") != "fallback" {
		t.Fatalf("expected fallback")
	}
	t.Setenv("CUESIX_STR", "value")
	if envStringDefault("CUESIX_STR", "fallback") != "value" {
		t.Fatalf("expected value")
	}
}

func TestEnvIntFloat(t *testing.T) {
	t.Setenv("CUESIX_INT", "42")
	if envInt("CUESIX_INT", 0) != 42 {
		t.Fatalf("expected int")
	}
	t.Setenv("CUESIX_FLOAT", "1.5")
	if envFloat("CUESIX_FLOAT", 0) != 1.5 {
		t.Fatalf("expected float")
	}
}

func TestBuildFilesystemsSkipsEmpty(t *testing.T) {
	dir := t.TempDir()
	fses, err := buildFilesystems([]string{"", dir})
	if err != nil {
		t.Fatalf("buildFilesystems returned error: %v", err)
	}
	if len(fses) != 1 {
		t.Fatalf("expected one filesystem")
	}
}

func TestEnvString(t *testing.T) {
	t.Setenv("CUESIX_RAW", "raw")
	if envString("CUESIX_RAW") != "raw" {
		t.Fatalf("expected raw value")
	}
}

func TestBuildPluginsEmpty(t *testing.T) {
	p, err := buildPlugins(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p == nil {
		t.Fatalf("expected non-nil plugin")
	}
}

func TestBuildFilesystemsMissing(t *testing.T) {
	_, err := buildFilesystems([]string{"/missing"})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestEnvBoolDefault(t *testing.T) {
	os.Unsetenv("CUESIX_BOOL_DEFAULT")
	if !envBool("CUESIX_BOOL_DEFAULT", true) {
		t.Fatalf("expected default true")
	}
}
