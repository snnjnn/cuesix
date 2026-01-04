package ssl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"
)

type FileManager struct {
	Filesystems []fs.FS
}

func (m FileManager) ResolveProvider(name string) (Provider, error) {
	if name != FilePrefix {
		return nil, fmt.Errorf("unknown file provider: %s", name)
	}
	return &fileProvider{
		filesystems: m.Filesystems,
	}, nil
}

type fileProvider struct {
	filesystems []fs.FS
}

func (p fileProvider) Name() string {
	return FilePrefix
}

func (p fileProvider) BestMatchFor(_ context.Context, identity string) (Certificate, bool) {
	certPath, keyPath, err := parseFileIdentity(identity)
	if err != nil {
		return Certificate{}, false
	}
	certPEM, err := readFileFromFS(p.filesystems, certPath)
	if err != nil {
		return Certificate{}, false
	}
	keyPEM, err := readFileFromFS(p.filesystems, keyPath)
	if err != nil {
		return Certificate{}, false
	}
	notAfter, err := parseCertNotAfter(certPEM)
	if err != nil {
		return Certificate{}, false
	}
	return Certificate{
		CertPEM:  certPEM,
		KeyPEM:   keyPEM,
		NotAfter: notAfter,
	}, true
}

func (p fileProvider) RequestCertificate(ctx context.Context, identity string) error {
	if _, ok := p.BestMatchFor(ctx, identity); !ok {
		return fmt.Errorf("file provider missing certificate for %s", identity)
	}
	return nil
}

func (p fileProvider) RemoveManaged(_ context.Context, _ ...string) {
}

func parseFileIdentity(identity string) (string, string, error) {
	certPath, keyPath, found := strings.Cut(identity, "+")
	if !found || certPath == "" || keyPath == "" {
		return "", "", errors.New("invalid file identity")
	}
	cleanCert, err := sanitizePath(certPath)
	if err != nil {
		return "", "", err
	}
	cleanKey, err := sanitizePath(keyPath)
	if err != nil {
		return "", "", err
	}
	return cleanCert, cleanKey, nil
}

func sanitizePath(name string) (string, error) {
	clean := path.Clean(name)
	if clean == "." || clean == "/" || strings.HasPrefix(clean, "/") || strings.HasPrefix(clean, "..") {
		return "", fmt.Errorf("invalid file path: %s", name)
	}
	return clean, nil
}

func readFileFromFS(filesystems []fs.FS, name string) ([]byte, error) {
	clean, err := sanitizePath(name)
	if err != nil {
		return nil, err
	}
	for _, filesystem := range filesystems {
		data, err := fs.ReadFile(filesystem, clean)
		if err == nil {
			return data, nil
		}
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		return nil, fmt.Errorf("ssl plugin read %s: %w", name, err)
	}
	return nil, fmt.Errorf("ssl plugin missing file: %s", name)
}
