package main

import (
	"io/fs"
	"log/slog"

	"github.com/warpcomdev/cuesix/internal/compiler"
)

// compilerAdapter wires the compiler into dispatcher config.
type compilerAdapter struct{}

// Compile delegates to the compiler module.
func (compilerAdapter) Compile(logger *slog.Logger, fses ...fs.FS) (map[string]any, error) {
	return compiler.Compile(logger, fses...)
}
