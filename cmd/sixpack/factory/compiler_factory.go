package factory

import (
	"iter"
	"log/slog"

	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/dispatcher"
)

// CompilerFactory wires the compiler into dispatcher config.
type CompilerFactory struct {
	Logger   *slog.Logger
	DeepCopy bool
}

// Instance returns the merger implementation used by the dispatcher run.
func (c CompilerFactory) Instance(virtualgw string) dispatcher.Merger {
	return c
}

// Merge delegates snippet merging to compiler.Merge.
func (c CompilerFactory) Merge(snippets iter.Seq[compiler.Snippet]) (map[string]any, error) {
	result, err := compiler.Merge(c.Logger, snippets)
	if err != nil {
		return nil, err
	}
	if c.DeepCopy {
		copied, err := deepcopy(result)
		if err != nil {
			return nil, err
		}
		asMap, ok := copied.(map[string]any)
		if !ok {
			return nil, compiler.ErrWrongFormat
		}
		return asMap, nil
	}
	return result, nil
}

// Reset the compiler for a new iteration. Currently it is a noop
func (CompilerFactory) Reset() {
}

// Commit the current compiler status after success. Currently it is a noop
func (CompilerFactory) Commit() {
}
