package cache

import "testing"

func TestMarshalDeterministicYAMLEndsWithEnd(t *testing.T) {
	payload, err := MarshalDeterministicYAML(map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("MarshalDeterministicYAML returned error: %v", err)
	}
	if len(payload) < len("#END\n") {
		t.Fatalf("expected payload to contain #END line")
	}
	if string(payload[len(payload)-5:]) != "#END\n" {
		t.Fatalf("expected trailing #END line, got %q", string(payload[len(payload)-5:]))
	}

	again, err := MarshalDeterministicYAML(map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("MarshalDeterministicYAML returned error: %v", err)
	}
	if string(again[len(again)-5:]) != "#END\n" {
		t.Fatalf("expected trailing #END line on repeated call")
	}
}
