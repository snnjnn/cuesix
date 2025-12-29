package ssl

import (
	"context"
	"io/fs"
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
	cert certmagicmgr.Certificate
	err  error
}

func (f *fakeACME) RequestCertificate(_ context.Context, _ string, _ string) (certmagicmgr.Certificate, error) {
	return f.cert, f.err
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

type notifyCounter struct {
	count int
}

func (n *notifyCounter) Notify() {
	n.count++
}

func TestExpiryManagerNotifyOncePerDay(t *testing.T) {
	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	notifier := &notifyCounter{}
	manager := NewExpiryManager(5*24*time.Hour, time.Hour, notifier)

	manager.RecordSNI("example.com", now.Add(48*time.Hour))
	manager.checkAndNotify(now, testutil.Logger())

	if notifier.count != 1 {
		t.Fatalf("expected 1 notification, got %d", notifier.count)
	}

	manager.checkAndNotify(now.Add(2*time.Hour), testutil.Logger())
	if notifier.count != 1 {
		t.Fatalf("expected 1 notification after same-day check, got %d", notifier.count)
	}

	manager.checkAndNotify(now.Add(25*time.Hour), testutil.Logger())
	if notifier.count != 2 {
		t.Fatalf("expected 2 notifications after next-day check, got %d", notifier.count)
	}
}

func TestExpiryManagerSuppressOnConfigDay(t *testing.T) {
	now := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	notifier := &notifyCounter{}
	manager := NewExpiryManager(5*24*time.Hour, time.Hour, notifier)

	manager.ResetForConfig(now)
	manager.RecordSNI("example.com", now.Add(48*time.Hour))
	manager.checkAndNotify(now, testutil.Logger())

	if notifier.count != 0 {
		t.Fatalf("expected no notification on config day, got %d", notifier.count)
	}
}
