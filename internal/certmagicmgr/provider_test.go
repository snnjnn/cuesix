package certmagicmgr

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/warpcomdev/sixpack/internal/testutil"
)

type stubIssuer struct {
	key string
}

func (s stubIssuer) Issue(context.Context, *x509.CertificateRequest) (*certmagic.IssuedCertificate, error) {
	return nil, nil
}

func (s stubIssuer) IssuerKey() string {
	return s.key
}

func TestProviderRequestCertificateValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		provider *Provider
		sni      string
		wantErr  string
	}{
		{name: "nil provider", provider: nil, sni: "example.com", wantErr: "provider is nil"},
		{name: "nil magic", provider: &Provider{}, sni: "example.com", wantErr: "provider magic is nil"},
		{name: "empty sni", provider: &Provider{magic: &certmagic.Config{}}, sni: "   ", wantErr: "sni is required"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.provider.RequestCertificate(context.Background(), tt.sni)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestProviderRequestCertificateCallsAdapter(t *testing.T) {
	t.Parallel()
	adapter := &testutil.MockCertMagic{
		ManageAsyncFunc: func(ctx context.Context, cfg *certmagic.Config, snis []string) error {
			return errors.New("boom")
		},
	}
	p := &Provider{
		adapter: adapter,
		magic:   &certmagic.Config{},
	}
	err := p.RequestCertificate(context.Background(), "example.com")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected adapter error, got %v", err)
	}
	if got := len(adapter.ManageAsyncCalls); got != 1 {
		t.Fatalf("expected one ManageAsync call, got %d", got)
	}
	call := adapter.ManageAsyncCalls[0]
	if call.Config != p.magic {
		t.Fatalf("expected magic config passed through")
	}
	if want := []string{"example.com"}; len(call.SNIs) != len(want) || call.SNIs[0] != want[0] {
		t.Fatalf("unexpected SNIs: %v", call.SNIs)
	}
}

func TestProviderBestMatchFor(t *testing.T) {
	t.Parallel()
	now := time.Now()
	newer := testutil.MakeCert(t, now.Add(2*time.Hour), "example.com")
	older := testutil.MakeCert(t, now.Add(time.Hour), "example.com")
	adapter := &testutil.MockCertMagic{
		AllMatchingCertificatesFunc: func(cache *certmagic.Cache, sni string) []certmagic.Certificate {
			if sni != "example.com" {
				t.Fatalf("expected trimmed sni, got %s", sni)
			}
			return []certmagic.Certificate{older, newer}
		},
	}
	p := &Provider{
		adapter: adapter,
		cache:   &certmagic.Cache{},
		logger:  testutil.Logger(),
	}
	wrap, ok := p.BestMatchFor(context.Background(), "  example.com ")
	if !ok {
		t.Fatalf("expected a match")
	}
	if !wrap.NotAfterTime().Equal(newer.Leaf.NotAfter) {
		t.Fatalf("expected newer certificate, got %s", wrap.NotAfterTime())
	}
}

func TestProviderBestMatchForEdgeCases(t *testing.T) {
	t.Parallel()
	adapter := &testutil.MockCertMagic{
		AllMatchingCertificatesFunc: func(_ *certmagic.Cache, _ string) []certmagic.Certificate {
			return []certmagic.Certificate{{}}
		},
	}
	p := &Provider{
		adapter: adapter,
		cache:   &certmagic.Cache{},
		logger:  testutil.Logger(),
	}
	if wrap, ok := p.BestMatchFor(context.Background(), ""); ok || wrap != nil {
		t.Fatalf("expected empty result when sni empty")
	}
	if wrap, ok := p.BestMatchFor(context.Background(), "example.com"); ok || wrap != nil {
		t.Fatalf("expected no match for empty candidates")
	}
	var nilProvider *Provider
	if _, ok := nilProvider.BestMatchFor(context.Background(), "example.com"); ok {
		t.Fatalf("nil provider should return false")
	}
}

func TestProviderName(t *testing.T) {
	t.Parallel()
	var nilProvider *Provider
	if name := nilProvider.Name(); name != "" {
		t.Fatalf("expected empty name for nil provider, got %q", name)
	}
	p := &Provider{cfg: ProviderConfig{Name: "foo"}}
	if name := p.Name(); name != "foo" {
		t.Fatalf("expected name foo, got %s", name)
	}
}

func TestProviderRemoveManagedNoop(t *testing.T) {
	t.Parallel()
	var nilProvider *Provider
	nilProvider.RemoveManaged(context.Background(), "example.com")
	p := &Provider{}
	p.RemoveManaged(context.Background(), "example.com")
}

func TestProviderRemoveManaged(t *testing.T) {
	t.Parallel()
	adapter := &testutil.MockCertMagic{}
	p := &Provider{
		adapter: adapter,
		cache:   &certmagic.Cache{},
		magic: &certmagic.Config{
			Issuers: []certmagic.Issuer{stubIssuer{key: "k1"}, stubIssuer{key: "k2"}},
		},
	}
	p.RemoveManaged(context.Background(), "example.com")
	if got := len(adapter.RemoveManagedCalls); got != 2 {
		t.Fatalf("expected one call per issuer, got %d", got)
	}
	expected := map[string]bool{"k1": false, "k2": false}
	for _, call := range adapter.RemoveManagedCalls {
		if call.Cache != p.cache {
			t.Fatalf("expected cache passed through")
		}
		if len(call.Issuers) != 1 {
			t.Fatalf("expected one subject issuer, got %d", len(call.Issuers))
		}
		issuer := call.Issuers[0]
		expected[issuer.IssuerKey] = true
		if issuer.Subject != "example.com" {
			t.Fatalf("expected subject to match sni, got %s", issuer.Subject)
		}
	}
	for k, seen := range expected {
		if !seen {
			t.Fatalf("issuer %s not used", k)
		}
	}
}

func TestMarshalCertificate(t *testing.T) {
	t.Parallel()
	now := time.Now()
	valid := testutil.MakeCert(t, now.Add(time.Hour))
	cert, err := MarshalCertificate(valid)
	if err != nil {
		t.Fatalf("MarshalCertificate returned error: %v", err)
	}
	if !cert.NotAfter.Equal(valid.Leaf.NotAfter) {
		t.Fatalf("unexpected NotAfter, got %s want %s", cert.NotAfter, valid.Leaf.NotAfter)
	}
	if len(cert.CertPEM) == 0 || len(cert.KeyPEM) == 0 {
		t.Fatalf("expected pem output")
	}

	noLeaf := testutil.MakeCertNoLeaf(t, now.Add(2*time.Hour))
	cert, err = MarshalCertificate(noLeaf)
	if err != nil {
		t.Fatalf("MarshalCertificate without leaf returned error: %v", err)
	}
	expectedNotAfter, err := parseLeaf(noLeaf.Certificate.Certificate)
	if err != nil {
		t.Fatalf("parseLeaf for expectation failed: %v", err)
	}
	if !cert.NotAfter.Equal(expectedNotAfter.NotAfter) {
		t.Fatalf("unexpected NotAfter, got %s want %s", cert.NotAfter, expectedNotAfter.NotAfter)
	}

	empty := certmagic.Certificate{}
	if _, err := MarshalCertificate(empty); err == nil {
		t.Fatalf("expected error for missing key")
	}
	empty.PrivateKey = valid.PrivateKey
	if _, err := MarshalCertificate(empty); err == nil {
		t.Fatalf("expected error for empty certificate chain")
	}
}

func TestLeafNotAfter(t *testing.T) {
	t.Parallel()
	now := time.Now()
	valid := testutil.MakeCert(t, now)
	na, err := leafNotAfter(valid.Certificate.Certificate)
	if err != nil {
		t.Fatalf("leafNotAfter returned error: %v", err)
	}
	if !na.Equal(valid.Leaf.NotAfter) {
		t.Fatalf("unexpected notAfter: %s vs %s", na, valid.Leaf.NotAfter)
	}
	if _, err := leafNotAfter(nil); err == nil {
		t.Fatalf("expected error for empty chain")
	}
}

func TestParseLeaf(t *testing.T) {
	t.Parallel()
	valid := testutil.MakeCert(t, time.Now())
	leaf, err := parseLeaf(valid.Certificate.Certificate)
	if err != nil {
		t.Fatalf("parseLeaf returned error: %v", err)
	}
	if leaf == nil {
		t.Fatalf("expected parsed certificate")
	}
	if _, err := parseLeaf(nil); err == nil {
		t.Fatalf("expected error for empty chain")
	}
}

func TestBestMatchForCandidates(t *testing.T) {
	t.Parallel()
	now := time.Now()
	latest := testutil.MakeCert(t, now.Add(3*time.Hour))
	stale := testutil.MakeCert(t, now.Add(time.Hour))
	broken := testutil.MakeCert(t, now.Add(2*time.Hour))
	broken.PrivateKey = nil // force marshal error
	invalidChain := certmagic.Certificate{
		Certificate: tls.Certificate{
			Certificate: [][]byte{[]byte("not a cert")},
		},
	}

	tests := []struct {
		name     string
		input    []certmagic.Certificate
		want     time.Time
		expected bool
	}{
		{
			name:     "prefers newest",
			input:    []certmagic.Certificate{stale, latest},
			want:     latest.Leaf.NotAfter,
			expected: true,
		},
		{
			name:     "skips invalid",
			input:    []certmagic.Certificate{broken, latest},
			want:     latest.Leaf.NotAfter,
			expected: true,
		},
		{
			name:     "parse leaf error yields no match",
			input:    []certmagic.Certificate{invalidChain},
			expected: false,
		},
		{
			name:     "no candidates",
			input:    nil,
			expected: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			wrap, ok := bestMatchForCandidates(tt.input, testutil.Logger())
			if ok != tt.expected {
				t.Fatalf("expected ok=%v, got %v", tt.expected, ok)
			}
			if tt.expected && !wrap.NotAfterTime().Equal(tt.want) {
				t.Fatalf("unexpected certificate NotAfter: %s want %s", wrap.NotAfterTime(), tt.want)
			}
		})
	}
}
