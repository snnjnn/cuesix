package ssl

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/internal/cursor"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

func TestSSLPluginUpdateValidation(t *testing.T) {
	t.Parallel()
	plugin := &SSLPlugin{}
	if _, err := plugin.Update(context.Background(), nil, nil); err == nil {
		t.Fatalf("expected error for nil fallback")
	}
	plugin.Fallback = Certificate{CertPEM: []byte("c")}
	if _, err := plugin.Update(context.Background(), nil, nil); err == nil {
		t.Fatalf("expected error for missing key")
	}
	plugin.Fallback.KeyPEM = []byte("k")
	if _, err := plugin.Update(context.Background(), nil, nil); err == nil {
		t.Fatalf("expected error for nil value map")
	}
	value := map[string]any{}
	if _, err := plugin.Update(context.Background(), value, nil); err != nil {
		t.Fatalf("unexpected error for missing ssls: %v", err)
	}
	if _, err := plugin.Update(context.Background(), map[string]any{"ssls": "not list"}, nil); err == nil {
		t.Fatalf("expected error for invalid ssls type")
	}
	if _, err := plugin.Update(context.Background(), map[string]any{"ssls": []any{}}, nil); err != nil {
		t.Fatalf("unexpected error for empty ssls: %v", err)
	}
}

func TestSSLPluginUpdateReplacesTargets(t *testing.T) {
	t.Parallel()
	certPEM, keyPEM := pemCertKey(t, time.Now().Add(time.Hour))
	fallback := Certificate{CertPEM: []byte("fb-cert"), KeyPEM: []byte("fb-key")}
	dir := t.TempDir()
	fileFS := os.DirFS(dir)
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0o644); err != nil {
		t.Fatalf("write cert file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), keyPEM, 0o644); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	acmeCert := sslCert(time.Now().Add(2 * time.Hour))
	tracker := &mockACMETracker{
		RequestCertificateFunc: func(ctx context.Context, provider, sni string) (Tracking, error) {
			return Tracking{Provider: provider, Identity: sni}, nil
		},
		WatchFunc: func(buffer int, topic string) cursor.Owned[Delivery] {
			ch := make(chan Delivery, buffer)
			ch <- Delivery{Tracking: Tracking{Provider: ACMEPrefix + "p-acme", Identity: "acme.example"}, Certificate: acmeCert}
			close(ch)
			return cursor.Owned[Delivery]{Cursor: cursor.Channel(ch), Close: func() {}}
		},
	}
	value := map[string]any{
		"ssls": []any{
			map[string]any{
				"id":   "1",
				"cert": "inline-cert",
				"key":  "inline-key",
				"snis": []any{"a.example"},
			},
			map[string]any{
				"id":   "2",
				"cert": "file://cert.pem",
				"key":  "file://key.pem",
				"snis": []string{"b.example"},
			},
			map[string]any{
				"id":   "3",
				"cert": "acme://p-acme",
				"key":  "ignored",
				"snis": []string{"acme.example"},
			},
			map[string]any{
				"id":    4,
				"certs": []any{"inline-cert-1", "inline-cert-2"},
				"keys":  []any{"inline-key-1", "inline-key-2"},
				"snis":  []any{"list.example"},
			},
		},
	}
	record := make(map[Tracking]time.Time)
	plugin := &SSLPlugin{
		Fallback: fallback,
		FileHandler: FileHandler{
			Filesystems: []fs.FS{fileFS},
		},
		LiveHandler: LiveHandler{
			Tracker: tracker,
		},
		Logger: testutil.Logger(),
	}
	out, err := plugin.Update(context.Background(), value, record)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	entries := out["ssls"].([]any)
	fileEntry := entries[1].(map[string]any)
	if fileEntry["cert"] != string(certPEM) || fileEntry["key"] != string(keyPEM) {
		t.Fatalf("file replacement failed: %v", fileEntry)
	}
	acmeEntry := entries[2].(map[string]any)
	if acmeEntry["cert"] != string(acmeCert.CertPEM) || acmeEntry["key"] != string(acmeCert.KeyPEM) {
		t.Fatalf("acme replacement failed: %v", acmeEntry)
	}
	listEntry := entries[3].(map[string]any)
	certs, _ := plugin.certPairs(listEntry)
	if len(certs) != 2 || listEntry["certs"].([]string)[0] != "inline-cert-1" {
		t.Fatalf("list pairs not preserved: %v", listEntry)
	}
	if len(tracker.requestCertificateCalls) != 1 {
		t.Fatalf("expected acme tracker request")
	}
}

func TestAsStringSlice(t *testing.T) {
	t.Parallel()
	if got := asStringSlice([]string{" a ", "b"}); got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected trim result %v", got)
	}
	if got := asStringSlice([]any{"a", "b"}); len(got) != 2 {
		t.Fatalf("unexpected conversion %v", got)
	}
	if got := asStringSlice([]any{"a", 1}); got != nil {
		t.Fatalf("expected nil for invalid type")
	}
	if got := asStringSlice("no slice"); got != nil {
		t.Fatalf("expected nil for non slice input")
	}
}

func TestCertPairs(t *testing.T) {
	t.Parallel()
	p := &SSLPlugin{}
	if certs, keys := p.certPairs(nil); certs != nil || keys != nil {
		t.Fatalf("expected nil for nil entry")
	}
	entry := map[string]any{"certs": []any{"c1", "c2"}, "keys": []any{"k1", "k2"}}
	certs, keys := p.certPairs(entry)
	if len(certs) != 2 || len(keys) != 2 {
		t.Fatalf("unexpected pairs: %v %v", certs, keys)
	}
	entry["keys"] = []any{"k1"}
	if certs, keys := p.certPairs(entry); certs != nil || keys != nil {
		t.Fatalf("expected nil for mismatched lengths")
	}
}

func TestCollectTargetsErrors(t *testing.T) {
	t.Parallel()
	p := &SSLPlugin{}
	_, err := p.collectTargets([]any{"not map"})
	if err == nil {
		t.Fatalf("expected error for invalid entry type")
	}
}

func TestCollectEntryTargetsAndResolve(t *testing.T) {
	t.Parallel()
	p := &SSLPlugin{}
	targets := map[targetType][]certTargets{
		textTarget: {},
		fileTarget: {},
		acmeTarget: {},
	}
	entry := map[string]any{
		"id":   123,
		"cert": "acme://p",
		"key":  "file://k",
		"snis": []any{"a", "a", "b"},
	}
	p.collectEntryTargets(entry, targets)
	if len(targets[acmeTarget]) != 1 {
		t.Fatalf("expected acme target")
	}
	if got := p.entrySNIs(entry); len(got) != 2 {
		t.Fatalf("expected deduped snis, got %v", got)
	}
	// list pairs
	entryList := map[string]any{
		"id":    "x",
		"certs": []any{"file://c", "plain"},
		"keys":  []any{"file://k", "plaink"},
	}
	p.collectEntryTargets(entryList, targets)
	if len(targets[fileTarget]) == 0 {
		t.Fatalf("expected file target from list")
	}
	if resolveTargetType("acme://x", "") != acmeTarget {
		t.Fatalf("resolveTargetType acme failed")
	}
	if resolveTargetType("file://x", "key") != fileTarget {
		t.Fatalf("resolveTargetType file cert failed")
	}
	if resolveTargetType("cert", "file://k") != fileTarget {
		t.Fatalf("resolveTargetType file key failed")
	}
	if resolveTargetType("cert", "key") != textTarget {
		t.Fatalf("resolveTargetType text failed")
	}
}

func TestLoadFallbackCertificate(t *testing.T) {
	t.Parallel()
	certPEM, keyPEM := pemCertKey(t, time.Now().Add(time.Hour))
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o644); err != nil {
		t.Fatalf("write key: %v", err)
	}
	cert, err := LoadFallbackCertificate(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadFallbackCertificate returned error: %v", err)
	}
	if cert.NotAfter.IsZero() {
		t.Fatalf("expected notAfter populated")
	}
	if _, err := LoadFallbackCertificate("missing", keyPath); err == nil {
		t.Fatalf("expected error for missing cert")
	}
	if err := os.WriteFile(certPath, []byte{}, 0o644); err != nil {
		t.Fatalf("write empty cert: %v", err)
	}
	if _, err := LoadFallbackCertificate(certPath, keyPath); err == nil {
		t.Fatalf("expected error for empty cert")
	}
}

func TestParseCertNotAfter(t *testing.T) {
	t.Parallel()
	certPEM, _ := pemCertKey(t, time.Now().Add(time.Hour))
	na, err := parseCertNotAfter(certPEM)
	if err != nil {
		t.Fatalf("parseCertNotAfter returned error: %v", err)
	}
	if na.IsZero() {
		t.Fatalf("expected notAfter populated")
	}
	if _, err := parseCertNotAfter([]byte("no cert")); err == nil {
		t.Fatalf("expected error for missing block")
	}
}

func pemCertKey(t *testing.T, notAfter time.Time) ([]byte, []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(time.Now().UnixNano()),
		Subject:               pkix.Name{CommonName: "example"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              notAfter,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyBytes})
	return certPEM, keyPEM
}
