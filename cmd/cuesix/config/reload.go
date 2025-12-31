package config

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
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

func (c Reload) BuildURL() (string, error) {
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
