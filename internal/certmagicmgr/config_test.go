package certmagicmgr

import (
	"testing"
	"time"
)

func TestParseProviderSpec(t *testing.T) {
	spec := "name=letsencrypt,ca=https://example, email=ops@example.com,timeout=30s"
	got, err := ParseProviderSpec(spec)
	if err != nil {
		t.Fatalf("ParseProviderSpec returned error: %v", err)
	}
	if got.Name != "letsencrypt" || got.CA != "https://example" || got.Email != "ops@example.com" {
		t.Fatalf("unexpected provider fields: %#v", got)
	}
	if got.Timeout != 30*time.Second {
		t.Fatalf("unexpected timeout: %v", got.Timeout)
	}
}

func TestParseProviderSpecUnknownField(t *testing.T) {
	if _, err := ParseProviderSpec("name=foo,unknown=bar"); err == nil {
		t.Fatalf("expected error for unknown field")
	}
}

func TestParseProviderSpecInvalidTimeout(t *testing.T) {
	if _, err := ParseProviderSpec("name=foo,timeout=bogus"); err == nil {
		t.Fatalf("expected error for invalid timeout")
	}
}
