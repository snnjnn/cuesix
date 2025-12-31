package main

import "testing"

func TestSplitSemicolon(t *testing.T) {
	got := splitSemicolon("a; b;;c ")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("unexpected split result: %#v", got)
	}
}

func TestEnvBoolInvalidFallsBack(t *testing.T) {
	t.Setenv("CUESIX_BOOL_INVALID", "nope")
	if envBool("CUESIX_BOOL_INVALID", true) != true {
		t.Fatalf("expected default value")
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

func TestEnvDurationInvalidFallsBack(t *testing.T) {
	t.Setenv("CUESIX_DUR_INVALID", "not-a-duration")
	if envDuration("CUESIX_DUR_INVALID", 3) != 3 {
		t.Fatalf("expected default value")
	}
}
