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
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/warpcomdev/cuesix/internal/cursor"
	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
)

// ProviderConfig configures a single ACME provider.
type ProviderConfig struct {
	Name  string
	CA    string
	Email string
}

// Config describes the certmagic manager configuration.
type Config struct {
	Providers         []ProviderConfig
	DefaultProvider   string
	DataDir           string
	CertObtainTimeout time.Duration
}

type Provider struct {
	cursor.Lock
	cfg    ProviderConfig
	cache  *certmagic.Cache
	magic  *certmagic.Config
	logger *slog.Logger
}

// Manager owns certmagic configuration and serialized operations.
type Manager struct {
	cfg       Config
	storage   certmagic.Storage
	providers map[string]*Provider
	fallback  ssl.Certificate
	logger    *slog.Logger
}

// NewManager builds a certmagic manager and validates configuration.
func NewManager(logger *slog.Logger, cfg Config, events chan ssl.ACMEKey, fallback ssl.Certificate) (Manager, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if len(cfg.Providers) == 0 {
		return Manager{}, errors.New("at least one provider is required")
	}
	if cfg.DataDir == "" {
		return Manager{}, errors.New("data dir is required")
	}
	storage := &certmagic.FileStorage{Path: cfg.DataDir}
	providers := make(map[string]*Provider, len(cfg.Providers))
	for _, p := range cfg.Providers {
		if p.Name == "" || p.CA == "" || p.Email == "" {
			return Manager{}, fmt.Errorf("provider %q requires name, ca, and email", p.Name)
		}
		if _, exists := providers[p.Name]; exists {
			return Manager{}, fmt.Errorf("duplicate provider name: %s", p.Name)
		}
		providers[p.Name] = buildProvider(logger, cfg, p, storage, events)
	}
	return Manager{
		cfg:       cfg,
		storage:   storage,
		providers: providers,
		fallback:  fallback,
		logger:    logger,
	}, nil
}

// RunChallengeServer exposes the HTTP-01 challenge handler.
func (m Manager) ChallengeHandler() http.Handler {
	logger := m.logger
	return certmagic.DefaultACME.HTTPChallengeHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info("challenge request", "host", r.Host, "url", r.URL.String())
	}))
}

// Return the manager for a particular provider
func (m Manager) ResolveProvider(name string) (*Provider, error) {
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

// Remove expired certificates across all providers
func (m Manager) RemoveExpired(ctx context.Context, interval time.Duration, gracePeriod time.Duration) error {
	return certmagic.CleanStorage(ctx, m.storage, certmagic.CleanStorageOptions{
		Interval:               interval,
		ExpiredCerts:           true,
		ExpiredCertGracePeriod: gracePeriod,
	})
}

// RequestCertificate obtains or loads a certificate for the given SNI.
func (p *Provider) RequestCertificate(ctx context.Context, sni string) error {
	if p == nil {
		return errors.New("provider is nil")
	}
	if p.magic == nil {
		return errors.New("provider magic is nil")
	}
	var key ssl.ACMEKey
	if strings.TrimSpace(sni) == "" {
		return errors.New("sni is required")
	}
	key.Provider = p.Name()
	key.SNI = sni
	var err error
	p.WithLock(func() {
		err = p.magic.ManageAsync(ctx, []string{key.SNI})
	})
	return err
}

func (p *Provider) Name() string {
	if p == nil {
		return ""
	}
	return p.cfg.Name
}

func (p *Provider) BestMatchFor(sni string) (ssl.Certificate, bool) {
	if p == nil {
		return ssl.Certificate{}, false
	}
	if p.cache == nil {
		return ssl.Certificate{}, false
	}
	sni = strings.TrimSpace(sni)
	if sni == "" {
		return ssl.Certificate{}, false
	}
	var matches []certmagic.Certificate
	p.WithLock(func() {
		matches = append([]certmagic.Certificate(nil), p.cache.AllMatchingCertificates(sni)...)
	})
	return bestMatchForCandidates(matches, p.logger)
}

// RemoveManaged stops tracking the SNI for future listings.
func (p *Provider) RemoveManaged(sni string) {
	if p == nil || p.magic == nil || p.cache == nil {
		return
	}
	p.WithLock(func() {
		for _, issuer := range p.magic.Issuers {
			c := certmagic.SubjectIssuer{
				Subject:   sni,
				IssuerKey: issuer.IssuerKey(),
			}
			p.cache.RemoveManaged([]certmagic.SubjectIssuer{c})
		}
	})
}

func buildProvider(logger *slog.Logger, cfg Config, providerCfg ProviderConfig, storage certmagic.Storage, events chan ssl.ACMEKey) *Provider {
	if logger == nil {
		logger = slog.Default()
	}
	p := &Provider{
		cfg:    providerCfg,
		logger: logger,
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
					case events <- ssl.ACMEKey{SNI: sni, Provider: p.Name()}:
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
	if cfg.CertObtainTimeout > 0 {
		issuerCfg.CertObtainTimeout = cfg.CertObtainTimeout
	}
	issuer := certmagic.NewACMEIssuer(p.magic, issuerCfg)
	p.magic.Issuers = []certmagic.Issuer{issuer}
	return p
}

func MarshalCertificate(cert certmagic.Certificate) (ssl.Certificate, error) {
	if cert.PrivateKey == nil {
		return ssl.Certificate{}, errors.New("private key is nil")
	}
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
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		return ssl.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: key,
	})
	var notAfter time.Time
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
