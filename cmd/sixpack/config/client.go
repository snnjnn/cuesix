package config

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/urfave/cli/v3"
)

type Client struct {
	BaseURL              string
	ConnectTimeout       time.Duration
	ReadTimeout          time.Duration
	BackoffInitial       time.Duration
	BackoffMaxInterval   time.Duration
	BackoffMultiplier    float64
	BackoffRandomization float64
	BackoffMaxElapsed    time.Duration
	BackoffMaxRetries    int
}

// Flags returns SSE client command-line flags.
func (c *Client) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "client-url",
			Usage:    "base URL for a remote sixpack server with SSE enabled",
			Sources:  cli.EnvVars("SIXPACK_CLIENT_URL"),
			Value:    "http://127.0.0.1:8080",
			Category: "Client",
		},
		&cli.DurationFlag{
			Name:     "client-connect-timeout",
			Usage:    "timeout for TCP/TLS connect and response headers",
			Sources:  cli.EnvVars("SIXPACK_CLIENT_CONNECT_TIMEOUT"),
			Value:    5 * time.Second,
			Category: "Client",
		},
		&cli.DurationFlag{
			Name:     "client-read-timeout",
			Usage:    "maximum silence period before reconnecting the SSE stream",
			Sources:  cli.EnvVars("SIXPACK_CLIENT_READ_TIMEOUT"),
			Value:    30 * time.Second,
			Category: "Client",
		},
		&cli.DurationFlag{
			Name:     "client-backoff-initial",
			Usage:    "initial reconnect backoff interval",
			Sources:  cli.EnvVars("SIXPACK_CLIENT_BACKOFF_INITIAL"),
			Value:    1 * time.Second,
			Category: "Client",
		},
		&cli.DurationFlag{
			Name:     "client-backoff-max-interval",
			Usage:    "maximum reconnect backoff interval",
			Sources:  cli.EnvVars("SIXPACK_CLIENT_BACKOFF_MAX_INTERVAL"),
			Value:    10 * time.Second,
			Category: "Client",
		},
		&cli.Float64Flag{
			Name:     "client-backoff-multiplier",
			Usage:    "reconnect backoff multiplier",
			Sources:  cli.EnvVars("SIXPACK_CLIENT_BACKOFF_MULTIPLIER"),
			Value:    2.0,
			Category: "Client",
		},
		&cli.Float64Flag{
			Name:     "client-backoff-randomization",
			Usage:    "reconnect backoff randomization factor [0..1]",
			Sources:  cli.EnvVars("SIXPACK_CLIENT_BACKOFF_RANDOMIZATION"),
			Value:    0.5,
			Category: "Client",
		},
		&cli.DurationFlag{
			Name:     "client-backoff-max-elapsed",
			Usage:    "max total time spent retrying before resetting to 0 for infinite retries",
			Sources:  cli.EnvVars("SIXPACK_CLIENT_BACKOFF_MAX_ELAPSED"),
			Value:    0,
			Category: "Client",
		},
		&cli.IntFlag{
			Name:     "client-backoff-max-retries",
			Usage:    "max retries for each reconnect cycle (<=0 means unlimited)",
			Sources:  cli.EnvVars("SIXPACK_CLIENT_BACKOFF_MAX_RETRIES"),
			Value:    0,
			Category: "Client",
		},
	}
}

// Apply loads client settings from parsed CLI flags.
func (c *Client) Apply(ctx *cli.Command) {
	c.BaseURL = ctx.String("client-url")
	c.ConnectTimeout = ctx.Duration("client-connect-timeout")
	c.ReadTimeout = ctx.Duration("client-read-timeout")
	c.BackoffInitial = ctx.Duration("client-backoff-initial")
	c.BackoffMaxInterval = ctx.Duration("client-backoff-max-interval")
	c.BackoffMultiplier = ctx.Float64("client-backoff-multiplier")
	c.BackoffRandomization = ctx.Float64("client-backoff-randomization")
	c.BackoffMaxElapsed = ctx.Duration("client-backoff-max-elapsed")
	c.BackoffMaxRetries = ctx.Int("client-backoff-max-retries")
}

// Validate verifies client settings.
func (c *Client) Validate() error {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("invalid client-url: %w", err)
	}
	if u == nil || strings.TrimSpace(u.Host) == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("client-url must be an absolute http(s) URL")
	}
	if c.ConnectTimeout <= 0 {
		return errors.New("client-connect-timeout must be > 0")
	}
	if c.ReadTimeout <= 0 {
		return errors.New("client-read-timeout must be > 0")
	}
	if c.BackoffInitial <= 0 {
		return errors.New("client-backoff-initial must be > 0")
	}
	if c.BackoffMaxInterval <= 0 {
		return errors.New("client-backoff-max-interval must be > 0")
	}
	if c.BackoffMultiplier <= 0 {
		return errors.New("client-backoff-multiplier must be > 0")
	}
	if c.BackoffRandomization < 0 || c.BackoffRandomization > 1 {
		return errors.New("client-backoff-randomization must be between 0 and 1")
	}
	if c.BackoffMaxElapsed < 0 {
		return errors.New("client-backoff-max-elapsed must be >= 0")
	}
	return nil
}

// BuildBackoffFactory creates a new backoff constructor for each client loop.
func (c *Client) BuildBackoffFactory() func() backoff.BackOff {
	return func() backoff.BackOff {
		bo := backoff.NewExponentialBackOff()
		bo.InitialInterval = c.BackoffInitial
		bo.MaxInterval = c.BackoffMaxInterval
		bo.Multiplier = c.BackoffMultiplier
		bo.RandomizationFactor = c.BackoffRandomization
		bo.MaxElapsedTime = c.BackoffMaxElapsed
		if c.BackoffMaxRetries > 0 {
			return backoff.WithMaxRetries(bo, uint64(c.BackoffMaxRetries))
		}
		return bo
	}
}
