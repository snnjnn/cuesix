package factory

import (
	"io/fs"
	"iter"
	"log/slog"

	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/dispatcher"
)

// CompilerFactory wires the compiler into dispatcher config.
type CompilerFactory struct {
	Logger *slog.Logger
}

func (c CompilerFactory) Instance() dispatcher.Merger {
	return c
}

// Compile delegates to the compiler module.
func (c CompilerFactory) Fetch(fses ...fs.FS) iter.Seq2[compiler.Snippet, error] {
	return compiler.Fetch(c.Logger, fses...)
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
