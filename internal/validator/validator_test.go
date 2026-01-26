package validator_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/warpcomdev/sixpack/internal/testutil"
	"github.com/warpcomdev/sixpack/internal/validator"
)

func TestBuildConfigPath(t *testing.T) {
	t.Setenv("APISIX_PROFILE", "")
	if got := validator.BuildConfigPath("/apisix", false); got != "/apisix/conf/apisix.json" {
		t.Fatalf("unexpected path %s", got)
	}
	t.Setenv("APISIX_PROFILE", "dev")
	if got := validator.BuildConfigPath("/apisix", true); got != "/apisix/conf/apisix-dev.yaml" {
		t.Fatalf("unexpected profiled path %s", got)
	}
}

func TestNewAndCleanupMirror(t *testing.T) {
	t.Parallel()
	sourceDir := t.TempDir()
	mirrorRoot := t.TempDir()
	mirrorDir := filepath.Join(mirrorRoot, "mirror")
	if err := os.MkdirAll(mirrorDir, 0o755); err != nil {
		t.Fatalf("mkdir mirror: %v", err)
	}
	// create source conf structure
	if err := os.MkdirAll(filepath.Join(sourceDir, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir source conf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceDir, "conf", "apisix.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write source file: %v", err)
	}
	v, err := validator.New(testutil.Logger(), sourceDir, mirrorDir, false, 0, testutil.NewMock())
	if err != nil {
		t.Fatalf("expected New to succeed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(mirrorDir, "conf", "apisix.json")); err != nil {
		t.Fatalf("expected file copied to mirror: %v", err)
	}
	if err := v.Cleanup(); err != nil {
		t.Fatalf("Cleanup error: %v", err)
	}
	if _, err := os.Stat(mirrorDir); !os.IsNotExist(err) {
		t.Fatalf("expected mirror dir removed, got %v", err)
	}
}

func TestNewValidatesInputs(t *testing.T) {
	t.Parallel()
	if _, err := validator.New(testutil.Logger(), "", "mirror", false, 0, testutil.NewMock()); !errors.Is(err, validator.ErrSourceDirRequired) {
		t.Fatalf("expected source dir required, got %v", err)
	}
	if _, err := validator.New(testutil.Logger(), "missing", "mirror", false, 0, testutil.NewMock()); !errors.Is(err, validator.ErrMissingDir) {
		t.Fatalf("expected missing dir error, got %v", err)
	}
	// mirror required
	source := t.TempDir()
	if _, err := validator.New(testutil.Logger(), source, "", false, 0, testutil.NewMock()); !errors.Is(err, validator.ErrMirrorDirRequired) {
		t.Fatalf("expected mirror dir required, got %v", err)
	}
}

func TestValidateRunsRunner(t *testing.T) {
	t.Parallel()
	sourceDir := t.TempDir()
	mirrorDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sourceDir, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir source conf: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(mirrorDir, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir mirror conf: %v", err)
	}
	mock := testutil.NewMock()
	v, err := validator.New(testutil.Logger(), sourceDir, mirrorDir, true, 0, mock)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	ok, err := v.Validate([]byte("{}"), false)
	if err != nil || !ok {
		t.Fatalf("expected validation success, got ok=%v err=%v", ok, err)
	}
	if len(mock.Inputs) != 1 {
		t.Fatalf("expected runner called once, got %d", len(mock.Inputs))
	}
	call := mock.Inputs[0]
	if call.WorkDir != mirrorDir {
		t.Fatalf("unexpected workdir %s", call.WorkDir)
	}
	if call.Cmd != "apisix" {
		t.Fatalf("expected apisix command, got %s", call.Cmd)
	}
}

func TestValidateErrors(t *testing.T) {
	t.Parallel()
	v := validator.Validator{}
	if _, err := v.Validate([]byte{}, false); err == nil {
		t.Fatalf("expected error for empty candidate")
	}

	sourceDir := t.TempDir()
	mirrorDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sourceDir, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir source conf: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(mirrorDir, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir mirror conf: %v", err)
	}
	mock := testutil.NewMock(testutil.MockOutput{
		Err:    errors.New("boom"),
		Stderr: []byte("stderr"),
	})
	v, err := validator.New(testutil.Logger(), sourceDir, mirrorDir, true, 10*time.Second, mock)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	ok, err := v.Validate([]byte("{}"), true)
	if err == nil || ok {
		t.Fatalf("expected validation failure, got ok=%v err=%v", ok, err)
	}
	if ve, ok := err.(*validator.ValidationError); !ok || string(ve.Output) != "stderr" {
		t.Fatalf("expected ValidationError with stderr, got %T %v", err, err)
	}
}
