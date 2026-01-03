package plugin_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/warpcomdev/cuesix/internal/plugin"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

func TestYAMLPluginUpdate(t *testing.T) {
	t.Parallel()
	p := &plugin.YAMLPlugin{}

	out, err := p.Update(testutil.Logger(), nil)
	if err != nil {
		t.Fatalf("expected nil error for empty payload: %v", err)
	}
	if string(out) != "#END\n" {
		t.Fatalf("unexpected empty payload output: %q", out)
	}

	if _, err := p.Update(testutil.Logger(), []byte("{invalid}")); err == nil {
		t.Fatalf("expected decode error")
	}

	jsonPayload, _ := json.Marshal(map[string]any{"a": 1})
	out, err = p.Update(testutil.Logger(), jsonPayload)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(string(out), "\n#END\n") {
		t.Fatalf("expected END marker, got %q", out)
	}
}
