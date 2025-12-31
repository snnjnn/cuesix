package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func TestServerShutdownTriggersOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	httpServer := &http.Server{}

	done := make(chan struct{}, 1)
	go func() {
		_ = serverShutdown(ctx, logger, "test", httpServer, time.Second)()
		done <- struct{}{}
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected shutdown handler to return")
	}
}
