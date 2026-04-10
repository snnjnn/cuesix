package app_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/warpcomdev/cuesix/internal/app"
)

type errReadCloser struct {
	err error
}

func (e errReadCloser) Read([]byte) (int, error) {
	return 0, e.err
}

func (e errReadCloser) Close() error {
	return nil
}

func TestEchoHandlerMethodNotAllowed(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPut, "/echo", nil)
	rec := httptest.NewRecorder()

	app.EchoHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Allow"); got != "GET, POST" {
		t.Fatalf("Allow = %q", got)
	}
	if !strings.Contains(rec.Body.String(), "method not allowed") {
		t.Fatalf("body = %q", rec.Body.String())
	}
}

func TestEchoHandlerRendersRequestData(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/echo?x=1", strings.NewReader("payload"))
	req.Header.Set("Z-Last", "z")
	req.Header.Set("A-First", "a")
	rec := httptest.NewRecorder()

	app.EchoHandler().ServeHTTP(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "text/html") {
		t.Fatalf("Content-Type = %q", got)
	}
	if !strings.Contains(body, "POST") {
		t.Fatalf("body missing method: %q", body)
	}
	if !strings.Contains(body, "/echo?x=1") {
		t.Fatalf("body missing path: %q", body)
	}
	if !strings.Contains(body, "payload") {
		t.Fatalf("body missing payload: %q", body)
	}
	if !strings.Contains(body, "A-First") || !strings.Contains(body, "Z-Last") {
		t.Fatalf("body missing headers: %q", body)
	}
	if strings.Index(body, "A-First") > strings.Index(body, "Z-Last") {
		t.Fatalf("headers not rendered in sorted order: %q", body)
	}
}

func TestEchoHandlerTruncatesOversizedBody(t *testing.T) {
	t.Parallel()

	oversized := strings.Repeat("a", 70*1024)
	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(oversized))
	rec := httptest.NewRecorder()

	app.EchoHandler().ServeHTTP(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(body, "Body truncated to 64KB.") {
		t.Fatalf("expected truncation notice, body=%q", body)
	}
	if !strings.Contains(body, strings.Repeat("a", 128)) {
		t.Fatalf("expected body preview content, body=%q", body)
	}
}

func TestEchoHandlerSurfacesBodyReadError(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodPost, "/echo", nil)
	req.Body = errReadCloser{err: errors.New("boom")}
	rec := httptest.NewRecorder()

	app.EchoHandler().ServeHTTP(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(body, "boom") {
		t.Fatalf("expected body error in response, body=%q", body)
	}
}

func TestEchoHandlerTruncatesLongHeaderValues(t *testing.T) {
	t.Parallel()

	longValue := strings.Repeat("x", 120)
	req := httptest.NewRequest(http.MethodGet, "/echo", nil)
	req.Header.Set("X-Long", longValue)
	rec := httptest.NewRecorder()

	app.EchoHandler().ServeHTTP(rec, req)

	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(body, "X-Long") {
		t.Fatalf("missing header name in body=%q", body)
	}
	if !strings.Contains(body, "…") {
		t.Fatalf("expected truncated header indicator in body=%q", body)
	}
	if !strings.Contains(body, `data-full="`+longValue+`"`) {
		t.Fatalf("expected full header value to remain available for copy, body=%q", body)
	}
}

func TestEchoHandlerNilBody(t *testing.T) {
	t.Parallel()

	req := httptest.NewRequest(http.MethodGet, "/echo", nil)
	req.Body = nil
	rec := httptest.NewRecorder()

	app.EchoHandler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if body := rec.Body.String(); strings.Contains(body, "Request body truncated") {
		t.Fatalf("unexpected truncation notice in body=%q", body)
	}
}

var _ io.ReadCloser = errReadCloser{}
