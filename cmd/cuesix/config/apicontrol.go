package config

import (
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/urfave/cli/v3"
)

type APIControl struct {
	URL             string
	APIKey          string
	Timeout         time.Duration
	RetryMax        int
	RetryInitial    time.Duration
	RetryMaxDelay   time.Duration
	RetryMultiplier float64
}

func (c *APIControl) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "apisix-control-url",
			Usage:    "apisix control API base url",
			Sources:  cli.EnvVars("CUESIX_APISIX_CONTROL_URL"),
			Value:    "http://127.0.0.1:9090",
			Category: "APISIX Control",
		},
		&cli.StringFlag{
			Name:     "apisix-api-key",
			Usage:    "apisix control API key",
			Sources:  cli.EnvVars("CUESIX_APISIX_API_KEY"),
			Category: "APISIX Control",
		},
		&cli.DurationFlag{
			Name:     "apisix-api-timeout",
			Usage:    "timeout for apisix control API requests",
			Sources:  cli.EnvVars("CUESIX_APISIX_API_TIMEOUT"),
			Value:    10 * time.Second,
			Category: "APISIX Control",
		},
		&cli.IntFlag{
			Name:     "retry-max",
			Usage:    "API request retry attempts",
			Sources:  cli.EnvVars("CUESIX_RETRY_MAX"),
			Value:    0,
			Category: "APISIX Control",
		},
		&cli.DurationFlag{
			Name:     "retry-initial",
			Usage:    "API request initial backoff",
			Sources:  cli.EnvVars("CUESIX_RETRY_INITIAL"),
			Value:    200 * time.Millisecond,
			Category: "APISIX Control",
		},
		&cli.DurationFlag{
			Name:     "retry-max-delay",
			Usage:    "API request max backoff",
			Sources:  cli.EnvVars("CUESIX_RETRY_MAX_DELAY"),
			Value:    2 * time.Second,
			Category: "APISIX Control",
		},
		&cli.Float64Flag{
			Name:     "retry-multiplier",
			Usage:    "API request backoff multiplier",
			Sources:  cli.EnvVars("CUESIX_RETRY_MULTIPLIER"),
			Value:    2,
			Category: "APISIX Control",
		},
	}
}

func (c *APIControl) Apply(ctx *cli.Command) {
	c.URL = ctx.String("apisix-control-url")
	c.APIKey = ctx.String("apisix-api-key")
	c.Timeout = ctx.Duration("apisix-api-timeout")
	c.RetryMax = ctx.Int("retry-max")
	c.RetryInitial = ctx.Duration("retry-initial")
	c.RetryMaxDelay = ctx.Duration("retry-max-delay")
	c.RetryMultiplier = ctx.Float64("retry-multiplier")
}

// BuildBackoff creates a backoff.BackOff from the retry configuration.
func (c APIControl) BuildBackoff(forever bool) backoff.BackOff {
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
	if forever || c.RetryMax <= 0 {
		bo.MaxElapsedTime = 0
		return bo
	}
	return backoff.WithMaxRetries(bo, uint64(c.RetryMax))
}
