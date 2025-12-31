package ssl

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
)

type FileHandler struct {
	Filesystems []fs.FS
}

func (f FileHandler) replaceTargets(logger *slog.Logger, targets []certTargets, fallback Certificate) {
	if len(targets) == 0 {
		return
	}
	for _, target := range targets {
		certBytes, certErr := f.resolveValue(target.cert)
		keyBytes, keyErr := f.resolveValue(target.key)
		if certErr != nil {
			logger.Error("ssl plugin failed to resolve cert", "error", certErr)
			certBytes = fallback.CertPEM
		}
		if keyErr != nil {
			logger.Error("ssl plugin failed to resolve key", "error", keyErr)
			keyBytes = fallback.KeyPEM
		}
		target.replace(certBytes, keyBytes)
	}
}

func (f FileHandler) resolveValue(text string) ([]byte, error) {
	if strings.HasPrefix(text, filePrefix) {
		if len(f.Filesystems) == 0 {
			return nil, errors.New("ssl plugin requires at least one filesystem")
		}
		name := strings.TrimPrefix(text, filePrefix)
		if name == "" {
			return nil, errors.New("ssl plugin empty file reference")
		}
		content, err := f.readFile(name)
		if err != nil {
			return nil, err
		}
		return []byte(content), nil
	}
	return []byte(text), nil
}

func (f FileHandler) readFile(name string) (string, error) {
	for _, filesystem := range f.Filesystems {
		data, err := fs.ReadFile(filesystem, name)
		if err == nil {
			return string(data), nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		return "", fmt.Errorf("ssl plugin read %s: %w", name, err)
	}
	return "", fmt.Errorf("ssl plugin missing file: %s", name)
}
