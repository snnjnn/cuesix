package config

import (
	"os"
	"testing"
	"time"
)

func TestSplitComma(t *testing.T) {
	got := splitComma("a, b,,c ")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("unexpected split result: %#v", got)
	}
}

func TestSplitSemicolon(t *testing.T) {
	got := splitSemicolon("a; b;;c ")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("unexpected split result: %#v", got)
	}
}

func TestEnvBool(t *testing.T) {
	t.Setenv("CUESIX_BOOL", "true")
	if !envBool("CUESIX_BOOL", false) {
		t.Fatalf("expected true")
	}
	t.Setenv("CUESIX_BOOL", "0")
	if envBool("CUESIX_BOOL", true) {
		t.Fatalf("expected false")
	}
}

func TestEnvBoolDefault(t *testing.T) {
	os.Unsetenv("CUESIX_BOOL_DEFAULT")
	if !envBool("CUESIX_BOOL_DEFAULT", true) {
		t.Fatalf("expected default true")
	}
}

func TestEnvBoolInvalidFallsBack(t *testing.T) {
	t.Setenv("CUESIX_BOOL_INVALID", "nope")
	if envBool("CUESIX_BOOL_INVALID", true) != true {
		t.Fatalf("expected default value")
	}
}

func TestEnvDuration(t *testing.T) {
	t.Setenv("CUESIX_DUR", "150ms")
	if envDuration("CUESIX_DUR", 0) != 150*time.Millisecond {
		t.Fatalf("unexpected duration")
	}
}

func TestEnvDurationInvalidFallsBack(t *testing.T) {
	t.Setenv("CUESIX_DUR_INVALID", "not-a-duration")
	if envDuration("CUESIX_DUR_INVALID", 3) != 3 {
		t.Fatalf("expected default value")
	}
}

func TestEnvStringDefault(t *testing.T) {
	t.Setenv("CUESIX_STR", "")
	if envStringDefault("CUESIX_STR", "fallback") != "fallback" {
		t.Fatalf("expected fallback")
	}
	t.Setenv("CUESIX_STR", "value")
	if envStringDefault("CUESIX_STR", "fallback") != "value" {
		t.Fatalf("expected value")
	}
}

func TestEnvString(t *testing.T) {
	t.Setenv("CUESIX_RAW", "raw")
	if envString("CUESIX_RAW") != "raw" {
		t.Fatalf("expected raw value")
	}
}

func TestEnvIntFloat(t *testing.T) {
	t.Setenv("CUESIX_INT", "42")
	if envInt("CUESIX_INT", 0) != 42 {
		t.Fatalf("expected int")
	}
	t.Setenv("CUESIX_FLOAT", "1.5")
	if envFloat("CUESIX_FLOAT", 0) != 1.5 {
		t.Fatalf("expected float")
	}
}

func TestEnvIntInvalidFallsBack(t *testing.T) {
	t.Setenv("CUESIX_INT_INVALID", "not-an-int")
	if envInt("CUESIX_INT_INVALID", 7) != 7 {
		t.Fatalf("expected default value")
	}
}

func TestEnvFloatInvalidFallsBack(t *testing.T) {
	t.Setenv("CUESIX_FLOAT_INVALID", "not-a-float")
	if envFloat("CUESIX_FLOAT_INVALID", 1.25) != 1.25 {
		t.Fatalf("expected default value")
	}
}
