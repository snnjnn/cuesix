package reloader_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/warpcomdev/sixpack/internal/reloader"
	"github.com/warpcomdev/sixpack/internal/testutil"
)

func TestApplyWritesConfig(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	r := &reloader.Reloader{
		ConfigPath: configPath,
		Logger:     testutil.Logger(),
	}
	if err := r.Apply(context.Background(), []byte(`{"a":1}`), true); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if string(data) != `{"a":1}` {
		t.Fatalf("unexpected config contents %q", data)
	}
}

func TestApplyValidationErrors(t *testing.T) {
	t.Parallel()
	r := &reloader.Reloader{}
	if err := r.Apply(context.Background(), []byte(""), true); err == nil {
		t.Fatalf("expected error for nil reloader / payload")
	}
	r.ConfigPath = "file"
	if err := r.Apply(context.Background(), []byte{}, true); err == nil {
		t.Fatalf("expected error for empty payload")
	}
}
