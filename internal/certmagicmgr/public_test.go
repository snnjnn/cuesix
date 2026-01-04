package certmagicmgr_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/warpcomdev/cuesix/internal/certmagicmgr"
	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

func TestProviderRequestAndBestMatchPublic(t *testing.T) {
	t.Parallel()
	adapter := &testutil.MockCertMagic{
		ManageAsyncFunc: func(ctx context.Context, cfg *certmagic.Config, snis []string) error {
			return nil
		},
		AllMatchingCertificatesFunc: func(cache *certmagic.Cache, sni string) []certmagic.Certificate {
			return []certmagic.Certificate{testutil.MakeCert(t, time.Now().Add(time.Hour))}
		},
	}
	cfg := certmagicmgr.Config{
		DataDir: t.TempDir(),
		Providers: []certmagicmgr.ProviderConfig{
			{Name: "p1", Email: "e", CA: "ca"},
		},
	}
	mgr, err := certmagicmgr.NewManager(testutil.Logger(), cfg, nil, ssl.Certificate{CertPEM: []byte("c"), KeyPEM: []byte("k")}, adapter, nil)
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	p, err := mgr.ResolveProvider("p1")
	if err != nil {
		t.Fatalf("ResolveProvider error: %v", err)
	}
	if err := p.RequestCertificate(context.Background(), "example.com"); err != nil {
		t.Fatalf("RequestCertificate returned error: %v", err)
	}
	if _, ok := p.BestMatchFor(context.Background(), "example.com"); !ok {
		t.Fatalf("expected best match")
	}
}

func TestManagerPublicAPI(t *testing.T) {
	t.Parallel()
	cfg := certmagicmgr.Config{
		DataDir: t.TempDir(),
		Providers: []certmagicmgr.ProviderConfig{
			{Name: "p1", Email: "e", CA: "ca"},
		},
	}
	adapter := &testutil.MockCertMagic{}
	mgr, err := certmagicmgr.NewManager(testutil.Logger(), cfg, nil, ssl.Certificate{CertPEM: []byte("c"), KeyPEM: []byte("k")}, adapter, &testutil.MockStorage{})
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}
	if _, err := mgr.ResolveProvider("p1"); err != nil {
		t.Fatalf("ResolveProvider returned error: %v", err)
	}
	if _, err := mgr.ResolveProvider("missing"); err == nil || !strings.Contains(err.Error(), "unknown provider") {
		t.Fatalf("expected unknown provider error")
	}
}
