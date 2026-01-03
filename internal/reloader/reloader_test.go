package reloader_test

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/warpcomdev/cuesix/internal/reloader"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

type roundTripResp struct {
	status int
	body   string
	err    error
}

type mockRoundTripper struct {
	responses []roundTripResp
	calls     []*http.Request
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	m.calls = append(m.calls, req)
	idx := len(m.calls) - 1
	if idx >= len(m.responses) {
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	}
	resp := m.responses[idx]
	if resp.err != nil {
		return nil, resp.err
	}
	return &http.Response{
		StatusCode: resp.status,
		Body:       io.NopCloser(strings.NewReader(resp.body)),
	}, nil
}

func TestApplyWritesAndReloads(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	rt := &mockRoundTripper{responses: []roundTripResp{{status: http.StatusOK}}}
	client := &http.Client{Transport: rt}

	r := &reloader.Reloader{
		ConfigPath:     configPath,
		ReloadURL:      "http://example/reload",
		ReloadMethod:   http.MethodPut,
		APIKey:         "secret",
		RequestTimeout: time.Second,
		Logger:         testutil.Logger(),
		HTTPClient:     client,
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
	if len(rt.calls) != 1 {
		t.Fatalf("expected one reload request, got %d", len(rt.calls))
	}
	if rt.calls[0].Header.Get("X-API-KEY") != "secret" {
		t.Fatalf("expected API key header")
	}
}

func TestApplySkipsWhenConfigured(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")
	r := &reloader.Reloader{
		ConfigPath: configPath,
		Logger:     testutil.Logger(),
	}
	if err := r.Apply(context.Background(), []byte("payload"), false); err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected payload written even when reload skipped: %v", err)
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

func TestTriggerReloadRetries(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "cfg")
	if err := os.WriteFile(cfgPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write cfg: %v", err)
	}
	rt := &mockRoundTripper{
		responses: []roundTripResp{
			{status: http.StatusBadGateway, body: "bad"},
			{status: http.StatusOK},
		},
	}
	client := &http.Client{Transport: rt}
	r := &reloader.Reloader{
		ReloadURL:    "http://example/reload",
		ConfigPath:   cfgPath,
		ReloadMethod: http.MethodPost,
		Backoff:      backoff.NewConstantBackOff(0),
		Logger:       testutil.Logger(),
		HTTPClient:   client,
	}
	if err := r.Apply(context.Background(), []byte("x"), true); err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if len(rt.calls) != 2 {
		t.Fatalf("expected retry, got %d attempts", len(rt.calls))
	}
}

func TestTriggerReloadFailure(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "cfg")
	if err := os.WriteFile(cfgPath, []byte("x"), 0o644); err != nil {
		t.Fatalf("write cfg: %v", err)
	}
	rt := &mockRoundTripper{
		responses: []roundTripResp{
			{status: http.StatusServiceUnavailable, body: "nope"},
			{status: http.StatusServiceUnavailable, body: "nope"},
		},
	}
	client := &http.Client{Transport: rt}
	r := &reloader.Reloader{
		ReloadURL:    "http://example/reload",
		ReloadMethod: http.MethodPost,
		ConfigPath:   cfgPath,
		Backoff:      backoff.WithMaxRetries(backoff.NewConstantBackOff(0), 1),
		Logger:       testutil.Logger(),
		HTTPClient:   client,
	}
	err := r.Apply(context.Background(), []byte("x"), true)
	if err == nil {
		t.Fatalf("expected reload failure")
	}
}
