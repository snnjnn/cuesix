package listener

import (
	"errors"
	"net/http"
)

// Publisher exposes readiness status
type Publisher interface {
	Ready() bool
}

// Notifier is notified when /compile is requested.
type Notifier interface {
	Notify()
}

func NewHandler(notifier Notifier, publisher Publisher) (*http.ServeMux, error) {
	mux := http.NewServeMux()
	if notifier == nil && publisher == nil {
		return nil, errors.New("notifier or publisher are required")
	}
	if notifier != nil {
		registerNotifier(mux, notifier)
	}
	if publisher != nil {
		registerPublisher(mux, publisher)
	}
	return mux, nil
}

// registerNotifier builds the HTTP handler that exposes POST /compile.
func registerNotifier(mux *http.ServeMux, notifier Notifier) {
	mux.Handle("POST /compile", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		notifier.Notify()
		w.WriteHeader(http.StatusNoContent)
	}))
}

// registerPublisher builds the HTTP handler that exposes GET /live and GET /ready.
func registerPublisher(mux *http.ServeMux, notifier Publisher) {
	mux.Handle("GET /ready", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if notifier.Ready() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusTooEarly)
	}))
	mux.Handle("GET /live", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}
