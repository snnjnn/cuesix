package certmagicmgr

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

func TestNewManagerValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		cfg  Config
	}{
		{
			name: "no providers",
			cfg: Config{
				DataDir: "/tmp",
			},
		},
		{
			name: "missing data dir",
			cfg: Config{
				Providers: []ProviderConfig{{Name: "p1", CA: "ca", Email: "mail"}},
			},
		},
		{
			name: "incomplete provider",
			cfg: Config{
				DataDir:   "/tmp",
				Providers: []ProviderConfig{{Name: "p1"}},
			},
		},
		{
			name: "duplicate provider",
			cfg: Config{
				DataDir: "/tmp",
				Providers: []ProviderConfig{
					{Name: "p1", CA: "ca", Email: "mail"},
					{Name: "p1", CA: "ca", Email: "mail"},
				},
			},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewManager(testutil.Logger(), tt.cfg, nil, ssl.PEMCertificate{}, nil, nil)
			if err == nil {
				t.Fatalf("expected error")
			}
		})
	}
}

func TestNewManagerUsesProvidedStorage(t *testing.T) {
	t.Parallel()
	cfg := Config{
		DataDir: "/tmp/data",
		Providers: []ProviderConfig{
			{Name: "p1", CA: "ca", Email: "mail"},
		},
	}
	storage := &testutil.MockStorage{}
	adapter := &testutil.MockCertMagic{}
	_, err := NewManager(testutil.Logger(), cfg, nil, ssl.PEMCertificate{CertPEM: []byte("c"), KeyPEM: []byte("k")}, adapter, storage)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	if got := len(storage.UpdateConfigCalls); got != 1 {
		t.Fatalf("expected UpdateConfig to be called once, got %d", got)
	}
}

func TestNewManagerUsesAdapterStorageWhenNil(t *testing.T) {
	t.Parallel()
	cfg := Config{
		DataDir: "/tmp/data",
		Providers: []ProviderConfig{
			{Name: "p1", CA: "ca", Email: "mail"},
		},
	}
	adapter := &testutil.MockCertMagic{}
	manager, err := NewManager(testutil.Logger(), cfg, nil, ssl.PEMCertificate{CertPEM: []byte("c"), KeyPEM: []byte("k")}, adapter, nil)
	if err != nil {
		t.Fatalf("NewManager returned error: %v", err)
	}
	storageAdapter, ok := manager.storage.(storageAdapter)
	if !ok {
		t.Fatalf("expected storageAdapter, got %T", manager.storage)
	}
	fs, ok := storageAdapter.storage.(*certmagic.FileStorage)
	if !ok {
		t.Fatalf("expected file storage, got %T", storageAdapter.storage)
	}
	if fs.Path != cfg.DataDir {
		t.Fatalf("expected storage path %q, got %q", cfg.DataDir, fs.Path)
	}
}

func TestManagerResolveProvider(t *testing.T) {
	t.Parallel()
	providerA := &Provider{cfg: ProviderConfig{Name: "a"}}
	providerB := &Provider{cfg: ProviderConfig{Name: "b"}}
	tests := []struct {
		name     string
		manager  Manager
		input    string
		expected *Provider
		wantErr  string
	}{
		{
			name: "by name",
			manager: Manager{
				providers: map[string]*Provider{"a": providerA},
			},
			input:    "a",
			expected: providerA,
		},
		{
			name: "unknown by name",
			manager: Manager{
				providers: map[string]*Provider{"a": providerA},
			},
			input:   "missing",
			wantErr: "unknown provider",
		},
		{
			name: "default present",
			manager: Manager{
				cfg:       Config{DefaultProvider: "b"},
				providers: map[string]*Provider{"a": providerA, "b": providerB},
			},
			expected: providerB,
		},
		{
			name: "default missing",
			manager: Manager{
				cfg:       Config{DefaultProvider: "c"},
				providers: map[string]*Provider{"a": providerA},
			},
			wantErr: "unknown default provider",
		},
		{
			name: "single provider fallback",
			manager: Manager{
				providers: map[string]*Provider{"a": providerA},
			},
			expected: providerA,
		},
		{
			name: "no provider chosen",
			manager: Manager{
				providers: map[string]*Provider{"a": providerA, "b": providerB},
			},
			wantErr: "provider is required",
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.manager.ResolveProvider(tt.input)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.expected {
				t.Fatalf("expected provider %v, got %v", tt.expected, got)
			}
		})
	}
}

func TestManagerRemoveExpired(t *testing.T) {
	t.Parallel()
	storage := &testutil.MockStorage{}
	manager := Manager{
		storage: storage,
	}
	ctx := context.Background()
	interval := 5 * time.Minute
	grace := 10 * time.Minute
	if err := manager.RemoveExpired(ctx, interval, grace); err != nil {
		t.Fatalf("RemoveExpired returned error: %v", err)
	}
	if got := len(storage.CleanStorageCalls); got != 1 {
		t.Fatalf("expected CleanStorage called once, got %d", got)
	}
	call := storage.CleanStorageCalls[0]
	if call.Opts.Interval != interval {
		t.Fatalf("expected interval %s, got %s", interval, call.Opts.Interval)
	}
	if !call.Opts.ExpiredCerts {
		t.Fatalf("expected expired certs cleanup enabled")
	}
	if call.Opts.ExpiredCertGracePeriod != grace {
		t.Fatalf("expected grace period %s, got %s", grace, call.Opts.ExpiredCertGracePeriod)
	}
	if call.Ctx != ctx {
		t.Fatalf("expected context propagated")
	}
}

func TestParseACMEProviderSpec(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		spec    string
		wantErr bool
	}{
		{
			name: "valid",
			spec: "name|email@example.com|https://ca",
		},
		{
			name:    "missing parts",
			spec:    "name|email",
			wantErr: true,
		},
		{
			name:    "empty fields",
			spec:    "||",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg, err := ParseACMEProviderSpec(tt.spec)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !strings.HasPrefix(cfg.Name, ssl.ACMEPrefix) {
				t.Fatalf("expected acme prefix, got %s", cfg.Name)
			}
			if cfg.Email != "email@example.com" || cfg.CA != "https://ca" {
				t.Fatalf("parsed fields mismatch: %+v", cfg)
			}
		})
	}
}
