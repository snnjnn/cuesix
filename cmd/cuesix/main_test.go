package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/internal/plugin"
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

func TestBuildPostRender(t *testing.T) {
	post, err := buildPostRender(true, true, 0)
	if err != nil {
		t.Fatalf("buildPostRender returned error: %v", err)
	}
	chain, ok := post.(plugin.PostRenderChain)
	if !ok {
		t.Fatalf("expected PostRenderChain, got %T", post)
	}
	if len(chain) != 2 {
		t.Fatalf("expected 2 plugins, got %d", len(chain))
	}
	if _, ok := chain[0].(*plugin.JQPlugin); !ok {
		t.Fatalf("expected first plugin to be JQPlugin, got %T", chain[0])
	}
	if _, ok := chain[1].(*plugin.YAMLPlugin); !ok {
		t.Fatalf("expected second plugin to be YAMLPlugin, got %T", chain[1])
	}
}

func TestBuildPostRenderEmpty(t *testing.T) {
	post, err := buildPostRender(false, false, 0)
	if err != nil {
		t.Fatalf("buildPostRender returned error: %v", err)
	}
	chain, ok := post.(plugin.PostRenderChain)
	if !ok {
		t.Fatalf("expected PostRenderChain, got %T", post)
	}
	if len(chain) != 0 {
		t.Fatalf("expected empty chain, got %d", len(chain))
	}
}

func TestDrainBody(t *testing.T) {
	body := io.NopCloser(bytes.NewBufferString("hello"))
	req := httptest.NewRequest(http.MethodPost, "/", body)
	closed := false
	req.Body = &trackBody{ReadCloser: req.Body, closed: &closed}

	handled := false
	handler := drainBody(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handled = true
		if !*r.Body.(*trackBody).closed {
			t.Fatalf("expected body to be closed")
		}
	}))
	handler.ServeHTTP(httptest.NewRecorder(), req)

	if !handled {
		t.Fatalf("expected handler to be called")
	}
}

func TestBuildReloadURL(t *testing.T) {
	got, err := buildReloadURL("http://127.0.0.1:9180")
	if err != nil {
		t.Fatalf("buildReloadURL returned error: %v", err)
	}
	want := "http://127.0.0.1:9180/apisix/admin/configs?reload=true"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}

	got, err = buildReloadURL("http://127.0.0.1:9180/base?x=1")
	if err != nil {
		t.Fatalf("buildReloadURL returned error: %v", err)
	}
	want = "http://127.0.0.1:9180/apisix/admin/configs?reload=true&x=1"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

type trackBody struct {
	io.ReadCloser
	closed *bool
}

func (t *trackBody) Close() error {
	*t.closed = true
	return t.ReadCloser.Close()
}
