package config

import (
	"errors"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/urfave/cli/v3"
)

type DeploymentMode string

const (
	InvalidMode     DeploymentMode = ""
	StandaloneMode  DeploymentMode = "standalone"
	TraditionalMode DeploymentMode = "traditional"
	DecoupledMode   DeploymentMode = "decoupled"
)

type APIControl struct {
	DeploymentMode  DeploymentMode
	ControlURL      string
	AdminURL        string
	APIKey          string
	Timeout         time.Duration
	RetryMax        int
	RetryInitial    time.Duration
	RetryMaxDelay   time.Duration
	RetryMultiplier float64
	RetryJitter     float64
}

// Flags returns APISIX Control command-line flags.
func (c *APIControl) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:     "apisix-deployment-mode",
			Usage:    "apisix operation mode (standalone / traditional / decoupled)",
			Sources:  cli.EnvVars("SIXPACK_APISIX_DEPLOYMENT_MODE"),
			Value:    "standalone",
			Category: "APISIX Control",
		},
		&cli.StringFlag{
			Name:     "apisix-control-url",
			Usage:    "apisix control API base url",
			Sources:  cli.EnvVars("SIXPACK_APISIX_CONTROL_URL"),
			Value:    "http://127.0.0.1:9090",
			Category: "APISIX Control",
		},
		&cli.StringFlag{
			Name:     "apisix-admin-url",
			Usage:    "apisix admin API base url",
			Sources:  cli.EnvVars("SIXPACK_APISIX_ADMIN_URL"),
			Value:    "http://127.0.0.1:9091",
			Category: "APISIX Control",
		},
		&cli.StringFlag{
			Name:     "apisix-api-key",
			Usage:    "apisix control API key",
			Sources:  cli.EnvVars("SIXPACK_APISIX_API_KEY"),
			Category: "APISIX Control",
		},
		&cli.DurationFlag{
			Name:     "apisix-api-timeout",
			Usage:    "timeout for apisix control API requests",
			Sources:  cli.EnvVars("SIXPACK_APISIX_API_TIMEOUT"),
			Value:    10 * time.Second,
			Category: "APISIX Control",
		},
		&cli.IntFlag{
			Name:     "retry-max",
			Usage:    "API request retry attempts",
			Sources:  cli.EnvVars("SIXPACK_RETRY_MAX"),
			Value:    0,
			Category: "APISIX Control",
		},
		&cli.DurationFlag{
			Name:     "retry-initial",
			Usage:    "API request initial backoff",
			Sources:  cli.EnvVars("SIXPACK_RETRY_INITIAL"),
			Value:    200 * time.Millisecond,
			Category: "APISIX Control",
		},
		&cli.DurationFlag{
			Name:     "retry-max-delay",
			Usage:    "API request max backoff",
			Sources:  cli.EnvVars("SIXPACK_RETRY_MAX_DELAY"),
			Value:    2 * time.Second,
			Category: "APISIX Control",
		},
		&cli.Float64Flag{
			Name:     "retry-multiplier",
			Usage:    "API request backoff multiplier",
			Sources:  cli.EnvVars("SIXPACK_RETRY_MULTIPLIER"),
			Value:    2,
			Category: "APISIX Control",
		},
		&cli.Float64Flag{
			Name:     "retry-jitter",
			Usage:    "API request backoff jitter factor [0..1]",
			Sources:  cli.EnvVars("SIXPACK_RETRY_JITTER"),
			Value:    0.5,
			Category: "APISIX Control",
		},
	}
}

// Apply loads APISIX Control values from parsed CLI flags.
func (c *APIControl) Apply(ctx *cli.Command) {
	switch strings.ToLower(strings.TrimSpace(ctx.String("apisix-deployment-mode"))) {
	case "standalone":
		c.DeploymentMode = StandaloneMode
	case "traditional":
		c.DeploymentMode = TraditionalMode
	case "decoupled":
		c.DeploymentMode = DecoupledMode
	default:
		c.DeploymentMode = InvalidMode
	}
	c.ControlURL = ctx.String("apisix-control-url")
	c.AdminURL = ctx.String("apisix-admin-url")
	c.APIKey = ctx.String("apisix-api-key")
	c.Timeout = ctx.Duration("apisix-api-timeout")
	c.RetryMax = ctx.Int("retry-max")
	c.RetryInitial = ctx.Duration("retry-initial")
	c.RetryMaxDelay = ctx.Duration("retry-max-delay")
	c.RetryMultiplier = ctx.Float64("retry-multiplier")
	c.RetryJitter = ctx.Float64("retry-jitter")
}

// Validate input flags values
func (c *APIControl) Validate() error {
	if c.RetryJitter < 0 || c.RetryJitter > 1 {
		return errors.New("retry-jitter must be between 0 and 1")
	}
	if c.DeploymentMode == InvalidMode {
		return errors.New("deployment mode must be one of: standalone, traditional, decoupled")
	}
	if c.DeploymentMode != StandaloneMode {
		if c.AdminURL == "" {
			return errors.New("admin URL is required")
		}
		if c.APIKey == "" {
			return errors.New("API key is required")
		}
	}
	return nil
}

// BuildBackoff creates a backoff.BackOff from the retry configuration.
func (c *APIControl) BuildBackoff(forever bool) backoff.BackOff {
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
	if c.RetryJitter >= 0 && c.RetryJitter <= 1 {
		bo.RandomizationFactor = c.RetryJitter
	}
	if forever || c.RetryMax <= 0 {
		bo.MaxElapsedTime = 0
		return bo
	}
	return backoff.WithMaxRetries(bo, uint64(c.RetryMax))
}
