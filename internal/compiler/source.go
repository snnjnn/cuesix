package compiler

import (
	"fmt"
	"io/fs"
	"path"
	"strings"
)

// InputRoot identifies an input filesystem and the name used for source refs under it.
type InputRoot struct {
	Name string
	FS   fs.FS
}

// SourceRef uniquely identifies a source within a named input root.
type SourceRef struct {
	Root string
	Path string
}

// Key returns a stable human-readable identifier suitable for caches.
func (ref SourceRef) Key() string {
	if ref.Root == "" {
		return ref.Path
	}
	if ref.Path == "" {
		return ref.Root + ":"
	}
	// Necesitamos quitar el primer "/" porque, en otro
	// caso, al construir un URL path usando este valor,
	// se duplica la barra y el resultado es que luego no
	// se encuentra la key en las cachés
	return strings.TrimPrefix(fmt.Sprintf("%s:%s", ref.Root, ref.Path), "/")
}

// Dir returns a ref pointing to the parent directory of the source path.
func (ref SourceRef) Dir() SourceRef {
	clean := strings.TrimPrefix(path.Clean(ref.Path), "./")
	if clean == "." {
		clean = ""
	}
	dir := path.Dir(clean)
	if dir == "." {
		dir = ""
	}
	return SourceRef{Root: ref.Root, Path: dir}
}

// Sibling returns a ref for a file in the same directory as the current path.
func (ref SourceRef) Sibling(name string) SourceRef {
	dir := ref.Dir().Path
	if dir == "" {
		return SourceRef{Root: ref.Root, Path: path.Clean(name)}
	}
	return SourceRef{Root: ref.Root, Path: path.Join(dir, name)}
}
