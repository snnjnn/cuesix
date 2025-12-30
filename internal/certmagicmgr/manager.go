package certmagicmgr

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/caddyserver/certmagic"
)

// ProviderConfig configures a single ACME provider.
type ProviderConfig struct {
	Name    string
	CA      string
	Email   string
	Timeout time.Duration
}

// Config describes the certmagic manager configuration.
type Config struct {
	Providers        []ProviderConfig
	DefaultProvider  string
	DataDir          string
	DefaultTimeout   time.Duration
	FallbackCertPath string
	FallbackKeyPath  string
}

// ManagedCert describes a managed certificate entry.
type ManagedCert struct {
	SNI        string
	Provider   string
	NotAfter   time.Time
	ObtainedAt time.Time
}

// Certificate bundles a certificate chain and key in PEM format.
type Certificate struct {
	CertPEM  []byte
	KeyPEM   []byte
	NotAfter time.Time
}

type provider struct {
	cfg   ProviderConfig
	cache *certmagic.Cache
	magic *certmagic.Config
}

// Manager owns certmagic configuration and serialized operations.
type Manager struct {
	cfg       Config
	storage   certmagic.Storage
	providers map[string]provider
	fallback  Certificate
}

// NewManager builds a certmagic manager and validates configuration.
func NewManager(cfg Config, logger *slog.Logger) (*Manager, error) {
	if logger == nil {
		return nil, errors.New("logger is required")
	}
	if len(cfg.Providers) == 0 {
		return nil, errors.New("at least one provider is required")
	}
	if cfg.DataDir == "" {
		return nil, errors.New("data dir is required")
	}
	if strings.TrimSpace(cfg.FallbackCertPath) == "" {
		return nil, errors.New("fallback cert path is required")
	}
	if strings.TrimSpace(cfg.FallbackKeyPath) == "" {
		return nil, errors.New("fallback key path is required")
	}
	fallback, err := loadFallbackCertificate(cfg.FallbackCertPath, cfg.FallbackKeyPath)
	if err != nil {
		return nil, err
	}
	storage := &certmagic.FileStorage{Path: cfg.DataDir}
	providers := make(map[string]provider, len(cfg.Providers))
	for _, p := range cfg.Providers {
		if p.Name == "" || p.CA == "" || p.Email == "" {
			return nil, fmt.Errorf("provider %q requires name, ca, and email", p.Name)
		}
		if _, exists := providers[p.Name]; exists {
			return nil, fmt.Errorf("duplicate provider name: %s", p.Name)
		}
		providers[p.Name] = buildProvider(logger, cfg, p, storage)
	}
	return &Manager{
		cfg:       cfg,
		storage:   storage,
		providers: providers,
		fallback:  fallback,
	}, nil
}

// RunChallengeServer exposes the HTTP-01 challenge handler.
func (m *Manager) ChallengeHandler(logger *slog.Logger) http.Handler {
	return certmagic.DefaultACME.HTTPChallengeHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("challenge request", "host", r.Host, "url", r.URL.String())
	}))
}

// RequestCertificate obtains or loads a certificate for the given SNI.
func (m *Manager) RequestCertificate(ctx context.Context, logger *slog.Logger, providerName string, sni string) (Certificate, error) {
	if strings.TrimSpace(sni) == "" {
		return Certificate{}, errors.New("sni is required")
	}
	p, err := m.resolveProvider(providerName)
	if err != nil {
		return Certificate{}, err
	}
	timeout := p.cfg.Timeout
	if timeout <= 0 {
		timeout = m.cfg.DefaultTimeout
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if err := p.magic.ManageSync(ctx, []string{sni}); err != nil {
		return Certificate{}, err
	}
	cert, err := p.magic.CacheManagedCertificate(ctx, sni)
	if err != nil {
		return Certificate{}, err
	}
	certPEM, keyPEM, notAfter, err := marshalCertificate(cert)
	if err != nil {
		return Certificate{}, err
	}
	return Certificate{CertPEM: certPEM, KeyPEM: keyPEM, NotAfter: notAfter}, nil
}

// FallbackCertificate returns the configured fallback certificate.
func (m *Manager) FallbackCertificate() (Certificate, error) {
	if len(m.fallback.CertPEM) == 0 || len(m.fallback.KeyPEM) == 0 {
		return Certificate{}, errors.New("fallback certificate not configured")
	}
	return m.fallback, nil
}

// RemoveManaged stops tracking the SNI for future listings.
func (m *Manager) RemoveManaged(logger *slog.Logger, providerName string, sni string) {
	provider, err := m.resolveProvider(providerName)
	if err != nil {
		logger.Error("removing managed cert", "error", err)
		return
	}
	for _, issuer := range provider.magic.Issuers {
		c := certmagic.SubjectIssuer{
			Subject:   sni,
			IssuerKey: issuer.IssuerKey(),
		}
		provider.cache.RemoveManaged([]certmagic.SubjectIssuer{c})
	}
}

func (m *Manager) RemoveExpired(ctx context.Context, logger *slog.Logger) error {
	return certmagic.CleanStorage(ctx, m.storage, certmagic.CleanStorageOptions{
		Interval:               12 * time.Hour,
		ExpiredCerts:           true,
		ExpiredCertGracePeriod: 5 * 25 * time.Hour,
	})
}

func (m *Manager) resolveProvider(name string) (provider, error) {
	if name != "" {
		p, ok := m.providers[name]
		if !ok {
			return provider{}, fmt.Errorf("unknown provider: %s", name)
		}
		return p, nil
	}
	if m.cfg.DefaultProvider != "" {
		p, ok := m.providers[m.cfg.DefaultProvider]
		if !ok {
			return provider{}, fmt.Errorf("unknown default provider: %s", m.cfg.DefaultProvider)
		}
		return p, nil
	}
	if len(m.providers) == 1 {
		for _, p := range m.providers {
			return p, nil
		}
	}
	return provider{}, errors.New("provider is required")
}

func buildProvider(logger *slog.Logger, cfg Config, providerCfg ProviderConfig, storage certmagic.Storage) provider {
	var (
		cache *certmagic.Cache
		magic *certmagic.Config
	)
	cache = certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(cert certmagic.Certificate) (*certmagic.Config, error) {
			return magic, nil
		},
	})
	magic = certmagic.New(cache, *certmagic.NewDefault())
	magic.Storage = storage
	issuerCfg := certmagic.ACMEIssuer{
		CA:     providerCfg.CA,
		Email:  providerCfg.Email,
		Agreed: true,
	}
	timeout := providerCfg.Timeout
	if timeout <= 0 {
		timeout = cfg.DefaultTimeout
	}
	if timeout > 0 {
		issuerCfg.CertObtainTimeout = timeout
	}
	issuer := certmagic.NewACMEIssuer(magic, issuerCfg)
	magic.Issuers = []certmagic.Issuer{issuer}
	return provider{
		cfg:   providerCfg,
		cache: cache,
		magic: magic,
	}
}

func marshalCertificate(cert certmagic.Certificate) ([]byte, []byte, time.Time, error) {
	var certPEM []byte
	for _, der := range cert.Certificate.Certificate {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: der,
		})...)
	}
	if len(certPEM) == 0 {
		return nil, nil, time.Time{}, errors.New("empty certificate chain")
	}
	key, err := x509.MarshalPKCS8PrivateKey(cert.Certificate.PrivateKey)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: key,
	})
	notAfter, err := leafNotAfter(cert.Certificate.Certificate)
	if err != nil {
		return nil, nil, time.Time{}, err
	}
	return certPEM, keyPEM, notAfter, nil
}

func loadFallbackCertificate(certPath string, keyPath string) (Certificate, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return Certificate{}, fmt.Errorf("read fallback cert: %w", err)
	}
	if len(certPEM) == 0 {
		return Certificate{}, errors.New("fallback cert is empty")
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return Certificate{}, fmt.Errorf("read fallback key: %w", err)
	}
	if len(keyPEM) == 0 {
		return Certificate{}, errors.New("fallback key is empty")
	}
	notAfter, err := parseCertNotAfter(certPEM)
	if err != nil {
		return Certificate{}, fmt.Errorf("parse fallback cert: %w", err)
	}
	return Certificate{CertPEM: certPEM, KeyPEM: keyPEM, NotAfter: notAfter}, nil
}

func parseCertNotAfter(certPEM []byte) (time.Time, error) {
	rest := certPEM
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return time.Time{}, err
		}
		return cert.NotAfter, nil
	}
	return time.Time{}, errors.New("fallback cert missing certificate block")
}

func leafNotAfter(chain [][]byte) (time.Time, error) {
	if len(chain) == 0 {
		return time.Time{}, errors.New("missing certificate chain")
	}
	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return time.Time{}, err
	}
	return leaf.NotAfter, nil
}
