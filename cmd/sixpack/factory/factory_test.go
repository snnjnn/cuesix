package factory_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/warpcomdev/sixpack/cmd/sixpack/factory"
)

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
