package factory_test

import (
	"testing"

	"github.com/warpcomdev/cuesix/cmd/sixpack/factory"
	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/dispatcher"
)

type spyValidator struct {
	calls int
}

func (s *spyValidator) Validate([]byte, bool) (bool, error) {
	s.calls++
	return false, nil
}

func (*spyValidator) Reset() {}

func (*spyValidator) Commit() {}

type validatorFactory struct {
	validator dispatcher.Validator
}

func (f validatorFactory) Instance(string) dispatcher.Validator {
	return f.validator
}

func TestSingletonValidatorFactory(t *testing.T) {
	t.Parallel()

	spy := &spyValidator{}
	build := factory.NewSingletonValidatorFactory(spy)

	top := build.Instance(compiler.DEFAULT_VIRTUALGW)
	nested := build.Instance("edge.child")

	ok, err := top.Validate([]byte("cfg"), false)
	if err != nil {
		t.Fatalf("top-level validate error: %v", err)
	}
	if ok {
		t.Fatalf("top-level gateway should delegate to the wrapped validator")
	}

	ok, err = nested.Validate([]byte("cfg"), false)
	if err != nil {
		t.Fatalf("nested validate error: %v", err)
	}
	if ok {
		t.Fatalf("nested gateway should delegate to the wrapped validator")
	}

	if spy.calls != 2 {
		t.Fatalf("singleton validator should be reused for all gateways, got %d calls", spy.calls)
	}
}

func TestSingletonValidatorFactoryNilValidator(t *testing.T) {
	t.Parallel()

	build := factory.NewSingletonValidatorFactory(nil)
	validator := build.Instance(compiler.DEFAULT_VIRTUALGW)

	ok, err := validator.Validate([]byte("cfg"), false)
	if err != nil {
		t.Fatalf("nil validator validate error: %v", err)
	}
	if !ok {
		t.Fatalf("nil validator should behave as dry-run and succeed")
	}

	validator.Reset()
	validator.Commit()
}

func TestHierarchicalValidatorFactoryDepthZero(t *testing.T) {
	t.Parallel()

	spy := &spyValidator{}
	build := factory.NewHierarchicalValidatorFactory(0, validatorFactory{validator: spy})

	top := build.Instance(compiler.DEFAULT_VIRTUALGW)
	ok, err := top.Validate([]byte("cfg"), false)
	if err != nil {
		t.Fatalf("top-level validate error: %v", err)
	}
	if ok {
		t.Fatalf("top-level should use wrapped validator")
	}
	if spy.calls != 1 {
		t.Fatalf("top-level should call wrapped validator once, got %d", spy.calls)
	}

	nested := build.Instance("edge.child")
	ok, err = nested.Validate([]byte("cfg"), false)
	if err != nil {
		t.Fatalf("nested validate error: %v", err)
	}
	if !ok {
		t.Fatalf("nested should skip validation")
	}
	if spy.calls != 1 {
		t.Fatalf("nested should not call wrapped validator, got %d calls", spy.calls)
	}
}

func TestHierarchicalValidatorFactoryDepthOneBoundary(t *testing.T) {
	t.Parallel()

	spy := &spyValidator{}
	build := factory.NewHierarchicalValidatorFactory(1, validatorFactory{validator: spy})

	oneSep := build.Instance("edge.child")
	ok, err := oneSep.Validate([]byte("cfg"), false)
	if err != nil {
		t.Fatalf("one-separator validate error: %v", err)
	}
	if ok {
		t.Fatalf("one-separator gateway should still be validated at depth 1")
	}

	twoSep := build.Instance("edge.child.grandchild")
	ok, err = twoSep.Validate([]byte("cfg"), false)
	if err != nil {
		t.Fatalf("two-separator validate error: %v", err)
	}
	if !ok {
		t.Fatalf("two-separator gateway should skip validation at depth 1")
	}
	if spy.calls != 1 {
		t.Fatalf("expected one wrapped validator call, got %d", spy.calls)
	}
}
