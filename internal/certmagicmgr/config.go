package certmagicmgr

import (
	"fmt"
	"strings"
)

const ProviderFieldSeparator = "|"

// ParseProviderSpec parses a pipe-separated provider definition.
// Format: "name|email|ca"
func ParseProviderSpec(spec string) (ProviderConfig, error) {
	var cfg ProviderConfig
	parts := strings.Split(spec, ProviderFieldSeparator)
	if len(parts) != 3 {
		return ProviderConfig{}, fmt.Errorf("invalid provider format, expected name|email|ca")
	}
	cfg.Name = strings.TrimSpace(parts[0])
	cfg.Email = strings.TrimSpace(parts[1])
	cfg.CA = strings.TrimSpace(parts[2])
	if cfg.Name == "" || cfg.Email == "" || cfg.CA == "" {
		return ProviderConfig{}, fmt.Errorf("provider requires name, email, and ca")
	}
	return cfg, nil
}
