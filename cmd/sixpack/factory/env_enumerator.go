package factory

import (
	"io/fs"
	"iter"
	"log/slog"

	"github.com/warpcomdev/sixpack/internal/compiler"
)

func NewEnvEnumerator(logger *slog.Logger, enumerator compiler.Enumerator, envFilename string) compiler.Enumerator {
	return envEnumerator{
		logger:      logger,
		enumerator:  compiler.DefaultEnumerator(logger, enumerator),
		envFilename: envFilename,
	}
}

type envEnumerator struct {
	logger      *slog.Logger
	enumerator  compiler.Enumerator
	envFilename string
}

func (e envEnumerator) Enumerate(fss ...fs.FS) iter.Seq2[compiler.Source, error] {
	return compiler.EnvEnumerate(e.logger, e.envFilename, e.enumerator.Enumerate(fss...))
}

type BuiltinFetcher struct {
	Logger     *slog.Logger
	Enumerator compiler.Enumerator
}

func (bf BuiltinFetcher) Fetch(fss ...fs.FS) iter.Seq2[compiler.Snippet, error] {
	return compiler.Fetch(bf.Logger, bf.Enumerator.Enumerate(fss...))
}
