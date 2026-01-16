package ssl

import (
	"context"
	"fmt"
)

type FallbackManager struct {
	Certificate PEMCertificate
}

func (m FallbackManager) ResolveProvider(name string) (Provider, error) {
	if name != FallbackPrefix {
		return nil, fmt.Errorf("unknown fallback provider: %s", name)
	}
	return fallbackProvider{cert: m.Certificate}, nil
}

type fallbackProvider struct {
	cert PEMCertificate
}

func (p fallbackProvider) Name() string {
	return FallbackPrefix
}

func (p fallbackProvider) BestMatchFor(_ context.Context, _ string) (Certificate, bool) {
	return p.cert, len(p.cert.CertPEM) > 0 && len(p.cert.KeyPEM) > 0
}

func (p fallbackProvider) RequestCertificate(_ context.Context, _ string) error {
	return nil
}

func (p fallbackProvider) RemoveManaged(_ context.Context, _ ...string) {
}
