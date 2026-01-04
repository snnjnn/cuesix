package ssl

import (
	"context"
	"time"

	"github.com/warpcomdev/cuesix/internal/cursor"
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
	WatchFunc              func(int, string) cursor.Owned[Delivery]

	requestCertificateCalls []requestCertificateCall
	watchCalls              []watchCall
}

func (m *mockACMETracker) RequestCertificate(ctx context.Context, providerName string, sni string) (Tracking, error) {
	m.requestCertificateCalls = append(m.requestCertificateCalls, requestCertificateCall{
		Ctx:      ctx,
		Provider: providerName,
		SNI:      sni,
	})
	if m.RequestCertificateFunc != nil {
		return m.RequestCertificateFunc(ctx, providerName, sni)
	}
	return Tracking{Provider: providerName, Identity: sni}, nil
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
	SNI string
}

// mockACMEProvider implements ACMEProvider for tests.
type mockACMEProvider struct {
	name                   string
	BestMatchForFunc       func(string) (Certificate, bool)
	RequestCertificateFunc func(context.Context, string) error
	RemoveManagedFunc      func(string)

	bestMatchCalls     []bestMatchCall
	requestCalls       []requestCall
	removeManagedCalls []removeManagedCall
}

func (m *mockACMEProvider) Name() string {
	return m.name
}

func (m *mockACMEProvider) BestMatchFor(sni string) (Certificate, bool) {
	m.bestMatchCalls = append(m.bestMatchCalls, bestMatchCall{SNI: sni})
	if m.BestMatchForFunc != nil {
		return m.BestMatchForFunc(sni)
	}
	return Certificate{}, false
}

func (m *mockACMEProvider) RequestCertificate(ctx context.Context, sni string) error {
	m.requestCalls = append(m.requestCalls, requestCall{Ctx: ctx, SNI: sni})
	if m.RequestCertificateFunc != nil {
		return m.RequestCertificateFunc(ctx, sni)
	}
	return nil
}

func (m *mockACMEProvider) RemoveManaged(sni string) {
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
func sslCert(notAfter time.Time) Certificate {
	return Certificate{
		CertPEM:  []byte("cert"),
		KeyPEM:   []byte("key"),
		NotAfter: notAfter,
	}
}
