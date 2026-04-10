package recorder

import (
	"log/slog"
	"testing"

	"github.com/warpcomdev/cuesix/internal/dispatcher"
)

type lifecycleValidator struct {
	resetCalls    int
	validateCalls int
	commitCalls   int
}

func (v *lifecycleValidator) Reset() {
	v.resetCalls++
}

func (v *lifecycleValidator) Validate([]byte, bool) (bool, error) {
	v.validateCalls++
	return true, nil
}

func (v *lifecycleValidator) Commit() {
	v.commitCalls++
}

type validatorFactory struct {
	validator dispatcher.Validator
}

func (f validatorFactory) Instance(string) dispatcher.Validator {
	return f.validator
}

func TestRecorderInstanceDelegatesLifecycle(t *testing.T) {
	t.Parallel()

	inner := &lifecycleValidator{}
	rec := NewRecorder(slog.Default(), nil, validatorFactory{validator: inner})

	inst := rec.Instance("default")
	inst.Reset()

	ok, err := inst.Validate([]byte(`{"routes":[]}`), false)
	if err != nil {
		t.Fatalf("validate error: %v", err)
	}
	if !ok {
		t.Fatalf("validate should succeed")
	}

	inst.Commit()

	if inner.resetCalls != 1 {
		t.Fatalf("expected one reset call, got %d", inner.resetCalls)
	}
	if inner.validateCalls != 1 {
		t.Fatalf("expected one validate call, got %d", inner.validateCalls)
	}
	if inner.commitCalls != 1 {
		t.Fatalf("expected one commit call, got %d", inner.commitCalls)
	}
}
