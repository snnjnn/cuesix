package ssl

import (
	"fmt"
	"strings"
)

type ProviderRouter struct {
	ACMEManager     Manager
	FileManager     Manager
	FallbackManager Manager
}

// ResolveProvider routes provider names to ACME, file, or fallback managers.
func (r ProviderRouter) ResolveProvider(name string) (Provider, error) {
	switch {
	case strings.HasPrefix(name, ACMEPrefix):
		if r.ACMEManager == nil {
			return nil, fmt.Errorf("acme manager missing")
		}
		return r.ACMEManager.ResolveProvider(name)
	case name == FileProviderName:
		if r.FileManager == nil {
			return nil, fmt.Errorf("file manager missing")
		}
		return r.FileManager.ResolveProvider(name)
	case name == FallbackPrefix:
		if r.FallbackManager == nil {
			return nil, fmt.Errorf("fallback manager missing")
		}
		return r.FallbackManager.ResolveProvider(name)
	default:
		return nil, fmt.Errorf("unknown provider: %s", name)
	}
}
