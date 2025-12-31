package config

import "testing"

func TestReloadBuildURL(t *testing.T) {
	cfg := Reload{URL: "http://127.0.0.1:9180"}
	got, err := cfg.BuildURL()
	if err != nil {
		t.Fatalf("BuildURL returned error: %v", err)
	}
	want := "http://127.0.0.1:9180/apisix/admin/configs?reload=true"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}

	cfg.URL = "http://127.0.0.1:9180/base?x=1"
	got, err = cfg.BuildURL()
	if err != nil {
		t.Fatalf("BuildURL returned error: %v", err)
	}
	want = "http://127.0.0.1:9180/apisix/admin/configs?reload=true&x=1"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestReloadBuildURLBlank(t *testing.T) {
	cfg := Reload{URL: "  "}
	got, err := cfg.BuildURL()
	if err != nil {
		t.Fatalf("BuildURL returned error: %v", err)
	}
	if got != "" {
		t.Fatalf("expected empty url, got %q", got)
	}
}
