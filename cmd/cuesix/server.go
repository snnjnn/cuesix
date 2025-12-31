package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/warpcomdev/cuesix/cmd/cuesix/config"
)

// buildServer creates an HTTP server with standard timeouts.
func buildServer(listenAddr string, handler http.Handler, timeouts config.Timeouts) *http.Server {
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
		if r.Body != nil {
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
		}
		next.ServeHTTP(w, r)
	})
}

// serverShutdown returns a shutdown handler bound to a context.
func serverShutdown(ctx context.Context, logger *slog.Logger, name string, server *http.Server, timeout time.Duration) func() error {
	return func() error {
		<-ctx.Done()
		cancelCtx, cancelFunc := context.WithTimeout(context.Background(), timeout)
		defer cancelFunc()
		if err := server.Shutdown(cancelCtx); err != nil {
			logger.Error("server shutdown error", "server", name, "error", err)
		}
		return nil
	}
}
