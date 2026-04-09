package certmagicmgr

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/warpcondev/cuesix/internal/plugin/ssl"
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
	ChallengePort     int
}

// Manager owns certmagic configuration and serialized operations.
type Manager struct {
	cfg       Config
	adapter   CertMagic
	storage   Storage
	providers map[string]*Provider
	fallback  ssl.PEMCertificate
	logger    *slog.Logger
}

// NewManager builds a certmagic manager and validates configuration.
func NewManager(logger *slog.Logger, cfg Config, events chan ssl.Tracking, fallback ssl.PEMCertificate, adapter CertMagic, storage Storage) (Manager, error) {
	if len(cfg.Providers) == 0 {
		return Manager{}, errors.New("at least one provider is required")
	}
	if cfg.DataDir == "" {
		return Manager{}, errors.New("data dir is required")
	}
	if adapter == nil {
		adapter = certmagicAdapter{}
	}
	if storage == nil {
		storage = storageAdapter{
			storage: &certmagic.FileStorage{Path: cfg.DataDir},
		}
	}
	if logger == nil {
		logger = slog.Default()
	}
	providers := make(map[string]*Provider, len(cfg.Providers))
	for _, p := range cfg.Providers {
		if p.Name == "" || p.CA == "" || p.Email == "" {
			return Manager{}, fmt.Errorf("provider %q requires name, ca, and email", p.Name)
		}
		if _, exists := providers[p.Name]; exists {
			return Manager{}, fmt.Errorf("duplicate provider name: %s", p.Name)
		}
		providers[p.Name] = buildProvider(logger, cfg, p, adapter, storage, events)
	}
	return Manager{
		cfg:       cfg,
		adapter:   adapter,
		storage:   storage,
		providers: providers,
		fallback:  fallback,
		logger:    logger,
	}, nil
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
	return m.storage.CleanStorage(ctx, certmagic.CleanStorageOptions{
		Interval:               interval,
		ExpiredCerts:           true,
		ExpiredCertGracePeriod: gracePeriod,
	})
}
