package compiler

import (
	"fmt"
	"path"
	"strings"
)

const (
	DEFAULT_VIRTUALGW = "default"
	VIRTUALGW_SEP     = "."
)

// SourceRef uniquely identifies a source within a named input root.
type SourceRef struct {
	Namespace string
	Path      string
}

// VirtualGateway represents a hierarchical virtual gateway name.
type VirtualGateway struct {
	hierarchy []string
}

// Source represents a source configuration content.
type Source struct {
	Ref       SourceRef
	Virtualgw VirtualGateway
	Data      []byte
}

// Key returns a stable human-readable identifier suitable for caches.
func (ref SourceRef) Key() string {
	if ref.Namespace == "" {
		return ref.Path
	}
	if ref.Path == "" {
		return ref.Namespace + ":"
	}
	// Necesitamos quitar el primer "/" porque, en otro
	// caso, al construir un URL path usando este valor,
	// se duplica la barra y el resultado es que luego no
	// se encuentra la key en las cachés
	return strings.TrimPrefix(fmt.Sprintf("%s:%s", ref.Namespace, ref.Path), "/")
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
	return SourceRef{Namespace: ref.Namespace, Path: dir}
}

// Sibling returns a ref for a file in the same directory as the current path.
func (ref SourceRef) Sibling(name string) SourceRef {
	dir := ref.Dir().Path
	if dir == "" {
		return SourceRef{Namespace: ref.Namespace, Path: path.Clean(name)}
	}
	return SourceRef{Namespace: ref.Namespace, Path: path.Join(dir, name)}
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
