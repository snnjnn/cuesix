package compiler

import (
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DEFAULT_VIRTUALGW = "default"
	VIRTUALGW_SEP     = "."
)

// VirtualGateway represents a hierarchical virtual gateway name.
type VirtualGateway struct {
	hierarchy []string
}

// FromKey builds a hierarchical virtual gateway name from a key.
// They key is considered to be a dot-separated list of nested gateways.
func FromKey(name string) VirtualGateway {
	parts := strings.Split(name, VIRTUALGW_SEP)
	clean := make([]string, 0, len(parts))
	for _, parts := range parts {
		if c := strings.TrimSpace(parts); c != "" {
			clean = append(clean, c)
		}
	}
	if len(clean) == 0 {
		clean = []string{DEFAULT_VIRTUALGW}
	}
	return FromLeaf(clean)
}

// FromKey builds a hierarchical virtual gateway name from a Leaf path.
// The leaf path is the longest gateway list in the hierarchy.
func FromLeaf(leaf []string) VirtualGateway {
	hierarchy := make([]string, 0, len(leaf))
	for i := len(leaf); i > 0; i-- {
		hierarchy = append(hierarchy, strings.Join(leaf[:i], VIRTUALGW_SEP))
	}
	return VirtualGateway{
		hierarchy: hierarchy,
	}
}

// Key returns the leaf virtual gateway in the hierarchy
func (vg VirtualGateway) Leaf() string {
	return vg.hierarchy[0]
}

// Hierarchy returns the complete virtual gateway hierarchy.
func (vg VirtualGateway) Hierarchy() []string {
	return vg.hierarchy
}

// Source represents a source configuration content.
type Source struct {
	FS        fs.FS
	Ref       SourceRef
	Data      []byte
	Virtualgw VirtualGateway
}

// Resolver resolves a path within a filesystem to a Virtualgw name
type Resolver interface {
	Virtualgw(root InputRoot, path string) (VirtualGateway, error)
}

// Enumerator enumerates source files from a set of filesystems.
type Enumerator interface {
	Enumerate(...InputRoot) iter.Seq2[Source, error]
}

// NewEnumerator returns the default source enumerator.
func NewEnumerator(logger *slog.Logger, resolver Resolver) DefaultEnumerator {
	return DefaultEnumerator{
		Logger:   logger,
		Resolver: resolver,
	}
}

type DefaultEnumerator struct {
	Logger   *slog.Logger
	Resolver Resolver
}

// Enumerate lists YAML files from each filesystem and yields raw sources.
func (be DefaultEnumerator) Enumerate(roots ...InputRoot) iter.Seq2[Source, error] {
	return Enumerate(be.Logger, be.Resolver, roots...)
}

// DefaultResolver resolves all paths to the same default virtual gateway.
type DefaultResolver struct {
	VirtualGateway
}

func (dr DefaultResolver) Virtualgw(root InputRoot, path string) (VirtualGateway, error) {
	return dr.VirtualGateway, nil
}

// Enumerate lists YAML files from each filesystem and yields raw sources.
func Enumerate(logger *slog.Logger, resolver Resolver, roots ...InputRoot) iter.Seq2[Source, error] {
	if logger == nil {
		logger = slog.Default()
	}
	return func(yield func(Source, error) bool) {
		if resolver == nil {
			yield(Source{}, errors.New("resolver cannot be nil"))
			return
		}
		for _, root := range roots {
			paths, err := listYAMLFiles(root.FS)
			if err != nil {
				if !yield(Source{}, err) {
					return
				}
				continue
			}
			for _, path := range paths {
				ref := SourceRef{Root: root.Name, Path: path}
				logger.Info("enumerator located file", "source", ref.Key())
				virtualgw, err := resolver.Virtualgw(root, path)
				if err != nil {
					if !yield(Source{}, fmt.Errorf("enumerator resolve virtualgw for %s: %w", path, err)) {
						return
					}
					continue
				}
				content, err := fs.ReadFile(root.FS, path)
				if err != nil {
					if !yield(Source{}, fmt.Errorf("enumerator read %s: %w", path, err)) {
						return
					}
					continue
				}
				source := Source{
					FS:        root.FS,
					Ref:       ref,
					Data:      content,
					Virtualgw: virtualgw,
				}
				if !yield(source, nil) {
					return
				}
			}
		}
	}
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
