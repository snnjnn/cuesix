package listener_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/warpcondev/cuesix/internal/listener"
)

type recordingNotifier struct {
	calls int
}

func (n *recordingNotifier) Notify() {
	n.calls++
}

type staticPublisher struct {
	ready bool
}

func (p staticPublisher) Ready() bool {
	return p.ready
}

func TestNewHandlerValidation(t *testing.T) {
	t.Parallel()

	if _, err := listener.NewHandler(nil, nil); err == nil {
		t.Fatalf("expected error when notifier and publisher are nil")
	}
}

func TestNewHandlerCompileEndpoint(t *testing.T) {
	t.Parallel()

	notifier := &recordingNotifier{}
	mux, err := listener.NewHandler(notifier, nil)
	if err != nil {
		t.Fatalf("NewHandler() error = %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/compile", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
	if notifier.calls != 1 {
		t.Fatalf("Notify() calls = %d", notifier.calls)
	}
}

func TestNewHandlerPublisherEndpoints(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ready  bool
		path   string
		method string
		want   int
	}{
		{name: "ready true", ready: true, path: "/ready", method: http.MethodGet, want: http.StatusOK},
		{name: "ready false", ready: false, path: "/ready", method: http.MethodGet, want: http.StatusTooEarly},
		{name: "live", ready: false, path: "/live", method: http.MethodGet, want: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mux, err := listener.NewHandler(nil, staticPublisher{ready: tt.ready})
			if err != nil {
				t.Fatalf("NewHandler() error = %v", err)
			}

			req := httptest.NewRequest(tt.method, tt.path, nil)
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, req)

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestNewHandlerRegistersOnlyProvidedDependencies(t *testing.T) {
	t.Parallel()

	t.Run("notifier only", func(t *testing.T) {
		mux, err := listener.NewHandler(&recordingNotifier{}, nil)
		if err != nil {
			t.Fatalf("NewHandler() error = %v", err)
		}

		req := httptest.NewRequest(http.MethodGet, "/ready", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d", rec.Code)
		}
	})

	t.Run("publisher only", func(t *testing.T) {
		mux, err := listener.NewHandler(nil, staticPublisher{ready: true})
		if err != nil {
			t.Fatalf("NewHandler() error = %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/compile", nil)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d", rec.Code)
		}
	})
}
