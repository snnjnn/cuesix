package ssl

import "testing"

func TestTextHandlerNoop(t *testing.T) {
	t.Parallel()
	target := certTargets{
		cert: "unchanged",
		key:  "unchanged",
		replace: func(cert, key []byte) {
			if string(cert) != "unchanged" || string(key) != "unchanged" {
				t.Fatalf("expected passthrough, got cert=%q key=%q", cert, key)
			}
		},
	}
	TextHandler{}.replaceTargets(nil, []certTargets{target}, Certificate{})
}
