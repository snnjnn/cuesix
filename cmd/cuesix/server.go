package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// buildServer creates an HTTP server with standard timeouts.
func buildServer(listenAddr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              listenAddr,
		Handler:           drainBody(handler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
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

// buildReloadURL builds the APISIX reload URL from the base admin URL.
func buildReloadURL(base string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return "", err
	}
	parsed.Path = "/apisix/admin/configs"
	query := parsed.Query()
	query.Set("reload", "true")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}
