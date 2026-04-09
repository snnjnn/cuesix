package recorder

import (
	"errors"
	"iter"
	"log/slog"
	"slices"
	"time"

	"github.com/warpcondev/cuesix/internal/compiler"
	"github.com/warpcondev/cuesix/internal/cursor"
	"go.yaml.in/yaml/v4"
)

type sourceData struct {
	Ref       compiler.SourceRef
	Virtualgw compiler.VirtualGateway
	Data      []byte
}

type SourcesEnumerator struct {
	cursor.Lock
	enumerator compiler.Enumerator
	sources    map[string]sourceData
	timestamp  time.Time
}

// NewSourcesEnumerator wraps a compiler enumerator and records discovered sources.
func NewSourcesEnumerator(logger *slog.Logger, enumerator compiler.Enumerator) (*SourcesEnumerator, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if enumerator == nil {
		return nil, errors.New("enumerator cannot be nil")
	}
	return &SourcesEnumerator{
		enumerator: enumerator,
		sources:    make(map[string]sourceData),
	}, nil
}

// Enumerate forwards enumeration while caching source contents and refresh timestamp.
func (se *SourcesEnumerator) Enumerate(roots ...compiler.InputRoot) iter.Seq2[compiler.Source, error] {
	return func(yield func(compiler.Source, error) bool) {
		defer func() {
			se.WithLock(func() {
				se.timestamp = time.Now()
			})
		}()
		for source, err := range se.enumerator.Enumerate(roots...) {
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

// SourceMap returns source key => leaf virtual gateway.
func (se *SourcesEnumerator) SourceMap() map[string]string {
	if se == nil {
		return nil
	}
	var result map[string]string
	se.WithLock(func() {
		result = make(map[string]string, len(se.sources))
		for path, source := range se.sources {
			if hierarchy := source.Virtualgw.Hierarchy(); len(hierarchy) > 0 {
				result[path] = source.Virtualgw.Leaf()
				continue
			}
			result[path] = ""
		}
	})
	return result
}

// LastModified returns when source enumeration last completed.
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

// Get returns a copy of cached source data for path.
func (se *SourcesEnumerator) Get(path string) ([]byte, bool) {
	if se == nil {
		return nil, false
	}
	var data []byte
	var ok bool
	se.WithLock(func() {
		if source, exists := se.sources[path]; exists {
			data = make([]byte, len(source.Data))
			copy(data, source.Data)
			ok = true
		}
	})
	return data, ok
}

// Snippets recompiles all sources into snippets, returning a map of path => snippet for the last enumeration.
// It is recommended to cache results by LastModified timestamp to avoid unnecessary recomputation.
func (se *SourcesEnumerator) Snippets(virtualgw string) map[string]compiler.Snippet {
	if se == nil {
		return nil
	}
	// Parse each snippet, store by path.
	sources := make(map[string]sourceData)
	se.WithLock(func() {
		for key, item := range se.sources {
			if slices.Contains(item.Virtualgw.Hierarchy(), virtualgw) {
				sources[key] = item
			}
		}
	})
	snippets := make(map[string]compiler.Snippet)
	for path, data := range sources {
		var decoded map[string]any
		if err := yaml.Unmarshal(data.Data, &decoded); err != nil {
			continue
		}
		snippet := compiler.Snippet{
			Ref:       data.Ref,
			Virtualgw: data.Virtualgw,
			Data:      decoded,
		}
		snippets[path] = snippet
	}
	return snippets
}

func (se *SourcesEnumerator) record(source compiler.Source) {
	if se == nil {
		return
	}
	if source.Ref.Path == "" {
		return
	}
	key := source.Ref.Key()
	data := make([]byte, len(source.Data))
	copy(data, source.Data)
	se.WithLock(func() {
		se.sources[key] = sourceData{
			Ref:       source.Ref,
			Virtualgw: source.Virtualgw,
			Data:      data,
		}
	})
}
