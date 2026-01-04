package ssl

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/warpcomdev/cuesix/internal/cursor"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

func TestACMEHandlerReplaceTargetsSuccess(t *testing.T) {
	t.Parallel()
	cert := sslCert(time.Now().Add(time.Hour))
	fallback := Certificate{CertPEM: []byte("fb-cert"), KeyPEM: []byte("fb-key")}
	target := certTargets{
		sslId: "id",
		cert:  ACMEPrefix + "p1",
		snis:  []string{"example.com"},
		replace: func(certPEM, keyPEM []byte) {
			cert.CertPEM = certPEM
			cert.KeyPEM = keyPEM
		},
	}
	ready := make(chan struct{})
	tracker := &mockACMETracker{
		RequestCertificateFunc: func(ctx context.Context, provider, sni string) (Tracking, error) {
			return Tracking{Provider: provider, Identity: sni}, nil
		},
		WatchFunc: func(buffer int, topic string) cursor.Owned[Delivery] {
			ch := make(chan Delivery, buffer)
			close(ready)
			go func() {
				<-ready
				ch <- Delivery{Tracking: Tracking{Provider: ACMEPrefix + "p1", Identity: "example.com"}, Certificate: cert}
				close(ch)
			}()
			return cursor.Owned[Delivery]{Cursor: cursor.Channel(ch), Close: func() {}}
		},
	}
	record := make(map[Tracking]time.Time)
	handler := LiveHandler{Tracker: tracker, RequestTimeout: time.Second}
	handler.replaceTargets(context.Background(), testutil.Logger(), []certTargets{target}, record, fallback)
	if string(cert.CertPEM) != "cert" || string(cert.KeyPEM) != "key" {
		t.Fatalf("expected cert/key to be replaced, got cert=%q key=%q", cert.CertPEM, cert.KeyPEM)
	}
	if len(record) != 1 {
		t.Fatalf("expected record updated, got %d entries", len(record))
	}
	if len(tracker.requestCertificateCalls) != 1 {
		t.Fatalf("expected RequestCertificate called once, got %d", len(tracker.requestCertificateCalls))
	}
}

func TestACMEHandlerReplaceTargetsFallbacks(t *testing.T) {
	t.Parallel()
	fallback := Certificate{CertPEM: []byte("fb-cert"), KeyPEM: []byte("fb-key")}
	tests := []struct {
		name     string
		handler  LiveHandler
		targets  []certTargets
		wantCert string
		wantKey  string
	}{
		{
			name:    "tracker missing",
			handler: LiveHandler{},
			targets: []certTargets{{
				cert: ACMEPrefix + "p1",
				snis: []string{"example.com"},
			}},
			wantCert: "fb-cert",
			wantKey:  "fb-key",
		},
		{
			name: "request fails",
			handler: LiveHandler{
				Tracker: &mockACMETracker{
					RequestCertificateFunc: func(context.Context, string, string) (Tracking, error) {
						return Tracking{}, errors.New("boom")
					},
					WatchFunc: func(buffer int, topic string) cursor.Owned[Delivery] {
						ch := make(chan Delivery, buffer)
						close(ch)
						return cursor.Owned[Delivery]{Cursor: cursor.Channel(ch), Close: func() {}}
					},
				},
				RequestTimeout: 5 * time.Millisecond,
			},
			targets: []certTargets{{
				cert: ACMEPrefix + "p1",
				snis: []string{"example.com"},
			}},
			wantCert: "fb-cert",
			wantKey:  "fb-key",
		},
		{
			name: "multiple snis fallback",
			handler: LiveHandler{
				Tracker: &mockACMETracker{
					WatchFunc: func(buffer int, topic string) cursor.Owned[Delivery] {
						ch := make(chan Delivery, buffer)
						close(ch)
						return cursor.Owned[Delivery]{Cursor: cursor.Channel(ch), Close: func() {}}
					},
				},
				RequestTimeout: 5 * time.Millisecond,
			},
			targets: []certTargets{{
				cert: ACMEPrefix + "p1",
				snis: []string{"a", "b"},
			}},
			wantCert: "fb-cert",
			wantKey:  "fb-key",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var certOut, keyOut string
			tt.targets[0].replace = func(certPEM, keyPEM []byte) {
				certOut = string(certPEM)
				keyOut = string(keyPEM)
			}
			tt.handler.replaceTargets(context.Background(), testutil.Logger(), tt.targets, nil, fallback)
			if certOut != tt.wantCert || keyOut != tt.wantKey {
				t.Fatalf("unexpected output cert=%q key=%q want cert=%q key=%q", certOut, keyOut, tt.wantCert, tt.wantKey)
			}
		})
	}
}
