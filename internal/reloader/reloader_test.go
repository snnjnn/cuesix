package reloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyReplacesFileAndReloads(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "apisix.yaml")
	if err := os.WriteFile(targetPath, []byte("old"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	tempPath := filepath.Join(dir, "temp.yaml")
	if err := os.WriteFile(tempPath, []byte("new"), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	var gotMethod string
	var gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotHeader = r.Header.Get("X-API-KEY")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	rel := &Reloader{
		ConfigPath: targetPath,
		ReloadURL:  server.URL,
		APIKey:     "secret",
	}

	if err := rel.Apply(context.Background(), tempPath); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(content) != "new" {
		t.Fatalf("expected new content, got %q", string(content))
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotHeader != "secret" {
		t.Fatalf("expected API key, got %q", gotHeader)
	}
}

func TestApplyRejectsMissingConfig(t *testing.T) {
	rel := &Reloader{
		ReloadURL: "http://example",
	}
	if err := rel.Apply(context.Background(), "temp"); err == nil {
		t.Fatalf("expected error for missing config path")
	}
}

func TestApplyRetriesReload(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "apisix.yaml")
	if err := os.WriteFile(targetPath, []byte("old"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	tempPath := filepath.Join(dir, "temp.yaml")
	if err := os.WriteFile(tempPath, []byte("new"), 0o600); err != nil {
		t.Fatalf("write temp: %v", err)
	}

	var calls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls < 3 {
			http.Error(w, "try again", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	rel := &Reloader{
		ConfigPath:     targetPath,
		ReloadURL:      server.URL,
		RetryMax:       3,
		RetryInitial:   1 * time.Millisecond,
		RetryMaxDelay:  5 * time.Millisecond,
		RetryMultiplier: 2,
	}

	if err := rel.Apply(context.Background(), tempPath); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}
