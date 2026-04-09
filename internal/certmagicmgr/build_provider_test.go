package certmagicmgr

import (
	"context"
	"testing"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/warpcondev/cuesix/internal/plugin/ssl"
	"github.com/warpcondev/cuesix/internal/testutil"
)

func TestBuildProviderConfiguresStorageAndIssuer(t *testing.T) {
	t.Parallel()
	storage := &testutil.MockStorage{}
	events := make(chan ssl.Tracking, 1)
	cfg := Config{CertObtainTimeout: 5 * time.Second}
	providerCfg := ProviderConfig{Name: "name", CA: "ca", Email: "mail"}

	p := buildProvider(nil, cfg, providerCfg, &testutil.MockCertMagic{}, storage, events)
	if p == nil || p.magic == nil {
		t.Fatalf("expected provider with magic config")
	}
	if got := len(storage.UpdateConfigCalls); got != 1 {
		t.Fatalf("expected UpdateConfig called once, got %d", got)
	}
	if p.Name() != providerCfg.Name {
		t.Fatalf("expected provider name %s, got %s", providerCfg.Name, p.Name())
	}
	if len(p.magic.Issuers) != 1 {
		t.Fatalf("expected one issuer, got %d", len(p.magic.Issuers))
	}
	issuer, ok := p.magic.Issuers[0].(*certmagic.ACMEIssuer)
	if !ok {
		t.Fatalf("expected ACME issuer, got %T", p.magic.Issuers[0])
	}
	if issuer.CertObtainTimeout != cfg.CertObtainTimeout {
		t.Fatalf("expected timeout %s, got %s", cfg.CertObtainTimeout, issuer.CertObtainTimeout)
	}

	// Exercise OnEvent hook when channel provided.
	err := p.magic.OnEvent(context.Background(), "cert_obtained", map[string]any{"identifier": "sni.example"})
	if err != nil {
		t.Fatalf("OnEvent returned error: %v", err)
	}
	select {
	case ev := <-events:
		if ev.Identity != "sni.example" || ev.Provider != providerCfg.Name {
			t.Fatalf("unexpected event payload: %+v", ev)
		}
	default:
		t.Fatalf("expected event to be enqueued")
	}
}

func TestBuildProviderNoEventsChannel(t *testing.T) {
	t.Parallel()
	p := buildProvider(testutil.Logger(), Config{}, ProviderConfig{Name: "name", CA: "ca", Email: "mail"}, &testutil.MockCertMagic{}, &testutil.MockStorage{}, nil)
	if p.magic.OnEvent != nil {
		t.Fatalf("expected OnEvent to remain unset when no channel provided")
	}
}
