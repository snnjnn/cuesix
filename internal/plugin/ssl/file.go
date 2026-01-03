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
	if logger == nil {
		logger = slog.Default()
	}
	if len(f.Filesystems) == 0 {
		logger.Error("ssl plugin requires at least one filesystem")
	}
	for _, target := range targets {
		certBytes, certErr := f.resolveValue(target.cert)
		keyBytes, keyErr := f.resolveValue(target.key)
		if certErr != nil {
			logger.Error("ssl plugin failed to resolve cert", "error", certErr, "sslid", target.sslId, "snis", target.snis)
			certBytes = fallback.CertPEM
		}
		if keyErr != nil {
			logger.Error("ssl plugin failed to resolve key", "error", keyErr, "sslid", target.sslId, "snis", target.snis)
			keyBytes = fallback.KeyPEM
		}
		target.replace(certBytes, keyBytes)
	}
}

func (f FileHandler) resolveValue(text string) ([]byte, error) {
	if strings.HasPrefix(text, FilePrefix) {
		name := strings.TrimPrefix(text, FilePrefix)
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
