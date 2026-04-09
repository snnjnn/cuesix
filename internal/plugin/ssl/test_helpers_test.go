package ssl

import (
	"context"
	"time"

	"github.com/warpcondev/cuesix/internal/cursor"
)

type requestCertificateCall struct {
	Ctx      context.Context
	Provider string
	SNI      string
}

type watchCall struct {
	Buffer int
	Topic  string
}

// mockACMETracker implements ACMETracker for tests.
type mockACMETracker struct {
	RequestCertificateFunc func(context.Context, string, string) (Tracking, error)
	BestMatchForFunc       func(context.Context, string, string) (Certificate, bool)
	WatchFunc              func(int, string) cursor.Owned[Delivery]

	requestCertificateCalls []requestCertificateCall
	watchCalls              []watchCall
}

type mockTrackedProvider struct {
	name string
	*mockACMETracker
}

func (m *mockACMETracker) ResolveProvider(provider string, _ *ProviderCache) (Provider, error) {
	return &mockTrackedProvider{
		mockACMETracker: m,
		name:            provider,
	}, nil
}

func (m *mockTrackedProvider) RequestCertificate(ctx context.Context, sni string) error {
	m.requestCertificateCalls = append(m.requestCertificateCalls, requestCertificateCall{
		Ctx:      ctx,
		Provider: m.name,
		SNI:      sni,
	})
	if m.RequestCertificateFunc != nil {
		_, err := m.RequestCertificateFunc(ctx, m.name, sni)
		return err
	}
	return nil
}

func (m *mockTrackedProvider) Name() string {
	return m.name
}

func (m *mockTrackedProvider) BestMatchFor(ctx context.Context, identity string) (Certificate, bool) {
	if m.BestMatchForFunc != nil {
		return m.BestMatchForFunc(ctx, m.name, identity)
	}
	return PEMCertificate{}, false
}

func (m *mockTrackedProvider) RemoveManaged(_ context.Context, _ ...string) {
}

func (m *mockACMETracker) Watch(buffer int, topic string) cursor.Owned[Delivery] {
	m.watchCalls = append(m.watchCalls, watchCall{Buffer: buffer, Topic: topic})
	if m.WatchFunc != nil {
		return m.WatchFunc(buffer, topic)
	}
	ch := make(chan Delivery, buffer)
	return cursor.Owned[Delivery]{
		Cursor: cursor.Channel(ch),
		Close: func() {
			close(ch)
		},
	}
}

type bestMatchCall struct {
	SNI string
}

type requestCall struct {
	Ctx context.Context
	SNI string
}

type removeManagedCall struct {
	SNI []string
}

// mockACMEProvider implements ACMEProvider for tests.
type mockACMEProvider struct {
	name                   string
	BestMatchForFunc       func(string) (Certificate, bool)
	RequestCertificateFunc func(context.Context, string) error
	RemoveManagedFunc      func([]string)

	bestMatchCalls     []bestMatchCall
	requestCalls       []requestCall
	removeManagedCalls []removeManagedCall
}

func (m *mockACMEProvider) Name() string {
	return m.name
}

func (m *mockACMEProvider) BestMatchFor(_ context.Context, sni string) (Certificate, bool) {
	m.bestMatchCalls = append(m.bestMatchCalls, bestMatchCall{SNI: sni})
	if m.BestMatchForFunc != nil {
		return m.BestMatchForFunc(sni)
	}
	return PEMCertificate{}, false
}

func (m *mockACMEProvider) RequestCertificate(ctx context.Context, sni string) error {
	m.requestCalls = append(m.requestCalls, requestCall{Ctx: ctx, SNI: sni})
	if m.RequestCertificateFunc != nil {
		return m.RequestCertificateFunc(ctx, sni)
	}
	return nil
}

func (m *mockACMEProvider) RemoveManaged(ctx context.Context, sni ...string) {
	m.removeManagedCalls = append(m.removeManagedCalls, removeManagedCall{SNI: sni})
	if m.RemoveManagedFunc != nil {
		m.RemoveManagedFunc(sni)
	}
}

// mockACMEManager resolves providers from a map.
type mockACMEManager struct {
	providers map[string]Provider
	err       error
}

func (m mockACMEManager) ResolveProvider(name string) (Provider, error) {
	if p, ok := m.providers[name]; ok {
		return p, nil
	}
	if m.err != nil {
		return nil, m.err
	}
	return nil, context.DeadlineExceeded
}

// sslCert builds a minimal ssl.Certificate with expiry set.
func sslCert(notAfter time.Time) PEMCertificate {
	return PEMCertificate{
		CertPEM:  []byte("cert"),
		KeyPEM:   []byte("key"),
		NotAfter: notAfter,
	}
}
