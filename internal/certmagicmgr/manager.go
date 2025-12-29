package certmagicmgr

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strings"
	"sync"
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
	Providers       []ProviderConfig
	DefaultProvider string
	DataDir         string
	ChallengeAddr   string
	DefaultTimeout  time.Duration
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
	cfg    ProviderConfig
	magic  *certmagic.Config
	issuer *certmagic.ACMEIssuer
}

// Manager owns certmagic configuration and serialized operations.
type Manager struct {
	cfg       Config
	logger    *slog.Logger
	providers map[string]*provider
	handler   http.Handler

	mu      sync.RWMutex
	managed map[string]ManagedCert
	certMu  sync.Mutex
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
	providers := make(map[string]*provider, len(cfg.Providers))
	for _, p := range cfg.Providers {
		if p.Name == "" || p.CA == "" || p.Email == "" {
			return nil, fmt.Errorf("provider %q requires name, ca, and email", p.Name)
		}
		if _, exists := providers[p.Name]; exists {
			return nil, fmt.Errorf("duplicate provider name: %s", p.Name)
		}
		magic, issuer := buildProvider(cfg, p)
		providers[p.Name] = &provider{
			cfg:    p,
			magic:  magic,
			issuer: issuer,
		}
	}
	handler := buildChallengeHandler(providers)
	return &Manager{
		cfg:       cfg,
		logger:    logger,
		providers: providers,
		handler:   handler,
		managed:   make(map[string]ManagedCert),
	}, nil
}

// RunChallengeServer exposes the HTTP-01 challenge handler.
func (m *Manager) RunChallengeServer(ctx context.Context) error {
	if m.cfg.ChallengeAddr == "" {
		return errors.New("challenge address is required")
	}
	server := &http.Server{
		Addr:              m.cfg.ChallengeAddr,
		Handler:           m.handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		BaseContext: func(_ net.Listener) context.Context {
			return ctx
		},
	}

	errCh := make(chan error, 1)
	go func() {
		m.logger.Info("certmagic challenge server start", "addr", m.cfg.ChallengeAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			m.logger.Error("certmagic challenge server shutdown failed", "error", err)
		}
		return nil
	case err := <-errCh:
		return err
	}
}

// RequestCertificate obtains or loads a certificate for the given SNI.
func (m *Manager) RequestCertificate(ctx context.Context, providerName string, sni string) (Certificate, error) {
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

	m.certMu.Lock()
	defer m.certMu.Unlock()

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
	m.recordManaged(sni, p.cfg.Name, notAfter)
	return Certificate{CertPEM: certPEM, KeyPEM: keyPEM, NotAfter: notAfter}, nil
}

// ListManaged returns the managed certificate entries.
func (m *Manager) ListManaged() []ManagedCert {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]ManagedCert, 0, len(m.managed))
	for _, entry := range m.managed {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider == out[j].Provider {
			return out[i].SNI < out[j].SNI
		}
		return out[i].Provider < out[j].Provider
	})
	return out
}

// RemoveManaged stops tracking the SNI for future listings.
func (m *Manager) RemoveManaged(sni string) {
	m.mu.Lock()
	delete(m.managed, sni)
	m.mu.Unlock()
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

func (m *Manager) recordManaged(sni string, providerName string, notAfter time.Time) {
	m.mu.Lock()
	m.managed[sni] = ManagedCert{
		SNI:        sni,
		Provider:   providerName,
		NotAfter:   notAfter,
		ObtainedAt: time.Now().UTC(),
	}
	m.mu.Unlock()
}

func buildProvider(cfg Config, providerCfg ProviderConfig) (*certmagic.Config, *certmagic.ACMEIssuer) {
	storage := &certmagic.FileStorage{Path: cfg.DataDir}
	magic := certmagic.NewDefault()
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
	if cfg.ChallengeAddr != "" {
		if port := portFromAddr(cfg.ChallengeAddr); port > 0 {
			issuerCfg.AltHTTPPort = port
		}
	}
	issuer := certmagic.NewACMEIssuer(magic, issuerCfg)
	magic.Issuers = []certmagic.Issuer{issuer}
	return magic, issuer
}

func buildChallengeHandler(providers map[string]*provider) http.Handler {
	handler := http.NotFoundHandler()
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		handler = providers[name].issuer.HTTPChallengeHandler(handler)
	}
	return handler
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

func portFromAddr(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	parsed, err := net.LookupPort("tcp", port)
	if err != nil {
		return 0
	}
	return parsed
}
