package config

import (
	"flag"
	"net/http"
	"net/url"
	"strings"
	"time"
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

func (c *Reload) RegisterFlags(fs *flag.FlagSet) {
	fs.StringVar(&c.URL, "apisix-url", envString("CUESIX_APISIX_URL"), "apisix admin base url")
	fs.BoolVar(&c.DryRun, "dry-run", envBool("CUESIX_DRY_RUN", false), "run pipeline without writing config or reloading apisix")
	fs.StringVar(&c.APIKey, "apisix-api-key", envString("CUESIX_APISIX_API_KEY"), "apisix admin api key")
	fs.StringVar(&c.Method, "reload-method", envStringDefault("CUESIX_RELOAD_METHOD", http.MethodPost), "reload HTTP method")
	fs.DurationVar(&c.Timeout, "reload-timeout", envDuration("CUESIX_RELOAD_TIMEOUT", 10*time.Second), "timeout for reload HTTP request")
	fs.IntVar(&c.RetryMax, "retry-max", envInt("CUESIX_RETRY_MAX", 0), "reload retry attempts")
	fs.DurationVar(&c.RetryInitial, "retry-initial", envDuration("CUESIX_RETRY_INITIAL", 200*time.Millisecond), "reload initial backoff")
	fs.DurationVar(&c.RetryMaxDelay, "retry-max-delay", envDuration("CUESIX_RETRY_MAX_DELAY", 2*time.Second), "reload max backoff")
	fs.Float64Var(&c.RetryMultiplier, "retry-multiplier", envFloat("CUESIX_RETRY_MULTIPLIER", 2), "reload backoff multiplier")
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
