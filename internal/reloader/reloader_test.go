package reloader

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/internal/testutil"
)

func TestApplyReplacesFileAndReloads(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "apisix.yaml")
	if err := os.WriteFile(targetPath, []byte("old"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
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

	if err := rel.Apply(context.Background(), testutil.Logger(), []byte("new")); err != nil {
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
	if err := rel.Apply(context.Background(), testutil.Logger(), []byte("payload")); err == nil {
		t.Fatalf("expected error for missing config path")
	}
}

func TestApplyAllowsMissingReloadURL(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "apisix.yaml")
	if err := os.WriteFile(targetPath, []byte("old"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	counter := &countingTransport{}
	client := &http.Client{Transport: counter}
	rel := &Reloader{
		ConfigPath: targetPath,
		HTTPClient: client,
	}

	if err := rel.Apply(context.Background(), testutil.Logger(), []byte("payload")); err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if counter.calls != 0 {
		t.Fatalf("expected no reload request, got %d", counter.calls)
	}
	content, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if string(content) != "payload" {
		t.Fatalf("expected payload written, got %q", string(content))
	}
}

func TestApplyRejectsEmptyPayload(t *testing.T) {
	rel := &Reloader{
		ConfigPath: "/tmp/apisix.yaml",
		ReloadURL:  "http://example",
	}
	if err := rel.Apply(context.Background(), testutil.Logger(), nil); err == nil {
		t.Fatalf("expected error for empty payload")
	}
}

func TestApplyRetriesReload(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "apisix.yaml")
	if err := os.WriteFile(targetPath, []byte("old"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
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
		ConfigPath:      targetPath,
		ReloadURL:       server.URL,
		RetryMax:        3,
		RetryInitial:    1 * time.Millisecond,
		RetryMaxDelay:   5 * time.Millisecond,
		RetryMultiplier: 2,
	}

	if err := rel.Apply(context.Background(), testutil.Logger(), []byte("new")); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("expected 3 calls, got %d", calls)
	}
}

func TestApplyPreservesFileMode(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "apisix.yaml")
	if err := os.WriteFile(targetPath, []byte("old"), 0o640); err != nil {
		t.Fatalf("write target: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	rel := &Reloader{
		ConfigPath: targetPath,
		ReloadURL:  server.URL,
	}

	if err := rel.Apply(context.Background(), testutil.Logger(), []byte("new")); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	info, err := os.Stat(targetPath)
	if err != nil {
		t.Fatalf("stat target: %v", err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("expected mode 0640, got %o", info.Mode().Perm())
	}
}

func TestApplyUsesCustomMethod(t *testing.T) {
	dir := t.TempDir()
	targetPath := filepath.Join(dir, "apisix.yaml")
	if err := os.WriteFile(targetPath, []byte("old"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}

	var gotMethod string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	rel := &Reloader{
		ConfigPath:   targetPath,
		ReloadURL:    server.URL,
		ReloadMethod: http.MethodPut,
	}

	if err := rel.Apply(context.Background(), testutil.Logger(), []byte("new")); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if gotMethod != http.MethodPut {
		t.Fatalf("expected PUT, got %s", gotMethod)
	}
}

func TestReplaceWithPayloadErrorsWhenDirMissing(t *testing.T) {
	destPath := filepath.Join(t.TempDir(), "missing", "apisix.yaml")
	if err := replaceWithPayload([]byte("payload"), destPath); err == nil {
		t.Fatalf("expected error when destination dir is missing")
	}
}

func TestReplaceWithPayloadRenameFailure(t *testing.T) {
	dir := t.TempDir()
	destPath := filepath.Join(dir, "conf")
	if err := os.MkdirAll(destPath, 0o755); err != nil {
		t.Fatalf("create dest dir: %v", err)
	}
	if err := replaceWithPayload([]byte("payload"), destPath); err == nil {
		t.Fatalf("expected error when renaming onto a directory")
	}
}

type countingTransport struct {
	calls int
}

func (t *countingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	t.calls++
	return nil, nil
}
