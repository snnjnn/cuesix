package reloader_test

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/reloader"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

type fakeFileInfo struct {
	mode fs.FileMode
}

func (i fakeFileInfo) Name() string       { return "config.json" }
func (i fakeFileInfo) Size() int64        { return 0 }
func (i fakeFileInfo) Mode() fs.FileMode  { return i.mode }
func (i fakeFileInfo) ModTime() time.Time { return time.Unix(0, 0) }
func (i fakeFileInfo) IsDir() bool        { return false }
func (i fakeFileInfo) Sys() any           { return nil }

type fakeTempFile struct {
	name     string
	writeErr error
	syncErr  error
	closeErr error
	data     []byte
	closed   bool
}

func (f *fakeTempFile) Stat() (fs.FileInfo, error) { return fakeFileInfo{mode: 0o600}, nil }
func (f *fakeTempFile) Close() error {
	f.closed = true
	return f.closeErr
}
func (f *fakeTempFile) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	f.data = append(f.data, p...)
	return len(p), nil
}
func (f *fakeTempFile) Name() string { return f.name }
func (f *fakeTempFile) Sync() error  { return f.syncErr }

type fakeFS struct {
	tempFile    *fakeTempFile
	createErr   error
	statInfo    fs.FileInfo
	statErr     error
	chmodErr    error
	renameErr   error
	removeErr   error
	renamedFrom string
	renamedTo   string
	removed     []string
	chmodName   string
	chmodMode   fs.FileMode
}

func (f *fakeFS) Stat(name string) (fs.FileInfo, error) {
	if f.statErr != nil {
		return nil, f.statErr
	}
	if f.statInfo == nil {
		return nil, fs.ErrNotExist
	}
	return f.statInfo, nil
}

func (f *fakeFS) CreateTemp(dir, pattern string) (reloader.TempFile, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	if f.tempFile == nil {
		f.tempFile = &fakeTempFile{name: filepath.Join(dir, "tmp-config")}
	}
	return f.tempFile, nil
}

func (f *fakeFS) Chmod(name string, mode fs.FileMode) error {
	f.chmodName = name
	f.chmodMode = mode
	return f.chmodErr
}

func (f *fakeFS) Rename(oldPath, newPath string) error {
	f.renamedFrom = oldPath
	f.renamedTo = newPath
	return f.renameErr
}

func (f *fakeFS) Remove(name string) error {
	f.removed = append(f.removed, name)
	return f.removeErr
}

func TestApplyWritesConfig(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	r := &reloader.FileReloader{
		Virtualgw:  compiler.DEFAULT_VIRTUALGW,
		ConfigPath: configPath,
		Logger:     testutil.Logger(),
	}
	if err := r.Apply(context.Background(), compiler.DEFAULT_VIRTUALGW, []byte(`{"a":1}`)); err != nil {
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

func TestApplySkipsUnconfiguredVirtualGateway(t *testing.T) {
	t.Parallel()

	fsys := &fakeFS{}
	r := &reloader.FileReloader{
		Virtualgw:  compiler.DEFAULT_VIRTUALGW,
		ConfigPath: "config.json",
		Logger:     testutil.Logger(),
		FS:         fsys,
	}

	if err := r.Apply(context.Background(), "secondary", []byte(`{"a":1}`)); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if fsys.tempFile != nil {
		t.Fatalf("expected no temp file creation for unconfigured virtual gateway")
	}
	if fsys.renamedFrom != "" || fsys.renamedTo != "" {
		t.Fatalf("unexpected rename = %q -> %q", fsys.renamedFrom, fsys.renamedTo)
	}
	if len(fsys.removed) != 0 {
		t.Fatalf("unexpected cleanup calls: %v", fsys.removed)
	}
}

func TestApplyValidationErrors(t *testing.T) {
	t.Parallel()

	if err := (*reloader.FileReloader)(nil).Apply(context.Background(), compiler.DEFAULT_VIRTUALGW, []byte("x")); err == nil {
		t.Fatalf("expected error for nil reloader")
	}

	r := &reloader.FileReloader{Virtualgw: compiler.DEFAULT_VIRTUALGW}
	if err := r.Apply(context.Background(), compiler.DEFAULT_VIRTUALGW, []byte("x")); err == nil {
		t.Fatalf("expected error for empty config path")
	}

	r.ConfigPath = "file"
	if err := r.Apply(context.Background(), compiler.DEFAULT_VIRTUALGW, nil); err == nil {
		t.Fatalf("expected error for empty payload")
	}
}

func TestApplyWithInjectedFSFailurePaths(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fsys     *fakeFS
		wantErr  string
		wantRm   bool
		wantMode fs.FileMode
	}{
		{
			name:    "create temp failure",
			fsys:    &fakeFS{createErr: errors.New("boom")},
			wantErr: "create temp file: boom",
		},
		{
			name:    "write failure cleans up",
			fsys:    &fakeFS{tempFile: &fakeTempFile{name: "tmp-file", writeErr: errors.New("boom")}},
			wantErr: "write temp file: boom",
			wantRm:  true,
		},
		{
			name:    "sync failure cleans up",
			fsys:    &fakeFS{tempFile: &fakeTempFile{name: "tmp-file", syncErr: errors.New("boom")}},
			wantErr: "sync temp file: boom",
			wantRm:  true,
		},
		{
			name:    "close failure cleans up",
			fsys:    &fakeFS{tempFile: &fakeTempFile{name: "tmp-file", closeErr: errors.New("boom")}},
			wantErr: "close temp file: boom",
			wantRm:  true,
		},
		{
			name:    "stat unexpected failure cleans up",
			fsys:    &fakeFS{tempFile: &fakeTempFile{name: "tmp-file"}, statErr: errors.New("boom")},
			wantErr: "stat config file: boom",
			wantRm:  true,
		},
		{
			name:     "chmod failure cleans up",
			fsys:     &fakeFS{tempFile: &fakeTempFile{name: "tmp-file"}, statInfo: fakeFileInfo{mode: 0o640}, chmodErr: errors.New("boom")},
			wantErr:  "chmod temp file: boom",
			wantRm:   true,
			wantMode: 0o640,
		},
		{
			name:    "rename failure cleans up",
			fsys:    &fakeFS{tempFile: &fakeTempFile{name: "tmp-file"}, renameErr: errors.New("boom")},
			wantErr: "replace config: boom",
			wantRm:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &reloader.FileReloader{
				Virtualgw:  compiler.DEFAULT_VIRTUALGW,
				ConfigPath: "config.json",
				Logger:     testutil.Logger(),
				FS:         tt.fsys,
			}

			err := r.Apply(context.Background(), compiler.DEFAULT_VIRTUALGW, []byte(`{"a":1}`))
			if err == nil {
				t.Fatalf("expected error")
			}
			if got := err.Error(); !strings.Contains(got, tt.wantErr) {
				t.Fatalf("error = %q, want substring %q", got, tt.wantErr)
			}
			if tt.wantRm && len(tt.fsys.removed) == 0 {
				t.Fatalf("expected temp file cleanup")
			}
			if tt.wantMode != 0 && tt.fsys.chmodMode != tt.wantMode {
				t.Fatalf("chmod mode = %v, want %v", tt.fsys.chmodMode, tt.wantMode)
			}
		})
	}
}

func TestApplyWithInjectedFSSuccessAndModePropagation(t *testing.T) {
	t.Parallel()

	fsys := &fakeFS{
		tempFile: &fakeTempFile{name: "tmp-file"},
		statInfo: fakeFileInfo{mode: 0o640},
	}
	r := &reloader.FileReloader{
		Virtualgw:  compiler.DEFAULT_VIRTUALGW,
		ConfigPath: "config.json",
		Logger:     testutil.Logger(),
		FS:         fsys,
	}

	if err := r.Apply(context.Background(), compiler.DEFAULT_VIRTUALGW, []byte(`{"a":1}`)); err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := string(fsys.tempFile.data); got != `{"a":1}` {
		t.Fatalf("temp file payload = %q", got)
	}
	if fsys.renamedFrom != "tmp-file" || fsys.renamedTo != "config.json" {
		t.Fatalf("rename = %q -> %q", fsys.renamedFrom, fsys.renamedTo)
	}
	if fsys.chmodName != "tmp-file" || fsys.chmodMode != 0o640 {
		t.Fatalf("chmod = %q %v", fsys.chmodName, fsys.chmodMode)
	}
	if len(fsys.removed) != 0 {
		t.Fatalf("unexpected cleanup calls: %v", fsys.removed)
	}
}
