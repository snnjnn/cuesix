package config

import (
	"path/filepath"
	"time"

	"github.com/urfave/cli/v2"
	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
)

type Plugins struct {
	EnableSSL      bool
	EnableYAML     bool
	EnableJQ       bool
	JQTimeout      time.Duration
	SSLPaths       []string
	SSLACMETimeout time.Duration
	FallbackCert   string
	FallbackKey    string
}

func (c *Plugins) Flags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			Name:     "plugin-ssl",
			Usage:    "enable ssl pre-render plugin",
			EnvVars:  []string{"CUESIX_PLUGIN_SSL"},
			Category: "Plugins",
		},
		&cli.BoolFlag{
			Name:     "plugin-yaml",
			Usage:    "enable yaml post-render plugin",
			EnvVars:  []string{"CUESIX_PLUGIN_YAML"},
			Category: "Plugins",
		},
		&cli.BoolFlag{
			Name:     "plugin-jq",
			Usage:    "enable jq post-render plugin",
			EnvVars:  []string{"CUESIX_PLUGIN_JQ"},
			Value:    true,
			Category: "Plugins",
		},
		&cli.DurationFlag{
			Name:     "plugin-ssl-acme-timeout",
			Usage:    "timeout for ssl plugin acme requests",
			EnvVars:  []string{"CUESIX_PLUGIN_SSL_ACME_TIMEOUT"},
			Value:    ssl.DefaultACMERequestTimeout,
			Category: "Plugins",
		},
		&cli.DurationFlag{
			Name:     "plugin-jq-timeout",
			Usage:    "timeout for jq transforms",
			EnvVars:  []string{"CUESIX_PLUGIN_JQ_TIMEOUT"},
			Value:    10 * time.Second,
			Category: "Plugins",
		},
		&cli.StringFlag{
			Name:     "plugin-ssl-fallback-cert",
			Usage:    "ssl plugin fallback certificate path",
			EnvVars:  []string{"CUESIX_PLUGIN_SSL_FALLBACK_CERT"},
			Category: "Plugins",
		},
		&cli.StringFlag{
			Name:     "plugin-ssl-fallback-key",
			Usage:    "ssl plugin fallback key path",
			EnvVars:  []string{"CUESIX_PLUGIN_SSL_FALLBACK_KEY"},
			Category: "Plugins",
		},
		&cli.StringSliceFlag{
			Name:     "plugin-ssl-path",
			Usage:    "ssl plugin certificate path (repeatable)",
			EnvVars:  []string{"CUESIX_PLUGIN_SSL_PATHS"},
			Category: "Plugins",
		},
	}
}

func (c *Plugins) Apply(ctx *cli.Context) {
	c.EnableSSL = ctx.Bool("plugin-ssl")
	c.EnableYAML = ctx.Bool("plugin-yaml")
	c.EnableJQ = ctx.Bool("plugin-jq")
	c.JQTimeout = ctx.Duration("plugin-jq-timeout")
	c.SSLACMETimeout = ctx.Duration("plugin-ssl-acme-timeout")
	c.SSLPaths = ctx.StringSlice("plugin-ssl-path")
	c.FallbackCert = ctx.String("plugin-ssl-fallback-cert")
	c.FallbackKey = ctx.String("plugin-ssl-fallback-key")
}

func (c *Plugins) LoadFallbackCertificate(apisixHome string, certmagicEnabled bool) (ssl.Certificate, bool, error) {
	if len(c.SSLPaths) == 0 && !certmagicEnabled {
		return ssl.Certificate{}, false, nil
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
		return ssl.Certificate{}, true, err
	}
	c.FallbackCert = certPath
	c.FallbackKey = keyPath
	return cert, true, nil
}
