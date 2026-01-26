package schema

import (
	"io/fs"
	"iter"
	"log/slog"
	"sort"
	"time"

	"github.com/warpcomdev/sixpack/internal/compiler"
	"github.com/warpcomdev/sixpack/internal/cursor"
)

type SourcesEnumerator struct {
	cursor.Lock
	enumerator compiler.Enumerator
	sources    map[string][]byte
	timestamp  time.Time
}

func NewSourcesEnumerator(logger *slog.Logger, enumerator compiler.Enumerator) *SourcesEnumerator {
	if logger == nil {
		logger = slog.Default()
	}
	return &SourcesEnumerator{
		enumerator: compiler.DefaultEnumerator(logger, enumerator),
		sources:    make(map[string][]byte),
	}
}

func (se *SourcesEnumerator) Enumerate(fss ...fs.FS) iter.Seq2[compiler.Source, error] {
	return func(yield func(compiler.Source, error) bool) {
		defer func() {
			se.WithLock(func() {
				se.timestamp = time.Now()
			})
		}()
		for source, err := range se.enumerator.Enumerate(fss...) {
			if err != nil {
				if !yield(source, err) {
					return
				}
				continue
			}
			se.record(source)
			if !yield(source, nil) {
				return
			}
		}
	}
}

func (se *SourcesEnumerator) ListPaths() []string {
	if se == nil {
		return nil
	}
	var paths []string
	se.WithLock(func() {
		paths = make([]string, 0, len(se.sources))
		for path := range se.sources {
			paths = append(paths, path)
		}
	})
	sort.Strings(paths)
	return paths
}

func (se *SourcesEnumerator) LastModified() time.Time {
	if se == nil {
		return time.Time{}
	}
	var ts time.Time
	se.WithLock(func() {
		ts = se.timestamp
	})
	return ts
}

func (se *SourcesEnumerator) Get(path string) ([]byte, bool) {
	if se == nil {
		return nil, false
	}
	var data []byte
	var ok bool
	se.WithLock(func() {
		if source, exists := se.sources[path]; exists {
			data = make([]byte, len(source))
			copy(data, source)
			ok = true
		}
	})
	return data, ok
}

func (se *SourcesEnumerator) record(source compiler.Source) {
	if se == nil {
		return
	}
	if source.Path == "" {
		return
	}
	data := make([]byte, len(source.Data))
	copy(data, source.Data)
	se.WithLock(func() {
		se.sources[source.Path] = data
	})
}
