package testutil

import (
	"context"
	"net/http"

	"github.com/caddyserver/certmagic"
)

type ManageAsyncCall struct {
	Ctx    context.Context
	Config *certmagic.Config
	SNIs   []string
}

type RemoveManagedCall struct {
	Cache   *certmagic.Cache
	Issuers []certmagic.SubjectIssuer
}

type AllMatchingCertificatesCall struct {
	Cache *certmagic.Cache
	SNI   string
}

type CleanStorageCall struct {
	Ctx  context.Context
	Opts certmagic.CleanStorageOptions
}

type UpdateConfigCall struct {
	Config *certmagic.Config
}

// MockCertMagic implements certmagicmgr.CertMagic for tests.
type MockCertMagic struct {
	HTTPChallengeHandlerFunc    func(http.Handler) http.Handler
	ManageAsyncFunc             func(context.Context, *certmagic.Config, []string) error
	RemoveManagedFunc           func(*certmagic.Cache, []certmagic.SubjectIssuer)
	AllMatchingCertificatesFunc func(*certmagic.Cache, string) []certmagic.Certificate

	ManageAsyncCalls             []ManageAsyncCall
	RemoveManagedCalls           []RemoveManagedCall
	AllMatchingCertificatesCalls []AllMatchingCertificatesCall
}

// HTTPChallengeHandler applies the configured HTTP challenge handler stub.
func (m *MockCertMagic) HTTPChallengeHandler(handler http.Handler) http.Handler {
	if m.HTTPChallengeHandlerFunc != nil {
		return m.HTTPChallengeHandlerFunc(handler)
	}
	return handler
}

// ManageAsync records a call and delegates to ManageAsyncFunc when configured.
func (m *MockCertMagic) ManageAsync(ctx context.Context, cfg *certmagic.Config, snis []string) error {
	m.ManageAsyncCalls = append(m.ManageAsyncCalls, ManageAsyncCall{
		Ctx:    ctx,
		Config: cfg,
		SNIs:   append([]string(nil), snis...),
	})
	if m.ManageAsyncFunc != nil {
		return m.ManageAsyncFunc(ctx, cfg, snis)
	}
	return nil
}

// RemoveManaged records a call and delegates to RemoveManagedFunc when configured.
func (m *MockCertMagic) RemoveManaged(cache *certmagic.Cache, issuers []certmagic.SubjectIssuer) {
	m.RemoveManagedCalls = append(m.RemoveManagedCalls, RemoveManagedCall{
		Cache:   cache,
		Issuers: append([]certmagic.SubjectIssuer(nil), issuers...),
	})
	if m.RemoveManagedFunc != nil {
		m.RemoveManagedFunc(cache, issuers)
	}
}

// AllMatchingCertificates records a call and returns configured certificates.
func (m *MockCertMagic) AllMatchingCertificates(cache *certmagic.Cache, sni string) []certmagic.Certificate {
	m.AllMatchingCertificatesCalls = append(m.AllMatchingCertificatesCalls, AllMatchingCertificatesCall{
		Cache: cache,
		SNI:   sni,
	})
	if m.AllMatchingCertificatesFunc != nil {
		return m.AllMatchingCertificatesFunc(cache, sni)
	}
	return nil
}

// MockStorage implements certmagicmgr.Storage for tests.
type MockStorage struct {
	CleanStorageFunc func(context.Context, certmagic.CleanStorageOptions) error
	UpdateConfigFunc func(*certmagic.Config)

	CleanStorageCalls []CleanStorageCall
	UpdateConfigCalls []UpdateConfigCall
}

// CleanStorage records a call and delegates to CleanStorageFunc when configured.
func (m *MockStorage) CleanStorage(ctx context.Context, opts certmagic.CleanStorageOptions) error {
	m.CleanStorageCalls = append(m.CleanStorageCalls, CleanStorageCall{
		Ctx:  ctx,
		Opts: opts,
	})
	if m.CleanStorageFunc != nil {
		return m.CleanStorageFunc(ctx, opts)
	}
	return nil
}

// UpdateConfig records a call and delegates to UpdateConfigFunc when configured.
func (m *MockStorage) UpdateConfig(cfg *certmagic.Config) {
	m.UpdateConfigCalls = append(m.UpdateConfigCalls, UpdateConfigCall{
		Config: cfg,
	})
	if m.UpdateConfigFunc != nil {
		m.UpdateConfigFunc(cfg)
	}
}
