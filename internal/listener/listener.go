package listener

import (
	"errors"
	"net/http"
)

// Notifier is notified when /compile is requested.
type Notifier interface {
	Notify()
	Ready() bool
}

// NewHandler builds the HTTP handler that exposes POST /compile.
func NewHandler(notifier Notifier) (http.Handler, error) {
	if notifier == nil {
		return nil, errors.New("notifier is required")
	}
	mux := http.NewServeMux()
	mux.Handle("GET /live", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	mux.Handle("GET /ready", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if notifier.Ready() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusTooEarly)
	}))
	mux.Handle("POST /compile", compileHandler(notifier))
	return mux, nil
}

func compileHandler(notifier Notifier) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		notifier.Notify()
		w.WriteHeader(http.StatusNoContent)
	})
}
