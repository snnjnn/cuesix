package factory

import (
	"strings"

	"github.com/warpcondev/cuesix/internal/compiler"
	"github.com/warpcondev/cuesix/internal/dispatcher"
)

// nop-validator always returns true
type SingletonValidator interface {
	Validate(candidate []byte, isYAML bool) (bool, error)
}

type validatorAdapter struct {
	validator SingletonValidator
}

type SingletonValidatorFactory struct {
	Validator SingletonValidator
}

type HierarchicalValidatorFactory struct {
	MaxGatewayDepth int
	Nested          dispatcher.ValidatorFactory
}

// NewSingletonValidatorFactory returns the same validator for all gateways.
func NewSingletonValidatorFactory(validator SingletonValidator) SingletonValidatorFactory {
	return SingletonValidatorFactory{Validator: validator}
}

// HierarchicalValidatorFactory controls validation based on virtual gateway depth
func NewHierarchicalValidatorFactory(maxGatewayDepth int, factory dispatcher.ValidatorFactory) HierarchicalValidatorFactory {
	return HierarchicalValidatorFactory{
		MaxGatewayDepth: maxGatewayDepth,
		Nested:          factory,
	}
}

func (f SingletonValidatorFactory) Instance(string) dispatcher.Validator {
	return validatorAdapter{validator: f.Validator}
}

func (f HierarchicalValidatorFactory) Instance(virtualgw string) dispatcher.Validator {
	if strings.Count(virtualgw, compiler.VIRTUALGW_SEP) > f.MaxGatewayDepth {
		// Nil hierarchivalValidator: always returns true
		return validatorAdapter{}
	}
	return f.Nested.Instance(virtualgw)
}

func (v validatorAdapter) Validate(candidate []byte, isYAML bool) (bool, error) {
	if v.validator == nil {
		return true, nil
	}
	return v.validator.Validate(candidate, isYAML)
}

func (validatorAdapter) Reset() {
}

func (validatorAdapter) Commit() {
}
