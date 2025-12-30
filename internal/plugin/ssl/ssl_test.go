package ssl

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/warpcomdev/cuesix/internal/certmagicmgr"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

func TestSSLPluginReplacesFields(t *testing.T) {
	fsOne := fstest.MapFS{
		"cert.pem":   {Data: []byte("cert-data")},
		"key.pem":    {Data: []byte("key-data")},
		"chain.pem":  {Data: []byte("chain-data")},
		"client.pem": {Data: []byte("client-data")},
	}
	fallback := certmagicmgr.Certificate{
		CertPEM:  []byte("fallback-cert"),
		KeyPEM:   []byte("fallback-key"),
		NotAfter: time.Now().Add(24 * time.Hour),
	}
	plugin := &SSLPlugin{
		Filesystems: []fs.FS{fsOne},
		Fallback:    fallback,
	}

	input := map[string]any{
		"ssls": []any{
			map[string]any{
				"id":    "1",
				"cert":  "file://cert.pem",
				"key":   "file://key.pem",
				"certs": []string{"file://chain.pem"},
				"keys":  []string{"inline-key"},
				"client": map[string]any{
					"ca": "file://client.pem",
				},
			},
		},
	}

	got, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	want := map[string]any{
		"ssls": []any{
			map[string]any{
				"id":    "1",
				"cert":  "cert-data",
				"key":   "key-data",
				"certs": []string{"chain-data"},
				"keys":  []string{"inline-key"},
				"client": map[string]any{
					"ca": "file://client.pem",
				},
			},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("unexpected result (-want +got):\n%s", diff)
	}
}

func TestSSLPluginMissingFile(t *testing.T) {
	fallback := certmagicmgr.Certificate{
		CertPEM:  []byte("fallback-cert"),
		KeyPEM:   []byte("fallback-key"),
		NotAfter: time.Now().Add(24 * time.Hour),
	}
	plugin := &SSLPlugin{
		Filesystems: []fs.FS{fstest.MapFS{}},
		Fallback:    fallback,
	}
	input := map[string]any{
		"ssls": []any{
			map[string]any{
				"cert": "file://missing.pem",
				"key":  "whatever",
			},
		},
	}
	got, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	entry := got["ssls"].([]any)[0].(map[string]any)
	if entry["cert"] != "fallback-cert" {
		t.Fatalf("expected fallback cert")
	}
	if entry["key"] != "whatever" {
		t.Fatalf("expected original key to remain")
	}
}

func TestSSLPluginACMEFallbackOnRequestError(t *testing.T) {
	fallback := certmagicmgr.Certificate{
		CertPEM:  []byte("fallback-cert"),
		KeyPEM:   []byte("fallback-key"),
		NotAfter: time.Now().Add(24 * time.Hour),
	}
	plugin := &SSLPlugin{
		Filesystems: []fs.FS{fstest.MapFS{}},
		ACME:        &fakeACME{err: errors.New("acme failed")},
		Fallback:    fallback,
	}
	input := map[string]any{
		"ssls": []any{
			map[string]any{
				"cert": "acme://letsencrypt",
				"key":  "file://ignored.pem",
				"snis": []any{"example.com"},
			},
		},
	}
	got, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	entry := got["ssls"].([]any)[0].(map[string]any)
	if entry["cert"] != "fallback-cert" {
		t.Fatalf("expected fallback cert")
	}
	if entry["key"] != "fallback-key" {
		t.Fatalf("expected fallback key")
	}
}

func TestSSLPluginACMESuccess(t *testing.T) {
	fallback := certmagicmgr.Certificate{
		CertPEM:  []byte("fallback-cert"),
		KeyPEM:   []byte("fallback-key"),
		NotAfter: time.Now().Add(24 * time.Hour),
	}
	acmeCert := certmagicmgr.Certificate{
		CertPEM:  []byte("acme-cert"),
		KeyPEM:   []byte("acme-key"),
		NotAfter: time.Now().Add(24 * time.Hour),
	}
	plugin := &SSLPlugin{
		Filesystems: []fs.FS{fstest.MapFS{}},
		ACME:        &fakeACME{notifyCert: acmeCert},
		Fallback:    fallback,
	}
	input := map[string]any{
		"ssls": []any{
			map[string]any{
				"cert": "acme://letsencrypt",
				"key":  "file://ignored.pem",
				"snis": []any{"example.com"},
			},
		},
	}
	got, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	entry := got["ssls"].([]any)[0].(map[string]any)
	if entry["cert"] != "acme-cert" {
		t.Fatalf("expected acme cert")
	}
	if entry["key"] != "acme-key" {
		t.Fatalf("expected acme key")
	}
}

func TestSSLPluginACMEInvalidSNIUsesFallback(t *testing.T) {
	fallback := certmagicmgr.Certificate{
		CertPEM:  []byte("fallback-cert"),
		KeyPEM:   []byte("fallback-key"),
		NotAfter: time.Now().Add(24 * time.Hour),
	}
	plugin := &SSLPlugin{
		Filesystems: []fs.FS{fstest.MapFS{}},
		ACME:        &fakeACME{notifyCert: fallback},
		Fallback:    fallback,
	}
	input := map[string]any{
		"ssls": []any{
			map[string]any{
				"cert": "acme://letsencrypt",
				"key":  "file://ignored.pem",
				"snis": []any{"example.com", "example.org"},
			},
		},
	}
	got, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	entry := got["ssls"].([]any)[0].(map[string]any)
	if entry["cert"] != "fallback-cert" {
		t.Fatalf("expected fallback cert")
	}
	if entry["key"] != "fallback-key" {
		t.Fatalf("expected fallback key")
	}
}

func TestSSLPluginLeavesInvalidCertKeyUntouched(t *testing.T) {
	fallback := certmagicmgr.Certificate{
		CertPEM:  []byte("fallback-cert"),
		KeyPEM:   []byte("fallback-key"),
		NotAfter: time.Now().Add(24 * time.Hour),
	}
	plugin := &SSLPlugin{
		Filesystems: []fs.FS{fstest.MapFS{}},
		Fallback:    fallback,
	}
	input := map[string]any{
		"ssls": []any{
			map[string]any{
				"cert": 123,
				"key":  "literal-key",
			},
		},
	}
	got, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	entry := got["ssls"].([]any)[0].(map[string]any)
	if entry["cert"] != 123 {
		t.Fatalf("expected cert to remain untouched")
	}
	if entry["key"] != "literal-key" {
		t.Fatalf("expected key to remain untouched")
	}
}

func TestSSLPluginLeavesInvalidListUntouched(t *testing.T) {
	fallback := certmagicmgr.Certificate{
		CertPEM:  []byte("fallback-cert"),
		KeyPEM:   []byte("fallback-key"),
		NotAfter: time.Now().Add(24 * time.Hour),
	}
	plugin := &SSLPlugin{
		Filesystems: []fs.FS{fstest.MapFS{}},
		Fallback:    fallback,
	}
	expectedEntry := map[string]any{
		"certs": []any{"file://cert.pem", 123},
		"keys":  []any{"file://key.pem", "other"},
	}
	input := map[string]any{
		"ssls": []any{
			expectedEntry,
		},
	}
	got, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	entry := got["ssls"].([]any)[0].(map[string]any)
	if diff := cmp.Diff(expectedEntry, entry); diff != "" {
		t.Fatalf("expected list to remain untouched (-want +got):\n%s", diff)
	}
}

func TestSSLPluginCertsKeysReplaceFiles(t *testing.T) {
	fsOne := fstest.MapFS{
		"cert.pem": {Data: []byte("cert-data")},
	}
	fallback := certmagicmgr.Certificate{
		CertPEM:  []byte("fallback-cert"),
		KeyPEM:   []byte("fallback-key"),
		NotAfter: time.Now().Add(24 * time.Hour),
	}
	plugin := &SSLPlugin{
		Filesystems: []fs.FS{fsOne},
		Fallback:    fallback,
	}
	input := map[string]any{
		"ssls": []any{
			map[string]any{
				"certs": []string{"file://cert.pem"},
				"keys":  []string{"inline-key"},
			},
		},
	}
	got, err := plugin.Update(testutil.Logger(), input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	entry := got["ssls"].([]any)[0].(map[string]any)
	certs := entry["certs"].([]string)
	keys := entry["keys"].([]string)
	if certs[0] != "cert-data" {
		t.Fatalf("expected cert to be replaced from file")
	}
	if keys[0] != "inline-key" {
		t.Fatalf("expected key to remain literal")
	}
}

type fakeACME struct {
	mu         sync.Mutex
	notifyCert certmagicmgr.Certificate
	err        error
	subs       []chan certmagicmgr.Notification
}

func (f *fakeACME) RequestCertificate(_ context.Context, _ *slog.Logger, _ string, sni string) error {
	if f.err != nil {
		return f.err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, ch := range f.subs {
		ch <- certmagicmgr.Notification{SNI: sni, Cert: f.notifyCert}
	}
	return nil
}

func (f *fakeACME) Subscribe(buffer int) chan certmagicmgr.Notification {
	ch := make(chan certmagicmgr.Notification, buffer)
	f.mu.Lock()
	f.subs = append(f.subs, ch)
	f.mu.Unlock()
	return ch
}

func (f *fakeACME) Unsubscribe(ch chan certmagicmgr.Notification) {
	f.mu.Lock()
	for i, sub := range f.subs {
		if sub == ch {
			f.subs = append(f.subs[:i], f.subs[i+1:]...)
			break
		}
	}
	f.mu.Unlock()
	close(ch)
}

func (f *fakeACME) ClearTracking(*slog.Logger) {}
