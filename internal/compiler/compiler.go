package compiler

import (
	"fmt"
	"io/fs"
)

// CompilerError is a custom error type for errors originating from the compiler module.
type CompilerError struct {
	Msg string
	Err error
}

func (e *CompilerError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("compiler error: %s: %v", e.Msg, e.Err)
	}
	return fmt.Sprintf("compiler error: %s", e.Msg)
}

func (e *CompilerError) Unwrap() error {
	return e.Err
}

// Source defines a source of CUE/YAML files for the compiler.
type Source struct {
	FS      fs.FS
	Include []string
	Exclude []string
}
