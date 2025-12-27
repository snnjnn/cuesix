package listener

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type stubNotifier struct {
	calls int
}

func (s *stubNotifier) Notify() {
	s.calls++
}

func TestHandlerCompile(t *testing.T) {
	notifier := &stubNotifier{}
	handler, err := NewHandler(notifier)
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/compile", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if notifier.calls != 1 {
		t.Fatalf("expected Notify to be called once, got %d", notifier.calls)
	}
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	notifier := &stubNotifier{}
	handler, err := NewHandler(notifier)
	if err != nil {
		t.Fatalf("NewHandler returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/compile", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if notifier.calls != 0 {
		t.Fatalf("expected Notify not to be called")
	}
}
