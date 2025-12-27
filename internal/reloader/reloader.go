package reloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"log/slog"
	"time"
)

type Reloader struct {
	ConfigPath   string
	ReloadURL    string
	ReloadMethod string
	APIKey       string
	HTTPClient   *http.Client
	RetryMax        int
	RetryInitial    time.Duration
	RetryMaxDelay   time.Duration
	RetryMultiplier float64
	Logger         *slog.Logger
}

func (r *Reloader) Apply(ctx context.Context, tempPath string) error {
	logger := ensureLogger(r.Logger)
	if r.ConfigPath == "" {
		return errors.New("config path is required")
	}
	if r.ReloadURL == "" {
		return errors.New("reload URL is required")
	}
	if tempPath == "" {
		return errors.New("temp path is required")
	}

	if err := replaceFile(tempPath, r.ConfigPath); err != nil {
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

func replaceFile(srcPath, destPath string) error {
	if err := os.Rename(srcPath, destPath); err == nil {
		return nil
	} else if !isCrossDevice(err) {
		return fmt.Errorf("replace config: %w", err)
	}

	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("stat temp file: %w", err)
	}

	destDir := filepath.Dir(destPath)
	tmp, err := os.CreateTemp(destDir, ".cuesix-reload-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := copyFileContents(tmp, srcPath); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(info.Mode()); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("chmod temp file: %w", err)
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
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}

func copyFileContents(dst *os.File, srcPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return fmt.Errorf("open temp file: %w", err)
	}
	defer src.Close()
	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("copy temp file: %w", err)
	}
	return nil
}

func isCrossDevice(err error) bool {
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) && linkErr.Err == syscall.EXDEV {
		return true
	}
	return false
}
