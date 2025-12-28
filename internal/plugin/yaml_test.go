package plugin

import (
	"bytes"
	"testing"
)

func TestYAMLPluginEmptyPayload(t *testing.T) {
	plugin := &YAMLPlugin{}
	got, err := plugin.Update(nil)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	want := []byte("#END\n")
	if !bytes.Equal(got, want) {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestYAMLPluginConvertsJSONToYAML(t *testing.T) {
	plugin := &YAMLPlugin{}
	input := []byte("{\"routes\":[{\"id\":1}]}")
	got, err := plugin.Update(input)
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}
	if !bytes.Contains(got, []byte("routes:")) {
		t.Fatalf("expected yaml output, got %q", got)
	}
	if !bytes.Contains(got, []byte("#END\n")) {
		t.Fatalf("expected end marker, got %q", got)
	}
}
