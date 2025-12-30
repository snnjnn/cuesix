package ssl

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
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
	plugin := &SSLPlugin{Filesystems: []fs.FS{fsOne}}

	input := map[string]any{
		"ssls": []any{
			map[string]any{
				"id":    "1",
				"cert":  "file://cert.pem",
				"key":   "file://key.pem",
				"certs": []any{"file://chain.pem"},
				"keys":  []any{"inline-key"},
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
				"certs": []any{"chain-data"},
				"keys":  []any{"inline-key"},
				"client": map[string]any{
					"ca": "client-data",
				},
			},
		},
	}

	if diff := cmp.Diff(want, got); diff != "" {
		t.Fatalf("unexpected result (-want +got):\n%s", diff)
	}
}

func TestSSLPluginMissingFile(t *testing.T) {
	plugin := &SSLPlugin{Filesystems: []fs.FS{fstest.MapFS{}}}
	input := map[string]any{
		"ssls": []any{
			map[string]any{
				"cert": "file://missing.pem",
			},
		},
	}
	if _, err := plugin.Update(testutil.Logger(), input); err == nil {
		t.Fatalf("expected error for missing file")
	}
}

type fakeACME struct {
	cert        certmagicmgr.Certificate
	fallback    certmagicmgr.Certificate
	err         error
	fallbackErr error
}

func (f *fakeACME) RequestCertificate(_ context.Context, _ *slog.Logger, _ string, _ string) (certmagicmgr.Certificate, error) {
	return f.cert, f.err
}

func (f *fakeACME) FallbackCertificate() (certmagicmgr.Certificate, error) {
	return f.fallback, f.fallbackErr
}

func TestSSLPluginACMEReplacesCertAndKey(t *testing.T) {
	plugin := &SSLPlugin{
		Filesystems: []fs.FS{fstest.MapFS{}},
	}
	cert := certmagicmgr.Certificate{
		CertPEM:  []byte("cert-data"),
		KeyPEM:   []byte("key-data"),
		NotAfter: time.Now().Add(24 * time.Hour),
	}
	plugin.ACME = &fakeACME{cert: cert}

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

	entries := got["ssls"].([]any)
	entry := entries[0].(map[string]any)
	if entry["cert"] != "cert-data" {
		t.Fatalf("expected cert to be replaced")
	}
	if entry["key"] != "key-data" {
		t.Fatalf("expected key to be replaced")
	}
}

func TestSSLPluginACMEFallback(t *testing.T) {
	plugin := &SSLPlugin{
		Filesystems: []fs.FS{fstest.MapFS{}},
	}
	fallback := certmagicmgr.Certificate{
		CertPEM:  []byte("fallback-cert"),
		KeyPEM:   []byte("fallback-key"),
		NotAfter: time.Now().Add(24 * time.Hour),
	}
	plugin.ACME = &fakeACME{
		err:      errors.New("acme failed"),
		fallback: fallback,
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

	entries := got["ssls"].([]any)
	entry := entries[0].(map[string]any)
	if entry["cert"] != "fallback-cert" {
		t.Fatalf("expected fallback cert to be used")
	}
	if entry["key"] != "fallback-key" {
		t.Fatalf("expected fallback key to be used")
	}
}

type notifyCounter struct {
	count int
}

func (n *notifyCounter) Notify() {
	n.count++
}
