package ssl

import (
	"context"
	"fmt"
)

type FallbackManager struct {
	Certificate PEMCertificate
}

// ResolveProvider returns the fallback certificate provider.
func (m FallbackManager) ResolveProvider(name string) (Provider, error) {
	if name != FallbackPrefix {
		return nil, fmt.Errorf("unknown fallback provider: %s", name)
	}
	return fallbackProvider{cert: m.Certificate}, nil
}

type fallbackProvider struct {
	cert PEMCertificate
}

// Name returns the provider name.
func (p fallbackProvider) Name() string {
	return FallbackPrefix
}

// BestMatchFor always returns the configured fallback certificate when present.
func (p fallbackProvider) BestMatchFor(_ context.Context, _ string) (Certificate, bool) {
	return p.cert, len(p.cert.CertPEM) > 0 && len(p.cert.KeyPEM) > 0
}

// RequestCertificate is a no-op for the static fallback provider.
func (p fallbackProvider) RequestCertificate(_ context.Context, _ string) error {
	return nil
}

// RemoveManaged is a no-op for the static fallback provider.
func (p fallbackProvider) RemoveManaged(_ context.Context, _ ...string) {
}
