package compiler

import (
	"fmt"
	"io/fs"
	"iter"
	"log/slog"
	"path/filepath"
	"sort"

	"go.yaml.in/yaml/v4"
)

func NewEnumerator(logger *slog.Logger) DefaultEnumerator {
	return DefaultEnumerator{Logger: logger}
}

type DefaultEnumerator struct {
	Logger *slog.Logger
}

type Source struct {
	// Since fs.FS does not support equality, we use FSID to identify different filesystems.
	FS   fs.FS
	FSID int
	Path string
	Data []byte
}

func (be DefaultEnumerator) Enumerate(fss ...fs.FS) iter.Seq2[Source, error] {
	return Enumerate(be.Logger, fss...)
}

func Enumerate(logger *slog.Logger, fses ...fs.FS) iter.Seq2[Source, error] {
	if logger == nil {
		logger = slog.Default()
	}
	return func(yield func(Source, error) bool) {
		for index, filesystem := range fses {
			paths, err := listYAMLFiles(filesystem)
			if err != nil {
				if !yield(Source{}, err) {
					return
				}
				continue
			}
			for _, path := range paths {
				logger.Info("enumerator located file", "path", path)
				content, err := fs.ReadFile(filesystem, path)
				if err != nil {
					if !yield(Source{}, fmt.Errorf("enumerator read %s: %w", path, err)) {
						return
					}
					continue
				}
				if !yield(Source{FSID: index, FS: filesystem, Path: path, Data: content}, nil) {
					return
				}
			}
		}
	}
}

func NewFetcher(logger *slog.Logger, enumerator Enumerator) DefaultFetcher {
	return DefaultFetcher{
		Logger:     logger,
		Enumerator: enumerator,
	}
}

type Enumerator interface {
	Enumerate(...fs.FS) iter.Seq2[Source, error]
}

type DefaultFetcher struct {
	Logger     *slog.Logger
	Enumerator Enumerator
}

func (bf DefaultFetcher) Fetch(fss ...fs.FS) iter.Seq2[Snippet, error] {
	return Fetch(bf.Logger, bf.Enumerator.Enumerate(fss...))
}

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
				if !yield(Snippet{}, fmt.Errorf("decode %s: %w", source.Path, err)) {
					return
				}
				continue
			}
			if !yield(Snippet{Path: source.Path, Data: value}, nil) {
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

func listYAMLFiles(filesystem fs.FS) ([]string, error) {
	var paths []string
	err := fs.WalkDir(filesystem, ".", func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext == ".yaml" || ext == ".yml" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	return paths, nil
}
