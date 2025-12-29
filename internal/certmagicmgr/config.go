package certmagicmgr

import (
	"fmt"
	"strings"
	"time"
)

// ParseProviderSpec parses a comma-separated provider definition.
// Example: "name=letsencrypt,ca=https://...,email=ops@example.com,timeout=30s"
func ParseProviderSpec(spec string) (ProviderConfig, error) {
	var cfg ProviderConfig
	parts := strings.Split(spec, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return ProviderConfig{}, fmt.Errorf("invalid provider field: %q", part)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "name":
			cfg.Name = value
		case "ca":
			cfg.CA = value
		case "email":
			cfg.Email = value
		case "timeout":
			if value == "" {
				cfg.Timeout = 0
				continue
			}
			parsed, err := time.ParseDuration(value)
			if err != nil {
				return ProviderConfig{}, fmt.Errorf("invalid timeout: %w", err)
			}
			cfg.Timeout = parsed
		default:
			return ProviderConfig{}, fmt.Errorf("unknown provider field: %s", key)
		}
	}
	return cfg, nil
}
