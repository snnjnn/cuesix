package validator

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/internal/testutil"
)

type mockCommandRunner struct {
	RunCommandFunc func(ctx context.Context, workDir string, name string, args ...string) ([]byte, error)
}

func (m *mockCommandRunner) RunCommand(ctx context.Context, workDir string, name string, args ...string) ([]byte, error) {
	if m.RunCommandFunc != nil {
		return m.RunCommandFunc(ctx, workDir, name, args...)
	}
	return nil, nil
}

func TestBuildConfigPath(t *testing.T) {
	t.Setenv("APISIX_PROFILE", "blue")
	path := BuildConfigPath("/usr/local/apisix", false)
	want := filepath.Join("/usr/local/apisix", "conf", "apisix-blue.json")
	if path != want {
		t.Fatalf("expected %q, got %q", want, path)
	}
	t.Setenv("APISIX_PROFILE", "")
	path = BuildConfigPath("/usr/local/apisix", true)
	want = filepath.Join("/usr/local/apisix", "conf", "apisix.yaml")
	if path != want {
		t.Fatalf("expected %q, got %q", want, path)
	}
}

func TestNewValidator(t *testing.T) {
	sourceDir := createSourceDir(t, false)
	v, err := New(sourceDir, t.TempDir(), false, 0)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if v == nil {
		t.Fatal("New() should not return nil")
	}
}

func TestNewValidator_EmptyMirrorDir(t *testing.T) {
	sourceDir := createSourceDir(t, false)
	if _, err := New(sourceDir, "", false, 0); err == nil {
		t.Fatalf("expected error for empty mirror dir")
	}
}

func TestPrepareMirror_ErrorsWithoutMirrorDir(t *testing.T) {
	sourceDir := createSourceDir(t, false)
	if err := prepareMirror(sourceDir, "", false); err == nil {
		t.Fatalf("expected error for empty mirror dir")
	}
}

func TestPrepareMirror_ErrorsWithoutSourceDir(t *testing.T) {
	if err := prepareMirror("", t.TempDir(), false); err == nil {
		t.Fatalf("expected error for empty source dir")
	}
}

func TestPrepareMirror_ClearsAndCopies(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sourceDir, "conf"), 0o755); err != nil {
		t.Fatalf("create source conf dir: %v", err)
	}
	sourceFile := filepath.Join(sourceDir, "conf", "config.yaml")
	if err := os.WriteFile(sourceFile, []byte("apisix:\n  node_listen: 9080\n"), 0o644); err != nil {
		t.Fatalf("write source config: %v", err)
	}

	mirrorDir := t.TempDir()
	junkFile := filepath.Join(mirrorDir, "junk.txt")
	if err := os.WriteFile(junkFile, []byte("junk"), 0o644); err != nil {
		t.Fatalf("write junk file: %v", err)
	}

	if err := prepareMirror(sourceDir, mirrorDir, false); err != nil {
		t.Fatalf("prepareMirror returned error: %v", err)
	}
	if _, err := os.Stat(junkFile); !os.IsNotExist(err) {
		t.Fatalf("expected junk file to be removed")
	}
	if _, err := os.Stat(filepath.Join(mirrorDir, "conf", "config.yaml")); err != nil {
		t.Fatalf("expected mirrored config.yaml: %v", err)
	}
}

func TestPrepareMirrorUseExistingKeepsContent(t *testing.T) {
	sourceDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(sourceDir, "conf"), 0o755); err != nil {
		t.Fatalf("create source conf dir: %v", err)
	}
	sourceFile := filepath.Join(sourceDir, "conf", "config.yaml")
	if err := os.WriteFile(sourceFile, []byte("apisix:\n  node_listen: 9080\n"), 0o644); err != nil {
		t.Fatalf("write source config: %v", err)
	}

	mirrorDir := t.TempDir()
	junkFile := filepath.Join(mirrorDir, "junk.txt")
	if err := os.WriteFile(junkFile, []byte("junk"), 0o644); err != nil {
		t.Fatalf("write junk file: %v", err)
	}

	if err := prepareMirror(sourceDir, mirrorDir, true); err != nil {
		t.Fatalf("prepareMirror returned error: %v", err)
	}
	if _, err := os.Stat(junkFile); err != nil {
		t.Fatalf("expected junk file to remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(mirrorDir, "conf", "config.yaml")); err == nil {
		t.Fatalf("expected not to mirror config.yaml")
	}
}

func TestValidator_Validate_SyntacticallyValid(t *testing.T) {
	t.Setenv("APISIX_PROFILE", "default")

	mockRunner := &mockCommandRunner{
		RunCommandFunc: func(ctx context.Context, workDir string, name string, args ...string) ([]byte, error) {
			if name != "apisix" {
				t.Fatalf("unexpected command: %s", name)
			}
			if len(args) != 3 || args[0] != "test" || args[1] != "-c" {
				t.Fatalf("unexpected args: %v", args)
			}
			configPath := args[2]
			mirrorDir := filepath.Dir(filepath.Dir(configPath))
			if workDir != mirrorDir {
				t.Fatalf("unexpected workDir: %s", workDir)
			}
			candidatePath := filepath.Join(mirrorDir, "conf", "apisix-default.json")
			got, err := os.ReadFile(candidatePath)
			if err != nil {
				t.Fatalf("read candidate in temp dir: %v", err)
			}
			if string(got) != `{"new":true}` {
				t.Fatalf("unexpected candidate content: %q", string(got))
			}
			return []byte("success"), nil
		},
	}

	sourceDir := createSourceDir(t, false)
	v, err := newWithRunner(sourceDir, t.TempDir(), false, 0, mockRunner)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if v == nil {
		t.Fatal("New() should not return nil")
	}

	candidate := []byte(`{"new":true}`)
	valid, err := v.Validate(testutil.Logger(), candidate, false)

	if !valid || err != nil {
		t.Errorf("Expected valid=true, err=nil for a syntactically valid config, got valid=%t, err=%v", valid, err)
	}
}

func TestValidator_Validate_YAMLProfileExtension(t *testing.T) {
	t.Setenv("APISIX_PROFILE", "default")

	mockRunner := &mockCommandRunner{
		RunCommandFunc: func(ctx context.Context, workDir string, name string, args ...string) ([]byte, error) {
			if len(args) != 3 || args[0] != "test" || args[1] != "-c" {
				t.Fatalf("unexpected args: %v", args)
			}
			configPath := args[2]
			mirrorDir := filepath.Dir(filepath.Dir(configPath))
			if workDir != mirrorDir {
				t.Fatalf("unexpected workDir: %s", workDir)
			}
			candidatePath := filepath.Join(mirrorDir, "conf", "apisix-default.yaml")
			got, err := os.ReadFile(candidatePath)
			if err != nil {
				t.Fatalf("read candidate in temp dir: %v", err)
			}
			if string(got) != "routes: []" {
				t.Fatalf("unexpected candidate content: %q", string(got))
			}
			return []byte("success"), nil
		},
	}

	sourceDir := createSourceDir(t, true)
	v, err := newWithRunner(sourceDir, t.TempDir(), false, 0, mockRunner)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	candidate := []byte("routes: []")
	valid, err := v.Validate(testutil.Logger(), candidate, true)
	if !valid || err != nil {
		t.Fatalf("expected valid=true, err=nil, got valid=%t, err=%v", valid, err)
	}
}

func TestValidator_Validate_SuccessOutputIgnored(t *testing.T) {
	t.Setenv("APISIX_PROFILE", "default")

	mockRunner := &mockCommandRunner{
		RunCommandFunc: func(ctx context.Context, workDir string, name string, args ...string) ([]byte, error) {
			return []byte("ok"), nil
		},
	}

	sourceDir := createSourceDir(t, false)
	v, err := newWithRunner(sourceDir, t.TempDir(), false, 0, mockRunner)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	candidate := []byte(`{"new":true}`)
	valid, err := v.Validate(testutil.Logger(), candidate, false)
	if !valid || err != nil {
		t.Fatalf("expected valid=true, err=nil, got valid=%t, err=%v", valid, err)
	}
}

func TestValidator_Validate_SyntacticallyInvalid(t *testing.T) {
	t.Setenv("APISIX_PROFILE", "default")

	mockRunner := &mockCommandRunner{
		RunCommandFunc: func(ctx context.Context, workDir string, name string, args ...string) ([]byte, error) {
			return []byte("error output"), io.ErrUnexpectedEOF
		},
	}

	sourceDir := createSourceDir(t, false)
	v, err := newWithRunner(sourceDir, t.TempDir(), false, 0, mockRunner)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if v == nil {
		t.Fatal("New() should not return nil")
	}

	candidate := []byte(`{"new":true}`)
	valid, err := v.Validate(testutil.Logger(), candidate, false)

	if valid || err == nil {
		t.Errorf("Expected valid=false, err!=nil for a syntactically invalid config, got valid=%t, err=%v", valid, err)
	}
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if string(validationErr.Output) != "error output" {
		t.Fatalf("expected output to be captured, got %q", string(validationErr.Output))
	}
	if validationErr.Unwrap() == nil {
		t.Fatalf("expected wrapped error")
	}
}

func TestValidator_Validate_EmptyProfile(t *testing.T) {
	t.Setenv("APISIX_PROFILE", "")

	mockRunner := &mockCommandRunner{
		RunCommandFunc: func(ctx context.Context, workDir string, name string, args ...string) ([]byte, error) {
			if len(args) != 3 || args[0] != "test" || args[1] != "-c" {
				t.Fatalf("unexpected args: %v", args)
			}
			configPath := args[2]
			mirrorDir := filepath.Dir(filepath.Dir(configPath))
			if workDir != mirrorDir {
				t.Fatalf("unexpected workDir: %s", workDir)
			}
			candidatePath := filepath.Join(mirrorDir, "conf", "apisix.json")
			if _, err := os.Stat(candidatePath); err != nil {
				t.Fatalf("expected apisix.json file: %v", err)
			}
			return []byte("success"), nil
		},
	}
	sourceDir := createSourceDir(t, false)
	v, err := newWithRunner(sourceDir, t.TempDir(), false, 0, mockRunner)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	candidate := []byte(`{"new":true}`)
	valid, err := v.Validate(testutil.Logger(), candidate, false)
	if !valid || err != nil {
		t.Fatalf("expected valid=true, err=nil, got valid=%t, err=%v", valid, err)
	}
}

func TestValidator_Validate_EmptyCandidate(t *testing.T) {
	t.Setenv("APISIX_PROFILE", "default")

	mockRunner := &mockCommandRunner{
		RunCommandFunc: func(ctx context.Context, workDir string, name string, args ...string) ([]byte, error) {
			t.Fatalf("runner should not be called for empty candidate")
			return nil, nil
		},
	}

	sourceDir := createSourceDir(t, false)
	v, err := newWithRunner(sourceDir, t.TempDir(), false, 0, mockRunner)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := v.Validate(testutil.Logger(), nil, false); err == nil {
		t.Fatalf("expected error for empty candidate")
	}
}

func TestValidationErrorErrorString(t *testing.T) {
	plain := &ValidationError{Output: []byte("oops")}
	if plain.Error() == "" {
		t.Fatalf("expected error string")
	}
	withCause := &ValidationError{Output: []byte("oops"), Cause: io.ErrUnexpectedEOF}
	if withCause.Error() == "" {
		t.Fatalf("expected error string with cause")
	}
	noOutput := &ValidationError{Cause: io.ErrUnexpectedEOF}
	if noOutput.Error() == "" {
		t.Fatalf("expected error string without output")
	}
}

func TestValidatorTimeoutPassesContext(t *testing.T) {
	sourceDir := createSourceDir(t, false)
	mockRunner := &mockCommandRunner{
		RunCommandFunc: func(ctx context.Context, workDir string, name string, args ...string) ([]byte, error) {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatalf("expected deadline in context")
			}
			return []byte("ok"), nil
		},
	}
	v, err := newWithRunner(sourceDir, t.TempDir(), false, 10*time.Millisecond, mockRunner)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if _, err := v.Validate(testutil.Logger(), []byte(`{"new":true}`), false); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func createSourceDir(t *testing.T, isYAML bool) string {
	t.Helper()
	dir := t.TempDir()
	dynamicPath := BuildConfigPath(dir, isYAML)
	if err := os.MkdirAll(filepath.Dir(dynamicPath), 0o755); err != nil {
		t.Fatalf("create apisix profile dir: %v", err)
	}
	if err := os.WriteFile(dynamicPath, []byte(`{"old":true}`), 0o644); err != nil {
		t.Fatalf("write apisix profile file: %v", err)
	}
	return dir
}
