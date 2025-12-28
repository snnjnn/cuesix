package reloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Reloader replaces the live config and triggers APISIX reloads.
type Reloader struct {
	// ConfigPath points to the dynamic config file under the APISIX home folder.
	ConfigPath string
	// ReloadURL and ReloadMethod target the Admin API reload endpoint.
	ReloadURL    string
	ReloadMethod string
	// APIKey adds the X-API-KEY header when required by APISIX.
	APIKey string
	// HTTPClient overrides the default client when provided.
	HTTPClient *http.Client
	// Retry* fields control backoff behavior for reload attempts.
	RetryMax        int
	RetryInitial    time.Duration
	RetryMaxDelay   time.Duration
	RetryMultiplier float64
	Logger          *slog.Logger
}

// Apply writes the payload to ConfigPath and triggers the reload endpoint.
func (r *Reloader) Apply(ctx context.Context, payload []byte) error {
	logger := ensureLogger(r.Logger)
	if r.ConfigPath == "" {
		return errors.New("config path is required")
	}
	if r.ReloadURL == "" {
		return errors.New("reload URL is required")
	}
	if len(payload) == 0 {
		return errors.New("payload is required")
	}

	if err := replaceWithPayload(payload, r.ConfigPath); err != nil {
		logger.Error("replace config failed", "error", err)
		return err
	}
	if err := r.triggerReload(ctx); err != nil {
		logger.Error("reload request failed", "error", err)
		return err
	}
	logger.Info("reload request succeeded")
	return nil
}

func (r *Reloader) triggerReload(ctx context.Context) error {
	retries := r.RetryMax
	if retries < 0 {
		retries = 0
	}
	delay := r.RetryInitial
	if delay <= 0 {
		delay = 200 * time.Millisecond
	}
	maxDelay := r.RetryMaxDelay
	if maxDelay <= 0 {
		maxDelay = 2 * time.Second
	}
	multiplier := r.RetryMultiplier
	if multiplier <= 1 {
		multiplier = 2
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if attempt > 0 {
			sleep := delay
			if sleep > maxDelay {
				sleep = maxDelay
			}
			if err := sleepContext(ctx, sleep); err != nil {
				return err
			}
			delay = time.Duration(float64(delay) * multiplier)
		}

		if err := r.doReloadRequest(ctx); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	return lastErr
}

func (r *Reloader) doReloadRequest(ctx context.Context) error {
	method := r.ReloadMethod
	if method == "" {
		method = http.MethodPost
	}
	client := r.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, method, r.ReloadURL, nil)
	if err != nil {
		return err
	}
	if r.APIKey != "" {
		req.Header.Set("X-API-KEY", r.APIKey)
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("reload failed: status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func sleepContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func ensureLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}

func replaceWithPayload(payload []byte, destPath string) error {
	dir := filepath.Dir(destPath)
	tmp, err := os.CreateTemp(dir, ".cuesix-reload-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if info, err := os.Stat(destPath); err == nil {
		if err := os.Chmod(tmpPath, info.Mode()); err != nil {
			_ = os.Remove(tmpPath)
			return fmt.Errorf("chmod temp file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("stat config file: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
