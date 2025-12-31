package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/warpcomdev/cuesix/internal/certmagicmgr"
)

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
