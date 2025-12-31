package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/warpcomdev/cuesix/internal/certmagicmgr"
)

// stringSliceFlag collects repeated string flags.
type stringSliceFlag struct {
	values []string
	set    bool
}

// String returns the comma-separated values.
func (s *stringSliceFlag) String() string {
	return strings.Join(s.values, ",")
}

// Set appends a new flag value.
func (s *stringSliceFlag) Set(value string) error {
	s.set = true
	s.values = append(s.values, value)
	return nil
}

// buildFilesystems creates read-only filesystems for the input paths.
func buildFilesystems(paths []string) ([]fs.FS, error) {
	fses := make([]fs.FS, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		clean := filepath.Clean(path)
		if _, err := os.Stat(clean); err != nil {
			return nil, err
		}
		fses = append(fses, os.DirFS(clean))
	}
	return fses, nil
}

// splitComma splits a comma-separated list and trims whitespace.
func splitComma(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// splitSemicolon splits a semicolon-separated list and trims whitespace.
func splitSemicolon(value string) []string {
	parts := strings.Split(value, ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// buildCertmagicProviders parses certmagic provider specs.
func buildCertmagicProviders(specs []string) ([]certmagicmgr.ProviderConfig, error) {
	if len(specs) == 0 {
		return nil, errors.New("at least one certmagic provider is required")
	}
	providers := make([]certmagicmgr.ProviderConfig, 0, len(specs))
	for _, spec := range specs {
		cfg, err := certmagicmgr.ParseProviderSpec(spec)
		if err != nil {
			return nil, err
		}
		providers = append(providers, cfg)
	}
	return providers, nil
}

// envString reads a raw environment variable.
func envString(key string) string {
	return os.Getenv(key)
}

// envStringDefault reads an environment variable or returns a default.
func envStringDefault(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}

// envBool reads a boolean environment variable or returns a default.
func envBool(key string, def bool) bool {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		switch strings.ToLower(value) {
		case "1", "true", "yes", "y", "on":
			return true
		case "0", "false", "no", "n", "off":
			return false
		default:
			return def
		}
	}
	return def
}

// envInt reads an integer environment variable or returns a default.
func envInt(key string, def int) int {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return def
}

// envFloat reads a float environment variable or returns a default.
func envFloat(key string, def float64) float64 {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		var parsed float64
		if _, err := fmt.Sscanf(value, "%f", &parsed); err == nil {
			return parsed
		}
	}
	return def
}

// envDuration reads a duration environment variable or returns a default.
func envDuration(key string, def time.Duration) time.Duration {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return def
}
