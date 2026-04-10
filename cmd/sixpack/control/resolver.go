package control

import (
	"path/filepath"
	"strings"

	"github.com/warpcomdev/cuesix/internal/compiler"
)

type GatewayFromDots struct {
	compiler.Resolver
}

// Virtualgw derives the virtual gateway name from the directory name:
//  1. if the directory name is not empty, and has one dot or more, then:
//     - split it by ".",
//     - trim whitespaces from each part,
//     - ignore empty parts,
//     Then  join the parts with ".", and that is the gateway.
//  2. Otherwise, return the resolver value.
func (resolver GatewayFromDots) Virtualgw(ref compiler.SourceRef) (compiler.VirtualGateway, error) {
	dirName := filepath.Base(filepath.Dir(filepath.Clean(ref.Path)))
	if dirName != "" {
		parts := strings.Split(dirName, ".")
		cleanParts := make([]string, 0, len(parts))
		for _, part := range parts {
			if s := strings.TrimSpace(part); s != "" {
				cleanParts = append(cleanParts, s)
			}
		}
		if len(cleanParts) > 1 {
			return compiler.FromLeaf(cleanParts), nil
		}
	}
	return resolver.Resolver.Virtualgw(ref)
}
