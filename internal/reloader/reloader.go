package reloader

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/cenkalti/backoff/v4"
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
	// Backoff controls retry behavior for reload attempts.
	Backoff backoff.BackOff
	// RequestTimeout caps the reload HTTP request duration.
	RequestTimeout time.Duration
	// Logger for this reloader instance.
	Logger *slog.Logger
}

// Apply writes the payload to ConfigPath and triggers the reload endpoint.
func (r *Reloader) Apply(ctx context.Context, payload []byte, useApi bool) error {
	if r == nil {
		return errors.New("reloader is nil")
	}
	logger := r.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if r.ConfigPath == "" {
		return errors.New("config path is required")
	}
	if len(payload) == 0 {
		return errors.New("payload is required")
	}

	if err := replaceWithPayload(payload, r.ConfigPath); err != nil {
		logger.Error("replace config failed", "error", err)
		return err
	}
	if r.ReloadURL == "" {
		logger.Info("reload request skipped by configuration")
		return nil
	}
	if !useApi {
		logger.Info("reload request skipped by flag")
		return nil
	}
	if err := r.triggerReload(ctx); err != nil {
		logger.Error("reload request failed", "error", err)
		return err
	}
	logger.Info("reload request succeeded")
	return nil
}

func (r *Reloader) triggerReload(ctx context.Context) error {
	if r == nil {
		return errors.New("reloader is nil")
	}
	bo := r.Backoff
	if bo == nil {
		bo = backoff.NewExponentialBackOff()
	}
	return backoff.Retry(func() error {
		return r.doReloadRequest(ctx)
	}, backoff.WithContext(bo, ctx))
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
	if r.RequestTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.RequestTimeout)
		defer cancel()
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
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	var reader bytes.Buffer
	var body string
	if _, err := io.CopyN(&reader, resp.Body, 1024); err != nil && err != io.EOF {
		body = fmt.Sprintf("read response body: %v", err)
	} else {
		body = reader.String()
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("reload failed: status %d: %s", resp.StatusCode, body)
	}
	return nil
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
			closeErr := os.Remove(tmpPath)
			if closeErr != nil {
				return fmt.Errorf("chmod temp file: %w (cleanup failed: %v)", err, closeErr)
			}
			return fmt.Errorf("chmod temp file: %w", err)
		}
	} else if !os.IsNotExist(err) {
		closeErr := os.Remove(tmpPath)
		if closeErr != nil {
			return fmt.Errorf("stat config file: %w (cleanup failed: %v)", err, closeErr)
		}
		return fmt.Errorf("stat config file: %w", err)
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
