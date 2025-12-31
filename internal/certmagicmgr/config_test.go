package certmagicmgr

import "testing"

func TestParseProviderSpec(t *testing.T) {
	spec := "letsencrypt|ops@example.com|https://example"
	got, err := ParseProviderSpec(spec)
	if err != nil {
		t.Fatalf("ParseProviderSpec returned error: %v", err)
	}
	if got.Name != "letsencrypt" || got.CA != "https://example" || got.Email != "ops@example.com" {
		t.Fatalf("unexpected provider fields: %#v", got)
	}
}

func TestParseProviderSpecUnknownField(t *testing.T) {
	if _, err := ParseProviderSpec("name=foo|email"); err == nil {
		t.Fatalf("expected error for invalid format")
	}
}
