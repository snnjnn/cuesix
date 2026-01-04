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
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/cmd/cuesix/config"
	"github.com/warpcomdev/cuesix/internal/reloader"
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
	cfg := config.APISIX{Home: home, MirrorDir: mirror, ValidationTimeout: time.Second}
	v, err := cfg.BuildValidator(logger())
	if err != nil {
		t.Fatalf("BuildValidator returned error: %v", err)
	}
	if err := v.Cleanup(); err != nil {
		t.Fatalf("Cleanup error: %v", err)
	}
}

func TestAPISIXBuildValidatorMissingHome(t *testing.T) {
	t.Parallel()
	cfg := config.APISIX{}
	if _, err := cfg.BuildValidator(logger()); err == nil {
		t.Fatalf("expected error for missing home")
	}
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
	apisixCfg := config.APISIX{Home: "/apisix"}
	pluginCfg := config.Plugins{EnableYAML: true}
	reloadCfg := config.Reload{
		URL:      "http://localhost:9180",
		APIKey:   "k",
		Method:   http.MethodPut,
		Timeout:  time.Second,
		RetryMax: 3,
	}
	rel, err := reloadCfg.BuildReloader(logger(), apisixCfg, pluginCfg)
	if err != nil {
		t.Fatalf("BuildReloader error: %v", err)
	}
	if real, ok := rel.(*reloader.Reloader); ok {
		if !strings.Contains(real.ReloadURL, "/apisix/admin/configs") {
			t.Fatalf("unexpected reload url %s", real.ReloadURL)
		}
	}

	reloadCfg.DryRun = true
	rel, err = reloadCfg.BuildReloader(logger(), apisixCfg, pluginCfg)
	if err != nil {
		t.Fatalf("dry run BuildReloader error: %v", err)
	}
	if err := rel.Apply(context.Background(), []byte("payload"), true); err != nil {
		t.Fatalf("expected dry run Apply to succeed, got %v", err)
	}
}

func TestServerTimeouts(t *testing.T) {
	t.Parallel()
	s := config.Server{ReadTimeout: time.Second}
	to := s.Timeouts()
	if to.ReadTimeout != s.ReadTimeout {
		t.Fatalf("timeouts mismatch")
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
