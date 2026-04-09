package certmagicmgr

import (
	"context"

	"github.com/caddyserver/certmagic"
)

// Certmagic interface encapsulates most of the behaviour of certmagic
// used by this module, to make it easier to mock and test the module.
type CertMagic interface {
	ManageAsync(ctx context.Context, config *certmagic.Config, snis []string) error
	RemoveManaged(cache *certmagic.Cache, issuers []certmagic.SubjectIssuer)
	AllMatchingCertificates(cache *certmagic.Cache, sni string) []certmagic.Certificate
}

// Storage interface encapsulates most of the behaviour of certmagic storage
// used by this module, to make it easier to mock and test the module.
type Storage interface {
	CleanStorage(ctx context.Context, opts certmagic.CleanStorageOptions) error
	UpdateConfig(cfg *certmagic.Config)
}

type certmagicAdapter struct{}

// ManageAsync forwards certificate management to certmagic.Config.
func (certmagicAdapter) ManageAsync(ctx context.Context, config *certmagic.Config, snis []string) error {
	return config.ManageAsync(ctx, snis)
}

// RemoveManaged forwards subject removal to certmagic.Cache.
func (certmagicAdapter) RemoveManaged(cache *certmagic.Cache, issuers []certmagic.SubjectIssuer) {
	cache.RemoveManaged(issuers)
}

// AllMatchingCertificates returns cache matches for the requested SNI.
func (certmagicAdapter) AllMatchingCertificates(cache *certmagic.Cache, sni string) []certmagic.Certificate {
	return cache.AllMatchingCertificates(sni)

}

type storageAdapter struct {
	storage certmagic.Storage
}

// CleanStorage runs certmagic storage cleanup with the wrapped storage backend.
func (s storageAdapter) CleanStorage(ctx context.Context, opts certmagic.CleanStorageOptions) error {
	return certmagic.CleanStorage(ctx, s.storage, opts)
}

// UpdateConfig injects the wrapped storage into the certmagic config.
func (s storageAdapter) UpdateConfig(cfg *certmagic.Config) {
	cfg.Storage = s.storage
}
