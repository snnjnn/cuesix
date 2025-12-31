package config

import (
	"flag"
	"path/filepath"
	"time"

	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
)

type Plugins struct {
	EnableYAML     bool
	EnableJQ       bool
	JQTimeout      time.Duration
	SSLPaths       []string
	SSLACMETimeout time.Duration
	FallbackCert   string
	FallbackKey    string
}

func (c *Plugins) RegisterFlags(fs *flag.FlagSet) {
	c.SSLPaths = splitComma(envString("CUESIX_PLUGIN_SSL_PATHS"))
	fs.BoolVar(&c.EnableYAML, "plugin-yaml", envBool("CUESIX_PLUGIN_YAML", false), "enable yaml post-render plugin")
	fs.BoolVar(&c.EnableJQ, "plugin-jq", envBool("CUESIX_PLUGIN_JQ", true), "enable jq post-render plugin")
	fs.DurationVar(&c.SSLACMETimeout, "plugin-ssl-acme-timeout", envDuration("CUESIX_PLUGIN_SSL_ACME_TIMEOUT", ssl.DefaultACMERequestTimeout), "timeout for ssl plugin acme requests")
	fs.DurationVar(&c.JQTimeout, "plugin-jq-timeout", envDuration("CUESIX_PLUGIN_JQ_TIMEOUT", 10*time.Second), "timeout for jq transforms")
	fs.StringVar(&c.FallbackCert, "plugin-ssl-fallback-cert", envString("CUESIX_PLUGIN_SSL_FALLBACK_CERT"), "ssl plugin fallback certificate path")
	fs.StringVar(&c.FallbackKey, "plugin-ssl-fallback-key", envString("CUESIX_PLUGIN_SSL_FALLBACK_KEY"), "ssl plugin fallback key path")
	fs.Var(&stringSliceValue{target: &c.SSLPaths}, "plugin-ssl-path", "ssl plugin certificate path (repeatable)")
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
