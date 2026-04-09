package config

import (
	"errors"
	"path/filepath"
	"time"

	"github.com/warpcondev/cuesix/internal/plugin/ssl"
	"github.com/urfave/cli/v3"
)

type Plugins struct {
	EnableSSL      bool
	SSLPaths       []string
	SSLACMETimeout time.Duration
	FallbackCert   string
	FallbackKey    string
	EnvFilename    string
}

// Flags returns plugin-related command-line flags.
func (c *Plugins) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:     "plugin-ssl",
			Usage:    "enable ssl pre-render plugin",
			Sources:  cli.EnvVars("SIXPACK_PLUGIN_SSL"),
			Category: "Plugins",
		},
		&cli.DurationFlag{
			Name:     "plugin-ssl-acme-timeout",
			Usage:    "timeout for ssl plugin acme requests",
			Sources:  cli.EnvVars("SIXPACK_PLUGIN_SSL_ACME_TIMEOUT"),
			Value:    10 * time.Second,
			Category: "Plugins",
		},
		&cli.StringFlag{
			Name:     "plugin-ssl-fallback-cert",
			Usage:    "ssl plugin fallback certificate path",
			Sources:  cli.EnvVars("SIXPACK_PLUGIN_SSL_FALLBACK_CERT"),
			Category: "Plugins",
		},
		&cli.StringFlag{
			Name:     "plugin-ssl-fallback-key",
			Usage:    "ssl plugin fallback key path",
			Sources:  cli.EnvVars("SIXPACK_PLUGIN_SSL_FALLBACK_KEY"),
			Category: "Plugins",
		},
		&cli.StringSliceFlag{
			Name:     "plugin-ssl-path",
			Usage:    "ssl plugin certificate path (repeatable)",
			Sources:  cli.EnvVars("SIXPACK_PLUGIN_SSL_PATHS"),
			Category: "Plugins",
		},
		&cli.StringFlag{
			Name:     "plugin-env",
			Usage:    "env filename to load from each input directory",
			Sources:  cli.EnvVars("SIXPACK_PLUGIN_ENV"),
			Category: "Plugins",
		},
	}
}

// Apply loads plugin settings from parsed CLI flags.
func (c *Plugins) Apply(ctx *cli.Command) {
	c.EnableSSL = ctx.Bool("plugin-ssl")
	c.SSLACMETimeout = ctx.Duration("plugin-ssl-acme-timeout")
	c.SSLPaths = ctx.StringSlice("plugin-ssl-path")
	c.FallbackCert = ctx.String("plugin-ssl-fallback-cert")
	c.FallbackKey = ctx.String("plugin-ssl-fallback-key")
	c.EnvFilename = ctx.String("plugin-env")
}

// LoadFallbackCertificate loads the configured SSL fallback certificate pair.
func (c *Plugins) LoadFallbackCertificate(apisixHome string, certmagicEnabled bool) (ssl.PEMCertificate, bool, error) {
	if len(c.SSLPaths) == 0 && !certmagicEnabled {
		return ssl.PEMCertificate{}, false, nil
	}
	certPath := c.FallbackCert
	if certPath == "" {
		certPath = filepath.Join(apisixHome, "conf", "cert", "ssl_PLACE_HOLDER.crt")
	}
	keyPath := c.FallbackKey
	if keyPath == "" {
		keyPath = filepath.Join(apisixHome, "conf", "cert", "ssl_PLACE_HOLDER.key")
	}
	cert, err := ssl.LoadFallbackCertificate(certPath, keyPath)
	if err != nil {
		return ssl.PEMCertificate{}, true, err
	}
	c.FallbackCert = certPath
	c.FallbackKey = keyPath
	return cert, true, nil
}

// Validate checks plugin configuration constraints.
func (c *Plugins) Validate() error {
	if c.EnableSSL && c.SSLACMETimeout <= 0 {
		return errors.New("plugin ssl acme timeout must be positive")
	}
	return nil
}
