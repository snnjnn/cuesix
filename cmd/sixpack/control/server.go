package control

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/warpcomdev/cuesix/cmd/sixpack/config"
)

// BuildServer creates an HTTP server with standard timeouts.
func BuildServer(listenAddr string, handler http.Handler, timeouts config.Timeouts) *http.Server {
	return &http.Server{
		Addr:              listenAddr,
		Handler:           drainBody(handler),
		ReadHeaderTimeout: timeouts.ReadHeaderTimeout,
		ReadTimeout:       timeouts.ReadTimeout,
		WriteTimeout:      timeouts.WriteTimeout,
		IdleTimeout:       timeouts.IdleTimeout,
	}
}

// drainBody ensures request bodies are fully read and closed.
func drainBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Defer this function! do not call it before next(),
		// otherwise it will drain the body before the handler
		// can read it.
		defer func() {
			if r.Body != nil {
				io.Copy(io.Discard, r.Body)
				r.Body.Close()
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// ServerShutdown returns a shutdown handler bound to a context.
func ServerShutdown(ctx context.Context, server *http.Server, timeout time.Duration) func() error {
	return func() error {
		<-ctx.Done()
		cancelCtx, cancelFunc := context.WithTimeout(context.Background(), timeout)
		defer cancelFunc()
		return server.Shutdown(cancelCtx)
	}
}
