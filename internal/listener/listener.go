package listener

import (
	"errors"
	"net/http"
)

type Notifier interface {
	Notify()
}

func NewHandler(notifier Notifier) (http.Handler, error) {
	if notifier == nil {
		return nil, errors.New("notifier is required")
	}
	mux := http.NewServeMux()
	mux.Handle("/compile", compileHandler(notifier))
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
