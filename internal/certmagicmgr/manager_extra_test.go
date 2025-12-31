package certmagicmgr

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewManagerValidatesConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{
		Providers: []ProviderConfig{
			{Name: "p1", CA: "https://example", Email: "ops@example.com"},
		},
		DataDir: dir,
	}
	if _, err := NewManager(cfg, nil, nil); err == nil {
		t.Fatalf("expected error for missing logger")
	}
	if _, err := NewManager(Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil); err == nil {
		t.Fatalf("expected error for missing providers")
	}
	if _, err := NewManager(Config{Providers: cfg.Providers}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil); err == nil {
		t.Fatalf("expected error for missing data dir")
	}
	dup := Config{
		Providers: []ProviderConfig{
			{Name: "p1", CA: "https://example", Email: "ops@example.com"},
			{Name: "p1", CA: "https://example", Email: "ops2@example.com"},
		},
		DataDir: dir,
	}
	if _, err := NewManager(dup, slog.New(slog.NewTextHandler(io.Discard, nil)), nil); err == nil {
		t.Fatalf("expected error for duplicate provider")
	}
}

func TestResolveProviderSelections(t *testing.T) {
	manager := &Manager{
		cfg: Config{},
		providers: map[string]*provider{
			"one": {cfg: ProviderConfig{Name: "one"}},
			"two": {cfg: ProviderConfig{Name: "two"}},
		},
	}

	p, err := manager.resolveProvider("two")
	if err != nil {
		t.Fatalf("resolveProvider returned error: %v", err)
	}
	if p.cfg.Name != "two" {
		t.Fatalf("expected provider two, got %q", p.cfg.Name)
	}

	manager.cfg.DefaultProvider = "one"
	p, err = manager.resolveProvider("")
	if err != nil {
		t.Fatalf("resolveProvider returned error: %v", err)
	}
	if p.cfg.Name != "one" {
		t.Fatalf("expected default provider one, got %q", p.cfg.Name)
	}

	manager.cfg.DefaultProvider = ""
	if _, err := manager.resolveProvider(""); err == nil {
		t.Fatalf("expected error when provider is required")
	}

	manager = &Manager{
		cfg: Config{},
		providers: map[string]*provider{
			"only": {cfg: ProviderConfig{Name: "only"}},
		},
	}
	p, err = manager.resolveProvider("")
	if err != nil {
		t.Fatalf("resolveProvider returned error: %v", err)
	}
	if p.cfg.Name != "only" {
		t.Fatalf("expected provider only, got %q", p.cfg.Name)
	}
}

func TestManagerRequestCertificateValidatesInputs(t *testing.T) {
	manager := &Manager{}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	if err := manager.RequestCertificate(context.Background(), logger, "", ""); err == nil {
		t.Fatalf("expected error for missing sni")
	}
	if err := manager.RequestCertificate(context.Background(), logger, "missing", "example.com"); err == nil {
		t.Fatalf("expected error for missing provider")
	}
}

func TestLoadFallbackCertificateReadsPEM(t *testing.T) {
	dir := t.TempDir()
	notAfter := time.Now().UTC().Add(24 * time.Hour).Truncate(time.Second)
	certPath, keyPath := writeFallbackCert(t, dir, notAfter)

	cert, err := LoadFallbackCertificate(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadFallbackCertificate returned error: %v", err)
	}
	if len(cert.CertPEM) == 0 || len(cert.KeyPEM) == 0 {
		t.Fatalf("expected cert and key pem")
	}
	if !timeClose(cert.NotAfter, notAfter, time.Second) {
		t.Fatalf("unexpected notAfter: %v", cert.NotAfter)
	}
}

func TestLoadFallbackCertificateEmptyFiles(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "empty.crt")
	keyPath := filepath.Join(dir, "empty.key")
	if err := os.WriteFile(certPath, []byte{}, 0o644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, []byte{}, 0o644); err != nil {
		t.Fatalf("write key: %v", err)
	}
	if _, err := LoadFallbackCertificate(certPath, keyPath); err == nil {
		t.Fatalf("expected error for empty files")
	}
}

func writeFallbackCert(t *testing.T, dir string, notAfter time.Time) (string, string) {
	t.Helper()
	cert := newTestCert(t, []string{"example.com"}, notAfter)
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Certificate.Certificate[0],
	})
	keyBytes, err := x509.MarshalPKCS8PrivateKey(cert.Certificate.PrivateKey)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})
	certPath := filepath.Join(dir, "fallback.crt")
	keyPath := filepath.Join(dir, "fallback.key")
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certPath, keyPath
}
