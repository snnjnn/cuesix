package certmagicmgr

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"log/slog"
	"math/big"
	"testing"
	"time"

	"github.com/caddyserver/certmagic"
)

func TestBestMatchForCandidatesSkipsMarshalErrors(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	now := time.Now().UTC().Truncate(time.Second)
	newest := newTestCert(t, []string{"api.example.com"}, now.Add(48*time.Hour))
	newest.Certificate.PrivateKey = struct{}{}
	older := newTestCert(t, []string{"*.example.com"}, now.Add(12*time.Hour))

	cert, ok := bestMatchForCandidates([]certmagic.Certificate{newest, older}, logger)
	if !ok {
		t.Fatalf("expected match")
	}
	if !timeClose(cert.NotAfter, now.Add(12*time.Hour), time.Second) {
		t.Fatalf("expected fallback cert")
	}
}

func TestBestMatchForCandidatesPrefersNewestExpiration(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	now := time.Now().UTC().Truncate(time.Second)
	older := newTestCert(t, []string{"a.example.com"}, now.Add(12*time.Hour))
	newer := newTestCert(t, []string{"b.example.com"}, now.Add(48*time.Hour))

	cert, ok := bestMatchForCandidates([]certmagic.Certificate{older, newer}, logger)
	if !ok {
		t.Fatalf("expected match")
	}
	if !timeClose(cert.NotAfter, now.Add(48*time.Hour), time.Second) {
		t.Fatalf("expected newest cert")
	}
}

func newTestCert(t *testing.T, dnsNames []string, notAfter time.Time) certmagic.Certificate {
	t.Helper()
	if len(dnsNames) == 0 {
		t.Fatalf("dnsNames required")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		t.Fatalf("serial: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: dnsNames[0]},
		DNSNames:     dnsNames,
		NotBefore:    time.Now().UTC().Add(-time.Hour).Truncate(time.Second),
		NotAfter:     notAfter.UTC().Truncate(time.Second),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	return certmagic.Certificate{
		Certificate: tls.Certificate{
			Certificate: [][]byte{der},
			PrivateKey:  key,
		},
	}
}

func timeClose(got time.Time, want time.Time, delta time.Duration) bool {
	diff := got.Sub(want)
	if diff < 0 {
		diff = -diff
	}
	return diff <= delta
}
