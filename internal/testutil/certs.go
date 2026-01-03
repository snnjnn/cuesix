package testutil

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"math/big"
	"testing"
	"time"

	"github.com/caddyserver/certmagic"
)

// MakeCert builds a certmagic.Certificate with the given expiration and a filled Leaf.
func MakeCert(t testing.TB, notAfter time.Time, dnsNames ...string) certmagic.Certificate {
	t.Helper()
	return buildCert(t, notAfter, true, dnsNames...)
}

// MakeCertNoLeaf builds a certmagic.Certificate without the Leaf field populated.
func MakeCertNoLeaf(t testing.TB, notAfter time.Time, dnsNames ...string) certmagic.Certificate {
	t.Helper()
	return buildCert(t, notAfter, false, dnsNames...)
}

func buildCert(t testing.TB, notAfter time.Time, includeLeaf bool, dnsNames ...string) certmagic.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		DNSNames:     dnsNames,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	tlsCert := tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
	}
	if includeLeaf {
		tlsCert.Leaf = leaf
	}
	return certmagic.Certificate{Certificate: tlsCert}
}
