package compiler

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
)

// Input manages namespaces and sources in those namespaces.
type Input interface {
	Namespaces() ([]string, error)
	Enumerate(namespace string) iter.Seq2[SourceRef, error]
	Open(ref SourceRef) (io.ReadCloser, error)
}

// Resolver helps managing Sources
type Resolver interface {
	Virtualgw(ref SourceRef) (VirtualGateway, error)
}

// NewEnumerator returns the default source enumerator.
func NewEnumerator(logger *slog.Logger, input Input, resolver Resolver) Enumerator {
	return InputEnumerator{
		Logger:   logger,
		Input:    input,
		Resolver: resolver,
	}
}

type InputEnumerator struct {
	Logger   *slog.Logger
	Input    Input
	Resolver Resolver
}

// Enumerate lists YAML files from each filesystem and yields raw sources.
func (be InputEnumerator) Enumerate() iter.Seq2[Source, error] {
	logger := be.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return func(yield func(Source, error) bool) {
		if be.Input == nil {
			yield(Source{}, errors.New("input cannot be nil"))
			return
		}
		if be.Resolver == nil {
			yield(Source{}, errors.New("resolver cannot be nil"))
			return
		}
		namespaces, err := be.Input.Namespaces()
		if err != nil {
			yield(Source{}, fmt.Errorf("enumerator namespaces: %w", err))
			return
		}
		for _, namespace := range namespaces {
			for ref, err := range be.Input.Enumerate(namespace) {
				if err != nil {
					if !yield(Source{}, fmt.Errorf("enumerator enumerate namespace %s: %w", namespace, err)) {
						return
					}
					continue
				}
				data, err := func() ([]byte, error) {
					reader, err := be.Input.Open(ref)
					if err != nil {
						return nil, fmt.Errorf("enumerator open %s: %w", ref.Key(), err)
					}
					defer reader.Close()
					content, err := io.ReadAll(reader)
					if err != nil {
						return nil, fmt.Errorf("enumerator read %s: %w", ref.Key(), err)
					}
					return content, nil
				}()
				if err != nil {
					if !yield(Source{}, err) {
						return
					}
					continue
				}
				virtualgw, err := be.Resolver.Virtualgw(ref)
				if err != nil {
					if !yield(Source{}, fmt.Errorf("enumerator resolve virtualgw for %s: %w", ref.Key(), err)) {
						return
					}
					continue
				}
				source := Source{
					Ref:       ref,
					Data:      data,
					Virtualgw: virtualgw,
				}
				if !yield(source, nil) {
					return
				}
			}
		}
	}
}

// DefaultResolver resolves all paths to the same default virtual gateway.
type DefaultResolver struct {
	VirtualGateway
}

func (dr DefaultResolver) Virtualgw(ref SourceRef) (VirtualGateway, error) {
	return dr.VirtualGateway, nil
}

// Input manages namespaces and sources in those namespaces.
type DefaultInput struct {
	roots map[string]fs.FS
	order []string
}

// InputFromFS builds a DefaultInput from an existing filesystem map.
// Filesystems missing from order are appended sorted by namespace.
func InputFromFS(roots map[string]fs.FS, order []string) DefaultInput {
	ownedRoots := make(map[string]fs.FS, len(roots))
	for namespace, filesystem := range roots {
		ownedRoots[namespace] = filesystem
	}
	finalOrder := make([]string, 0, len(ownedRoots))
	seen := make(map[string]struct{}, len(ownedRoots))
	for _, namespace := range order {
		if _, ok := ownedRoots[namespace]; !ok {
			continue
		}
		if _, ok := seen[namespace]; ok {
			continue
		}
		finalOrder = append(finalOrder, namespace)
		seen[namespace] = struct{}{}
	}
	var remaining []string
	for namespace := range ownedRoots {
		if _, ok := seen[namespace]; ok {
			continue
		}
		remaining = append(remaining, namespace)
	}
	sort.Strings(remaining)
	finalOrder = append(finalOrder, remaining...)
	return DefaultInput{
		roots: ownedRoots,
		order: finalOrder,
	}
}

// InputFromPaths builds a DefaultInput from local directory paths.
func InputFromPaths(paths []string) (DefaultInput, error) {
	roots := make(map[string]fs.FS)
	order := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		clean := filepath.Clean(path)
		if _, err := os.Stat(clean); err != nil {
			return DefaultInput{}, err
		}
		order = append(order, clean)
		roots[clean] = os.DirFS(clean)
	}
	return InputFromFS(roots, order), nil
}

// Namespaces implements Input
func (fi DefaultInput) Namespaces() ([]string, error) {
	return fi.order, nil
}

func (fi DefaultInput) Filesystems() []fs.FS {
	fsys := make([]fs.FS, 0, len(fi.order))
	for _, ns := range fi.order {
		fsys = append(fsys, fi.roots[ns])
	}
	return fsys
}

func (fi DefaultInput) Enumerate(namespace string) iter.Seq2[SourceRef, error] {
	return func(yield func(SourceRef, error) bool) {
		fs, ok := fi.roots[namespace]
		if !ok {
			yield(SourceRef{}, fmt.Errorf("namespace %s not found", namespace))
			return
		}
		paths, err := listYAMLFiles(fs)
		if err != nil {
			yield(SourceRef{}, fmt.Errorf("enumerate namespace %s: %w", namespace, err))
			return
		}
		for _, path := range paths {
			if !yield(SourceRef{Namespace: namespace, Path: path}, nil) {
				return
			}
		}
	}
}

// Open implements Input
func (fi DefaultInput) Open(ref SourceRef) (io.ReadCloser, error) {
	fs, ok := fi.roots[ref.Namespace]
	if !ok {
		return nil, fmt.Errorf("namespace %s not found", ref.Namespace)
	}
	file, err := fs.Open(ref.Path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", ref.Key(), err)
	}
	return file, nil
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
