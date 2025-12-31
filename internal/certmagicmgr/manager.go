package certmagicmgr

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
)

// ProviderConfig configures a single ACME provider.
type ProviderConfig struct {
	Name    string
	CA      string
	Email   string
}

// Config describes the certmagic manager configuration.
type Config struct {
	Providers       []ProviderConfig
	DefaultProvider string
	DataDir         string
	DefaultTimeout  time.Duration
}

type provider struct {
	sync.Mutex
	cfg   ProviderConfig
	cache *certmagic.Cache
	magic *certmagic.Config
}

type CertEvent struct {
	sni      string
	provider providerView
}

// providerView exposes provider operations needed by Watcher.
type providerView interface {
	Name() string
	BestMatchFor(sni string, logger *slog.Logger) (ssl.Certificate, bool)
	RemoveManaged(logger *slog.Logger, sni string)
}

// Manager owns certmagic configuration and serialized operations.
type Manager struct {
	cfg       Config
	storage   certmagic.Storage
	providers map[string]*provider
	fallback  ssl.Certificate
}

// NewManager builds a certmagic manager and validates configuration.
func NewManager(cfg Config, logger *slog.Logger, events chan CertEvent) (*Manager, error) {
	if logger == nil {
		return nil, errors.New("logger is required")
	}
	if len(cfg.Providers) == 0 {
		return nil, errors.New("at least one provider is required")
	}
	if cfg.DataDir == "" {
		return nil, errors.New("data dir is required")
	}
	storage := &certmagic.FileStorage{Path: cfg.DataDir}
	providers := make(map[string]*provider, len(cfg.Providers))
	for _, p := range cfg.Providers {
		if p.Name == "" || p.CA == "" || p.Email == "" {
			return nil, fmt.Errorf("provider %q requires name, ca, and email", p.Name)
		}
		if _, exists := providers[p.Name]; exists {
			return nil, fmt.Errorf("duplicate provider name: %s", p.Name)
		}
		providers[p.Name] = buildProvider(logger, cfg, p, storage, events)
	}
	return &Manager{
		cfg:       cfg,
		storage:   storage,
		providers: providers,
	}, nil
}

// RunChallengeServer exposes the HTTP-01 challenge handler.
func (m *Manager) ChallengeHandler(logger *slog.Logger) http.Handler {
	return certmagic.DefaultACME.HTTPChallengeHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("challenge request", "host", r.Host, "url", r.URL.String())
	}))
}

// RequestCertificate obtains or loads a certificate for the given SNI.
func (m *Manager) RequestCertificate(ctx context.Context, logger *slog.Logger, providerName string, sni string) error {
	if strings.TrimSpace(sni) == "" {
		return errors.New("sni is required")
	}
	p, err := m.resolveProvider(providerName)
	if err != nil {
		return err
	}
	if m.cfg.DefaultTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.cfg.DefaultTimeout)
		defer cancel()
	}
	p.Lock()
	defer p.Unlock()
	if err := p.magic.ManageAsync(ctx, []string{sni}); err != nil {
		return err
	}
	return nil
}

// FallbackCertificate returns the configured fallback certificate.
func (m *Manager) FallbackCertificate() (ssl.Certificate, error) {
	if len(m.fallback.CertPEM) == 0 || len(m.fallback.KeyPEM) == 0 {
		return ssl.Certificate{}, errors.New("fallback certificate not configured")
	}
	return m.fallback, nil
}

func (m *Manager) RemoveExpired(ctx context.Context, logger *slog.Logger, interval time.Duration, gracePeriod time.Duration) error {
	return certmagic.CleanStorage(ctx, m.storage, certmagic.CleanStorageOptions{
		Interval:               interval,
		ExpiredCerts:           true,
		ExpiredCertGracePeriod: gracePeriod,
	})
}

func (m *Manager) resolveProvider(name string) (*provider, error) {
	if name != "" {
		p, ok := m.providers[name]
		if !ok {
			return nil, fmt.Errorf("unknown provider: %s", name)
		}
		return p, nil
	}
	if m.cfg.DefaultProvider != "" {
		p, ok := m.providers[m.cfg.DefaultProvider]
		if !ok {
			return nil, fmt.Errorf("unknown default provider: %s", m.cfg.DefaultProvider)
		}
		return p, nil
	}
	if len(m.providers) == 1 {
		for _, p := range m.providers {
			return p, nil
		}
	}
	return nil, errors.New("provider is required")
}

func (m *Manager) resolveProviderView(name string) (providerView, error) {
	p, err := m.resolveProvider(name)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func buildProvider(logger *slog.Logger, cfg Config, providerCfg ProviderConfig, storage certmagic.Storage, events chan CertEvent) *provider {
	p := &provider{
		cfg: providerCfg,
	}
	configTemplate := *certmagic.NewDefault()
	configTemplate.Storage = storage
	if events != nil {
		configTemplate.OnEvent = func(ctx context.Context, event string, data map[string]any) error {
			logger.Info("certmagic event", "event", event, "data", data)
			if event == "cert_obtained" {
				if sni, ok := data["identifier"].(string); ok {
					select {
					case <-ctx.Done():
						return nil
					case events <- CertEvent{sni: sni, provider: p}:
					default:
					}
				}
			}
			return nil
		}
	}
	p.cache = certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(cert certmagic.Certificate) (*certmagic.Config, error) {
			return p.magic, nil
		},
	})
	p.magic = certmagic.New(p.cache, configTemplate)
	issuerCfg := certmagic.ACMEIssuer{
		CA:     providerCfg.CA,
		Email:  providerCfg.Email,
		Agreed: true,
	}
	if cfg.DefaultTimeout > 0 {
		issuerCfg.CertObtainTimeout = cfg.DefaultTimeout
	}
	issuer := certmagic.NewACMEIssuer(p.magic, issuerCfg)
	p.magic.Issuers = []certmagic.Issuer{issuer}
	return p
}

func (p *provider) Name() string {
	return p.cfg.Name
}

func (p *provider) BestMatchFor(sni string, logger *slog.Logger) (ssl.Certificate, bool) {
	sni = strings.TrimSpace(sni)
	if sni == "" {
		return ssl.Certificate{}, false
	}
	p.Lock()
	matches := append([]certmagic.Certificate(nil), p.cache.AllMatchingCertificates(sni)...)
	p.Unlock()
	return bestMatchForCandidates(matches, logger)
}

// RemoveManaged stops tracking the SNI for future listings.
func (p *provider) RemoveManaged(logger *slog.Logger, sni string) {
	p.Lock()
	defer p.Unlock()
	for _, issuer := range p.magic.Issuers {
		c := certmagic.SubjectIssuer{
			Subject:   sni,
			IssuerKey: issuer.IssuerKey(),
		}
		p.cache.RemoveManaged([]certmagic.SubjectIssuer{c})
	}
}

func MarshalCertificate(cert certmagic.Certificate) (ssl.Certificate, error) {
	var certPEM []byte
	for _, der := range cert.Certificate.Certificate {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: der,
		})...)
	}
	if len(certPEM) == 0 {
		return ssl.Certificate{}, errors.New("empty certificate chain")
	}
	key, err := x509.MarshalPKCS8PrivateKey(cert.Certificate.PrivateKey)
	if err != nil {
		return ssl.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: key,
	})
	notAfter := time.Time{}
	if cert.Leaf != nil {
		notAfter = cert.Leaf.NotAfter
	} else {
		var err error
		notAfter, err = leafNotAfter(cert.Certificate.Certificate)
		if err != nil {
			return ssl.Certificate{}, err
		}
	}
	return ssl.Certificate{
		CertPEM:  certPEM,
		KeyPEM:   keyPEM,
		NotAfter: notAfter,
	}, nil
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

func parseLeaf(chain [][]byte) (*x509.Certificate, error) {
	if len(chain) == 0 {
		return nil, errors.New("missing certificate chain")
	}
	return x509.ParseCertificate(chain[0])
}

func bestMatchForCandidates(matches []certmagic.Certificate, logger *slog.Logger) (ssl.Certificate, bool) {
	if logger == nil {
		logger = slog.Default()
	}
	type candidate struct {
		cert     certmagic.Certificate
		notAfter time.Time
	}
	candidates := make([]candidate, 0, len(matches))
	for _, match := range matches {
		if match.Empty() {
			continue
		}
		leaf := match.Leaf
		if leaf == nil {
			parsed, err := parseLeaf(match.Certificate.Certificate)
			if err != nil {
				logger.Error("parse leaf cert", "error", err)
				continue
			}
			leaf = parsed
		}
		candidates = append(candidates, candidate{
			cert:     match,
			notAfter: leaf.NotAfter,
		})
	}
	if len(candidates) == 0 {
		return ssl.Certificate{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].notAfter.After(candidates[j].notAfter)
	})
	for _, cand := range candidates {
		cert, err := MarshalCertificate(cand.cert)
		if err != nil {
			logger.Error("marshal cert", "error", err)
			continue
		}
		return cert, true
	}
	return ssl.Certificate{}, false
}
