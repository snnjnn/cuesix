package factory

import (
	"github.com/warpcomdev/cuesix/internal/dispatcher"
	"github.com/warpcomdev/cuesix/internal/validator"
)

// compilerAdapter wires the compiler into dispatcher config.
type ValidatorFactory struct {
	validator.Validator
}

func (f ValidatorFactory) Instance() dispatcher.Validator {
	return f
}

// Reset the validator for a new iteration. Currently it is a noop
func (ValidatorFactory) Reset() {
}

// Commit the current compiler status after success. Currently it is a noop
func (ValidatorFactory) Commit() {
}
