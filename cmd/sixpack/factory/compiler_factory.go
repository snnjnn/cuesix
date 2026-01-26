package factory

import (
	"iter"
	"log/slog"

	"github.com/warpcomdev/sixpack/internal/compiler"
	"github.com/warpcomdev/sixpack/internal/dispatcher"
)

// CompilerFactory wires the compiler into dispatcher config.
type CompilerFactory struct {
	Logger *slog.Logger
}

func (c CompilerFactory) Instance() dispatcher.Merger {
	return c
}

func (c CompilerFactory) Merge(snippets iter.Seq[compiler.Snippet]) (map[string]any, error) {
	return compiler.Merge(c.Logger, snippets)
}

// Reset the compiler for a new iteration. Currently it is a noop
func (CompilerFactory) Reset() {
}

// Commit the current compiler status after success. Currently it is a noop
func (CompilerFactory) Commit() {
}
