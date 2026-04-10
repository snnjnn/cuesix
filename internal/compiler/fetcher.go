package compiler

import (
	"fmt"
	"iter"
	"log/slog"

	"go.yaml.in/yaml/v4"
)

// Enumerate lists YAML files from each filesystem and yields raw sources.
type Enumerator interface {
	Enumerate() iter.Seq2[Source, error]
}

// NewFetcher returns the default snippet fetcher.
func NewFetcher(logger *slog.Logger, enumerator Enumerator) DefaultFetcher {
	return DefaultFetcher{
		Logger:     logger,
		Enumerator: enumerator,
	}
}

// DefaultFetcher decodes enumerated YAML sources into snippets.
type DefaultFetcher struct {
	Logger     *slog.Logger
	Enumerator Enumerator
}

// Fetch decodes enumerated YAML sources into snippets.
func (bf DefaultFetcher) Fetch() iter.Seq2[Snippet, error] {
	return Fetch(bf.Logger, bf.Enumerator.Enumerate())
}

// Fetch decodes enumerated YAML sources into snippets.
func Fetch(logger *slog.Logger, enumeration iter.Seq2[Source, error]) iter.Seq2[Snippet, error] {
	if logger == nil {
		logger = slog.Default()
	}
	return func(yield func(Snippet, error) bool) {
		for source, err := range enumeration {
			if err != nil {
				if !yield(Snippet{}, err) {
					return
				}
				continue
			}
			if len(source.Data) == 0 {
				if !yield(Snippet{}, fmt.Errorf("empty file")) {
					return
				}
				continue
			}
			value, err := decodeYAML(source.Data)
			if err != nil {
				if !yield(Snippet{}, fmt.Errorf("decode %s: %w", source.Ref.Path, err)) {
					return
				}
				continue
			}
			if !yield(Snippet{Ref: source.Ref, Virtualgw: source.Virtualgw, Data: value}, nil) {
				return
			}
		}
	}
}

func decodeYAML(content []byte) (map[string]any, error) {
	var value map[string]any
	if err := yaml.Unmarshal(content, &value); err != nil {
		return nil, err
	}
	return value, nil
}
