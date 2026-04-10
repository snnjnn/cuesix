package config_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/urfave/cli/v3"
	"github.com/warpcomdev/cuesix/cmd/sixpack/config"
	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/reloader"
	"github.com/warpcomdev/cuesix/internal/validator"
)

func logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestAPISIXBuildValidator(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	mirror := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, "conf"), 0o755); err != nil {
		t.Fatalf("mkdir conf: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, "conf", "apisix.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg := config.StandaloneValidator{MirrorDir: mirror, ValidationTimeout: time.Second}
	v, err := cfg.BuildValidator(logger(), home)
	if err != nil {
		t.Fatalf("BuildValidator returned error: %v", err)
	}
	if err := v.Cleanup(); err != nil {
		t.Fatalf("Cleanup error: %v", err)
	}
}

func TestAPISIXBuildValidatorMissingHome(t *testing.T) {
	t.Parallel()
	cfg := config.StandaloneValidator{}
	if _, err := cfg.BuildValidator(logger(), ""); err == nil {
		t.Fatalf("expected error for missing home")
	}
}

func TestAPISIXValidate(t *testing.T) {
	t.Parallel()

	invalid := &config.Apisix{}
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected validation error for missing virtual gateway")
	}
	valid := &config.Apisix{Virtualgw: compiler.DEFAULT_VIRTUALGW}
	if err := valid.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestStandaloneValidatorMaxGatewayDepthValidate(t *testing.T) {
	t.Parallel()

	invalid := config.StandaloneValidator{MaxGatewayDepth: -1}
	if err := invalid.Validate(); err == nil {
		t.Fatalf("expected validation error for negative max-gateway-depth")
	}

	valid := config.StandaloneValidator{MaxGatewayDepth: 0}
	if err := valid.Validate(); err != nil {
		t.Fatalf("unexpected validation error for max-gateway-depth=0: %v", err)
	}
}

func TestStandaloneValidatorFlagsDefaultMaxGatewayDepth(t *testing.T) {
	t.Parallel()

	cfg := &config.StandaloneValidator{}
	flags := cfg.Flags()
	for _, flag := range flags {
		intFlag, ok := flag.(*cli.IntFlag)
		if !ok || intFlag.Name != "max-gateway-depth" {
			continue
		}
		if intFlag.Value != 0 {
			t.Fatalf("expected default max-gateway-depth=0, got %d", intFlag.Value)
		}
		return
	}
	t.Fatalf("max-gateway-depth flag not found")
}

func TestCertmagicValidate(t *testing.T) {
	t.Parallel()
	c := config.Certmagic{Enabled: true, WatchInterval: 0, DataDir: t.TempDir()}
	if err := c.Validate(); err == nil {
		t.Fatalf("expected validation error for non-positive watch interval")
	}
	c.WatchInterval = time.Second
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInputValidate(t *testing.T) {
	t.Parallel()
	c := config.Input{}
	if err := c.Validate(); err == nil {
		t.Fatalf("expected error for missing input dirs")
	}
	c.InputDirs = []string{"/tmp"}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPluginsLoadFallbackCertificate(t *testing.T) {
	t.Parallel()
	c := config.Plugins{}
	if _, ok, err := c.LoadFallbackCertificate("", false); ok || err != nil {
		t.Fatalf("expected no load when ssl disabled and no paths, got ok=%v err=%v", ok, err)
	}

	home := t.TempDir()
	certDir := filepath.Join(home, "conf", "cert")
	if err := os.MkdirAll(certDir, 0o755); err != nil {
		t.Fatalf("mkdir cert dir: %v", err)
	}
	certPath := filepath.Join(certDir, "ssl_PLACE_HOLDER.crt")
	keyPath := filepath.Join(certDir, "ssl_PLACE_HOLDER.key")
	certPEM, keyPEM := makeCert(t)
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o644); err != nil {
		t.Fatalf("write key: %v", err)
	}
	c.SSLPaths = []string{"dummy"} // force load
	cert, ok, err := c.LoadFallbackCertificate(home, false)
	if err != nil || !ok {
		t.Fatalf("expected fallback loaded, ok=%v err=%v", ok, err)
	}
	if len(cert.CertPEM) == 0 || len(cert.KeyPEM) == 0 {
		t.Fatalf("expected pem contents")
	}
	if !strings.HasSuffix(c.FallbackCert, ".crt") || !strings.HasSuffix(c.FallbackKey, ".key") {
		t.Fatalf("expected fallback paths set, got %s %s", c.FallbackCert, c.FallbackKey)
	}
}

func TestReloadBuildReloader(t *testing.T) {
	t.Parallel()
	apisixCfg := config.Apisix{Home: "/apisix"}
	reloadCfg := config.Reload{}
	rel, err := reloadCfg.BuildReloader(logger(), compiler.DEFAULT_VIRTUALGW, validator.BuildConfigPath(apisixCfg.Home, true))
	if err != nil {
		t.Fatalf("BuildReloader error: %v", err)
	}
	if real, ok := rel.(*reloader.FileReloader); ok {
		if !strings.HasSuffix(real.ConfigPath, "apisix.yaml") {
			t.Fatalf("unexpected config path %s", real.ConfigPath)
		}
	}

	reloadCfg.DryRun = true
	rel, err = reloadCfg.BuildReloader(logger(), compiler.DEFAULT_VIRTUALGW, validator.BuildConfigPath(apisixCfg.Home, true))
	if err != nil {
		t.Fatalf("dry run BuildReloader error: %v", err)
	}
	if err := rel.Apply(context.Background(), compiler.DEFAULT_VIRTUALGW, []byte("payload")); err != nil {
		t.Fatalf("expected dry run Apply to succeed, got %v", err)
	}
}

func TestServerTimeouts(t *testing.T) {
	t.Parallel()
	s := config.HTTPServer{ReadTimeout: time.Second}
	to := s.Timeouts()
	if to.ReadTimeout != s.ReadTimeout {
		t.Fatalf("timeouts mismatch")
	}
}

func TestClientValidate(t *testing.T) {
	t.Parallel()

	valid := config.Client{
		BaseURL:              "https://example.com",
		ConnectTimeout:       time.Second,
		ReadTimeout:          2 * time.Second,
		BackoffInitial:       100 * time.Millisecond,
		BackoffMaxInterval:   time.Second,
		BackoffMultiplier:    2,
		BackoffRandomization: 0.5,
		BackoffMaxElapsed:    0,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	tests := []struct {
		name string
		cfg  config.Client
	}{
		{name: "bad url", cfg: config.Client{BaseURL: "://bad", ConnectTimeout: time.Second, ReadTimeout: time.Second, BackoffInitial: time.Second, BackoffMaxInterval: time.Second, BackoffMultiplier: 2}},
		{name: "relative url", cfg: config.Client{BaseURL: "/local", ConnectTimeout: time.Second, ReadTimeout: time.Second, BackoffInitial: time.Second, BackoffMaxInterval: time.Second, BackoffMultiplier: 2}},
		{name: "zero connect timeout", cfg: config.Client{BaseURL: "https://example.com", ReadTimeout: time.Second, BackoffInitial: time.Second, BackoffMaxInterval: time.Second, BackoffMultiplier: 2}},
		{name: "zero read timeout", cfg: config.Client{BaseURL: "https://example.com", ConnectTimeout: time.Second, BackoffInitial: time.Second, BackoffMaxInterval: time.Second, BackoffMultiplier: 2}},
		{name: "zero initial", cfg: config.Client{BaseURL: "https://example.com", ConnectTimeout: time.Second, ReadTimeout: time.Second, BackoffMaxInterval: time.Second, BackoffMultiplier: 2}},
		{name: "zero max interval", cfg: config.Client{BaseURL: "https://example.com", ConnectTimeout: time.Second, ReadTimeout: time.Second, BackoffInitial: time.Second, BackoffMultiplier: 2}},
		{name: "zero multiplier", cfg: config.Client{BaseURL: "https://example.com", ConnectTimeout: time.Second, ReadTimeout: time.Second, BackoffInitial: time.Second, BackoffMaxInterval: time.Second}},
		{name: "negative randomization", cfg: config.Client{BaseURL: "https://example.com", ConnectTimeout: time.Second, ReadTimeout: time.Second, BackoffInitial: time.Second, BackoffMaxInterval: time.Second, BackoffMultiplier: 2, BackoffRandomization: -0.1}},
		{name: "randomization too large", cfg: config.Client{BaseURL: "https://example.com", ConnectTimeout: time.Second, ReadTimeout: time.Second, BackoffInitial: time.Second, BackoffMaxInterval: time.Second, BackoffMultiplier: 2, BackoffRandomization: 1.1}},
		{name: "negative max elapsed", cfg: config.Client{BaseURL: "https://example.com", ConnectTimeout: time.Second, ReadTimeout: time.Second, BackoffInitial: time.Second, BackoffMaxInterval: time.Second, BackoffMultiplier: 2, BackoffMaxElapsed: -time.Second}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.cfg.Validate(); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestClientBuildBackoffFactory(t *testing.T) {
	t.Parallel()

	cfg := config.Client{
		BackoffInitial:       100 * time.Millisecond,
		BackoffMaxInterval:   time.Second,
		BackoffMultiplier:    2,
		BackoffRandomization: 0,
		BackoffMaxElapsed:    0,
		BackoffMaxRetries:    3,
	}

	bo1 := cfg.BuildBackoffFactory()()
	for i := range 3 {
		got := bo1.NextBackOff()
		if got <= 0 || got > cfg.BackoffMaxInterval {
			t.Fatalf("backoff[%d] = %v", i, got)
		}
	}
	if got := bo1.NextBackOff(); got != backoff.Stop {
		t.Fatalf("expected stop after max retries, got %v", got)
	}

	bo2 := cfg.BuildBackoffFactory()()
	if got := bo2.NextBackOff(); got <= 0 || got > cfg.BackoffMaxInterval {
		t.Fatalf("new factory first backoff = %v", got)
	}
}

func TestAPIControlBuildBackoff(t *testing.T) {
	t.Parallel()

	cfg := config.APIControl{
		RetryMax:        2,
		RetryInitial:    50 * time.Millisecond,
		RetryMaxDelay:   500 * time.Millisecond,
		RetryMultiplier: 2,
		RetryJitter:     0.5,
	}

	bo := cfg.BuildBackoff(false)
	for i := 0; i < cfg.RetryMax; i++ {
		got := bo.NextBackOff()
		if got <= 0 {
			t.Fatalf("backoff[%d] = %v", i, got)
		}
	}
	if got := bo.NextBackOff(); got != backoff.Stop {
		t.Fatalf("expected finite backoff to stop, got %v", got)
	}

	forever := cfg.BuildBackoff(true)
	for i := 0; i < cfg.RetryMax+2; i++ {
		got := forever.NextBackOff()
		if got <= 0 {
			t.Fatalf("forever backoff[%d] = %v", i, got)
		}
	}
}

func TestAPIControlValidate(t *testing.T) {
	t.Parallel()

	if err := (&config.APIControl{DeploymentMode: config.StandaloneMode, RetryJitter: -0.1}).Validate(); err == nil {
		t.Fatalf("expected validation error")
	}
	if err := (&config.APIControl{DeploymentMode: config.StandaloneMode, RetryJitter: 1.1}).Validate(); err == nil {
		t.Fatalf("expected validation error")
	}
	if err := (&config.APIControl{DeploymentMode: config.StandaloneMode, RetryJitter: 0.5}).Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestServerSideEventsValidate(t *testing.T) {
	t.Parallel()

	if err := (&config.ServerSideEvents{KeepAlive: -time.Second}).Validate(); err == nil {
		t.Fatalf("expected validation error")
	}
	if err := (&config.ServerSideEvents{KeepAlive: 0}).Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPluginsValidateMore(t *testing.T) {
	t.Parallel()

	if err := (&config.Plugins{EnableSSL: true, SSLACMETimeout: 0}).Validate(); err == nil {
		t.Fatalf("expected validation error")
	}
	if err := (&config.Plugins{EnableSSL: false, SSLACMETimeout: 0}).Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func makeCert(t *testing.T) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("key: %v", err)
	}
	tpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "example"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})
	return certPEM, keyPEM
}
