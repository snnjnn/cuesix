package ssl

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
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
	plugin.Fallback = PEMCertificate{CertPEM: []byte("c")}
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
	fallback := PEMCertificate{CertPEM: []byte("fb-cert"), KeyPEM: []byte("fb-key")}
	dir := t.TempDir()
	fileFS := os.DirFS(dir)
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), certPEM, 0o644); err != nil {
		t.Fatalf("write cert file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "key.pem"), keyPEM, 0o644); err != nil {
		t.Fatalf("write key file: %v", err)
	}
	acmeCert := sslCert(time.Now().Add(2 * time.Hour))
	acmeProvider := &mockACMEProvider{
		name: ACMEPrefix + "p-acme",
		BestMatchForFunc: func(sni string) (Certificate, bool) {
			return acmeCert, true
		},
	}
	router := ProviderRouter{
		ACMEManager:     mockACMEManager{providers: map[string]Provider{acmeProvider.Name(): acmeProvider}},
		FileManager:     FileManager{Filesystems: []fs.FS{fileFS}},
		FallbackManager: FallbackManager{Certificate: fallback},
	}
	tracker, err := NewTracker(testutil.Logger(), router)
	if err != nil {
		t.Fatalf("NewTracker returned error: %v", err)
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
				"cert": "$secret://file/cert.pem",
				"key":  "$secret://file/key.pem",
				"snis": []string{"b.example"},
			},
			map[string]any{
				"id":   "3",
				"cert": "$secret://acme/p-acme",
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
		Fallback:    fallback,
		LiveHandler: LiveHandler{Tracker: tracker, RequestTimeout: time.Second},
		Logger:      testutil.Logger(),
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
	if len(record) != 2 {
		t.Fatalf("expected record updated for live targets, got %d entries", len(record))
	}
}

func TestSSLPluginLeavesInlineUntouched(t *testing.T) {
	t.Parallel()
	fallback := PEMCertificate{CertPEM: []byte("fb-cert"), KeyPEM: []byte("fb-key")}
	value := map[string]any{
		"ssls": []any{
			map[string]any{
				"id":   "1",
				"cert": "inline-cert",
				"key":  "inline-key",
				"snis": []any{"a.example"},
			},
		},
	}
	plugin := &SSLPlugin{
		Fallback:    fallback,
		LiveHandler: LiveHandler{},
		Logger:      testutil.Logger(),
	}
	out, err := plugin.Update(context.Background(), value, nil)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	entry := out["ssls"].([]any)[0].(map[string]any)
	if entry["cert"] != "inline-cert" || entry["key"] != "inline-key" {
		t.Fatalf("expected inline values unchanged: %v", entry)
	}
}

func TestLiveHandlerFileIdentityAndMismatchFallback(t *testing.T) {
	t.Parallel()
	now := time.Now().Add(time.Hour)
	fallback := PEMCertificate{CertPEM: []byte("fb-cert"), KeyPEM: []byte("fb-key")}
	fileCert := PEMCertificate{CertPEM: []byte("file-cert"), KeyPEM: []byte("file-key"), NotAfter: now}
	provider := &stubProvider{name: FileProviderName, best: fileCert}
	tracker := &stubLiveTracker{
		resolve: func(name string, _ *ProviderCache) (Provider, error) {
			if name == FileProviderName {
				return provider, nil
			}
			if name == FallbackPrefix {
				return fallbackProvider{cert: fallback}, nil
			}
			return nil, errors.New("unknown provider")
		},
		watch: func(buffer int, topic string) cursor.Owned[Delivery] {
			ch := make(chan Delivery, buffer)
			go func() {
				ch <- Delivery{
					Tracking:       Tracking{Provider: FileProviderName, Identity: "c.pem+k.pem"},
					PEMCertificate: fileCert,
				}
				close(ch)
			}()
			return cursor.Owned[Delivery]{Cursor: cursor.Channel(ch), Close: func() {}}
		},
	}
	// Valid file pair
	var certOut, keyOut string
	target := certTargets{
		sslId: "id-file",
		cert:  "$secret://file/c.pem",
		key:   "$secret://file/k.pem",
		snis:  []string{"example.com"},
		replace: func(c, k []byte) {
			certOut = string(c)
			keyOut = string(k)
		},
	}
	handler := LiveHandler{Tracker: tracker, RequestTimeout: time.Second}
	handler.replaceTargets(context.Background(), testutil.Logger(), []certTargets{target}, nil, fallback)
	if certOut != string(fileCert.CertPEM) || keyOut != string(fileCert.KeyPEM) {
		t.Fatalf("expected file provider replacement, got cert=%q key=%q", certOut, keyOut)
	}
	if len(provider.requested) != 1 || provider.requested[0] != "c.pem+k.pem" {
		t.Fatalf("unexpected file identity requested: %+v", provider.requested)
	}
	// Mismatched file/key should fallback
	certOut, keyOut = "", ""
	mismatch := certTargets{
		sslId: "id-mismatch",
		cert:  "$secret://file/only-cert.pem",
		key:   "inline-key",
		snis:  []string{"example.com"},
		replace: func(c, k []byte) {
			certOut = string(c)
			keyOut = string(k)
		},
	}
	handler.replaceTargets(context.Background(), testutil.Logger(), []certTargets{mismatch}, nil, fallback)
	if certOut != string(fallback.CertPEM) || keyOut != string(fallback.KeyPEM) {
		t.Fatalf("expected fallback for mismatched file target, got cert=%q key=%q", certOut, keyOut)
	}
}

func TestLiveHandlerAcmeMultipleSNIFallback(t *testing.T) {
	t.Parallel()
	fallback := PEMCertificate{CertPEM: []byte("fb-cert"), KeyPEM: []byte("fb-key")}
	tracker := &stubLiveTracker{
		resolve: func(name string, _ *ProviderCache) (Provider, error) {
			return fallbackProvider{cert: fallback}, nil
		},
		watch: func(buffer int, topic string) cursor.Owned[Delivery] {
			ch := make(chan Delivery, buffer)
			return cursor.Owned[Delivery]{Cursor: cursor.Channel(ch), Close: func() { close(ch) }}
		},
	}
	var certOut, keyOut string
	target := certTargets{
		sslId: "id-acme",
		cert:  "$secret://acme/p",
		key:   "ignored",
		snis:  []string{"a.example", "b.example"},
		replace: func(c, k []byte) {
			certOut = string(c)
			keyOut = string(k)
		},
	}
	handler := LiveHandler{Tracker: tracker, RequestTimeout: 5 * time.Millisecond}
	handler.replaceTargets(context.Background(), testutil.Logger(), []certTargets{target}, nil, fallback)
	if certOut != string(fallback.CertPEM) || keyOut != string(fallback.KeyPEM) {
		t.Fatalf("expected fallback for multiple SNI acme target, got cert=%q key=%q", certOut, keyOut)
	}
}

func TestFileProviderWithNestedFilesystems(t *testing.T) {
	t.Parallel()
	certPEM, keyPEM := pemCertKey(t, time.Now().Add(time.Hour))
	fs1 := fstest.MapFS{
		"other.txt": {Data: []byte("other")},
	}
	fs2 := fstest.MapFS{
		"customer1/cert.pem": {Data: certPEM},
		"customer1/key.pem":  {Data: keyPEM},
	}
	target := certTargets{
		cert: "$secret://file/customer1/cert.pem",
		key:  "$secret://file/customer1/key.pem",
		snis: []string{"example.com"},
	}
	tracking, err := target.tracking()
	if err != nil {
		t.Fatalf("tracking returned error: %v", err)
	}
	provider := fileProvider{filesystems: []fs.FS{fs1, fs2}}
	wrap, ok := provider.BestMatchFor(context.Background(), tracking.Identity)
	if !ok {
		t.Fatalf("expected file provider to load nested cert and key")
	}
	cert, err := wrap.PEM()
	if err != nil {
		t.Fatalf("wrap PEM returned error: %v", err)
	}
	if string(cert.CertPEM) != string(certPEM) || string(cert.KeyPEM) != string(keyPEM) {
		t.Fatalf("unexpected cert/key data: cert=%q key=%q", cert.CertPEM, cert.KeyPEM)
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
		"cert": "$secret://acme/p",
		"key":  "$secret://file/k",
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
		"certs": []any{"$secret://file/c", "plain"},
		"keys":  []any{"$secret://file/k", "plaink"},
	}
	p.collectEntryTargets(entryList, targets)
	if len(targets[fileTarget]) == 0 {
		t.Fatalf("expected file target from list")
	}
	if resolveTargetType("$secret://acme/x", "") != acmeTarget {
		t.Fatalf("resolveTargetType acme failed")
	}
	if resolveTargetType("$secret://file/x", "key") != fileTarget {
		t.Fatalf("resolveTargetType file cert failed")
	}
	if resolveTargetType("cert", "$secret://file/k") != fileTarget {
		t.Fatalf("resolveTargetType file key failed")
	}
	if resolveTargetType("cert", "key") != textTarget {
		t.Fatalf("resolveTargetType text failed")
	}
	if resolveTargetType("$secret://vault/kv", "key") != textTarget {
		t.Fatalf("resolveTargetType unknown secret failed")
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

type stubLiveTracker struct {
	resolve func(string, *ProviderCache) (Provider, error)
	watch   func(int, string) cursor.Owned[Delivery]
}

func (s *stubLiveTracker) ResolveProvider(name string, cache *ProviderCache) (Provider, error) {
	return s.resolve(name, cache)
}

func (s *stubLiveTracker) Watch(buffer int, topic string) cursor.Owned[Delivery] {
	return s.watch(buffer, topic)
}

type stubProvider struct {
	name      string
	best      PEMCertificate
	requested []string
}

func (p *stubProvider) Name() string { return p.name }

func (p *stubProvider) BestMatchFor(_ context.Context, identity string) (Certificate, bool) {
	return p.best, true
}

func (p *stubProvider) RequestCertificate(_ context.Context, identity string) error {
	p.requested = append(p.requested, identity)
	return nil
}

func (p *stubProvider) RemoveManaged(_ context.Context, _ ...string) {}
