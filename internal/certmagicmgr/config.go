package certmagicmgr

import (
	"fmt"
	"strings"

	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
)

const ProviderFieldSeparator = "|"

// ParseACMEProviderSpec parses a pipe-separated provider definition.
// Format: "name|email|ca"
// Returns a ProviderConfig with the ACME prefix added to the name, or an error if the format is invalid.
func ParseACMEProviderSpec(spec string) (ProviderConfig, error) {
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
	// ACME providers must include acme prefix
	cfg.Name = ssl.ACMEPrefix + cfg.Name
	return cfg, nil
}
