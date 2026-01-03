package config

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/urfave/cli/v3"
	"github.com/warpcomdev/cuesix/internal/dispatcher"
	"github.com/warpcomdev/cuesix/internal/reloader"
)

type Reload struct {
	URL             string
	DryRun          bool
	APIKey          string
	Method          string
	Timeout         time.Duration
	RetryMax        int
	RetryInitial    time.Duration
	RetryMaxDelay   time.Duration
	RetryMultiplier float64
}

func (c *Reload) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "apisix-url",
			Usage:    "apisix admin base url",
			Sources:  cli.EnvVars("CUESIX_APISIX_URL"),
			Category: "Reload",
		},
		&cli.BoolFlag{
			Name:     "dry-run",
			Usage:    "run pipeline without writing config or reloading apisix",
			Sources:  cli.EnvVars("CUESIX_DRY_RUN"),
			Category: "Reload",
		},
		&cli.StringFlag{
			Name:     "apisix-api-key",
			Usage:    "apisix admin api key",
			Sources:  cli.EnvVars("CUESIX_APISIX_API_KEY"),
			Category: "Reload",
		},
		&cli.StringFlag{
			Name:     "reload-method",
			Usage:    "reload HTTP method",
			Sources:  cli.EnvVars("CUESIX_RELOAD_METHOD"),
			Value:    http.MethodPost,
			Category: "Reload",
		},
		&cli.DurationFlag{
			Name:     "reload-timeout",
			Usage:    "timeout for reload HTTP request",
			Sources:  cli.EnvVars("CUESIX_RELOAD_TIMEOUT"),
			Value:    10 * time.Second,
			Category: "Reload",
		},
		&cli.IntFlag{
			Name:     "retry-max",
			Usage:    "reload retry attempts",
			Sources:  cli.EnvVars("CUESIX_RETRY_MAX"),
			Value:    0,
			Category: "Reload",
		},
		&cli.DurationFlag{
			Name:     "retry-initial",
			Usage:    "reload initial backoff",
			Sources:  cli.EnvVars("CUESIX_RETRY_INITIAL"),
			Value:    200 * time.Millisecond,
			Category: "Reload",
		},
		&cli.DurationFlag{
			Name:     "retry-max-delay",
			Usage:    "reload max backoff",
			Sources:  cli.EnvVars("CUESIX_RETRY_MAX_DELAY"),
			Value:    2 * time.Second,
			Category: "Reload",
		},
		&cli.Float64Flag{
			Name:     "retry-multiplier",
			Usage:    "reload backoff multiplier",
			Sources:  cli.EnvVars("CUESIX_RETRY_MULTIPLIER"),
			Value:    2,
			Category: "Reload",
		},
	}
}

func (c *Reload) Apply(ctx *cli.Command) {
	c.URL = ctx.String("apisix-url")
	c.DryRun = ctx.Bool("dry-run")
	c.APIKey = ctx.String("apisix-api-key")
	c.Method = ctx.String("reload-method")
	c.Timeout = ctx.Duration("reload-timeout")
	c.RetryMax = ctx.Int("retry-max")
	c.RetryInitial = ctx.Duration("retry-initial")
	c.RetryMaxDelay = ctx.Duration("retry-max-delay")
	c.RetryMultiplier = ctx.Float64("retry-multiplier")
}

func (reloadCfg Reload) BuildReloader(logger *slog.Logger, apisixCfg APISIX, pluginCfg Plugins) (dispatcher.Reloader, error) {
	// Wire reloader (or dry-run).
	configPath := apisixCfg.ConfigPath(pluginCfg.EnableYAML)
	var reloadTarget dispatcher.Reloader
	if reloadCfg.DryRun {
		reloadTarget = &dryRunReloader{}
	} else {
		// Reload target configuration.
		reloadURL, err := reloadCfg.buildURL()
		if err != nil {
			return reloadTarget, fmt.Errorf("invalid apisix url: %w", err)
		}
		reloadTarget = &reloader.Reloader{
			ConfigPath:     configPath,
			ReloadURL:      reloadURL,
			ReloadMethod:   reloadCfg.Method,
			APIKey:         reloadCfg.APIKey,
			Backoff:        reloadCfg.buildBackoff(),
			RequestTimeout: reloadCfg.Timeout,
			Logger:         logger,
		}
	}
	return reloadTarget, nil
}

func (c Reload) buildURL() (string, error) {
	if strings.TrimSpace(c.URL) == "" {
		return "", nil
	}
	parsed, err := url.Parse(strings.TrimSpace(c.URL))
	if err != nil {
		return "", err
	}
	parsed.Path = "/apisix/admin/configs"
	query := parsed.Query()
	query.Set("reload", "true")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// buildBackoff creates a backoff.BackOff from the retry configuration.
func (c Reload) buildBackoff() backoff.BackOff {
	bo := backoff.NewExponentialBackOff()
	if c.RetryInitial > 0 {
		bo.InitialInterval = c.RetryInitial
	}
	if c.RetryMaxDelay > 0 {
		bo.MaxInterval = c.RetryMaxDelay
	}
	if c.RetryMultiplier > 1 {
		bo.Multiplier = c.RetryMultiplier
	}
	if c.RetryMax >= 0 {
		return backoff.WithMaxRetries(bo, uint64(c.RetryMax))
	}
	return bo
}

// dryRunReloader is a no-op reloader used for dry-run mode.
type dryRunReloader struct{}

// Apply logs the payload size without making changes.
func (r *dryRunReloader) Apply(_ context.Context, payload []byte, useApi bool) error {
	return nil
}
