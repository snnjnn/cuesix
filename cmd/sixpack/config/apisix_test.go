package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/cmd/sixpack/config"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

func TestAPISIXMirrorCreationAndCleanup(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "conf", "apisix.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg := config.StandaloneValidator{ValidationTimeout: time.Second}
	v, err := cfg.BuildValidator(testutil.Logger(), home)
	if err != nil {
		t.Fatalf("BuildValidator error: %v", err)
	}
	// Mirror dir should exist and be preserved because keep-mirror defaults to true when auto-created.
	if info, err := os.Stat(v.MirrorDir()); err != nil || !info.IsDir() {
		t.Fatalf("mirror dir missing or not dir: %v", err)
	}
	if err := v.Cleanup(); err != nil {
		t.Fatalf("Cleanup error: %v", err)
	}
	if _, err := os.Stat(v.MirrorDir()); !os.IsNotExist(err) {
		t.Fatalf("expected mirror dir removed after cleanup, got %v", err)
	}
}
