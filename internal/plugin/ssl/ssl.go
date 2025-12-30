package ssl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"

	"github.com/warpcomdev/cuesix/internal/certmagicmgr"
	"github.com/warpcomdev/cuesix/internal/testutil"
)

// SSLPlugin inlines certificate files referenced via file:// in ssls entries.
type SSLPlugin struct {
	Filesystems []fs.FS
	ACME        ACMEManager
}

// ACMEManager provides access to ACME certificates.
type ACMEManager interface {
	RequestCertificate(ctx context.Context, logger *slog.Logger, providerName string, sni string) (certmagicmgr.Certificate, error)
	FallbackCertificate() (certmagicmgr.Certificate, error)
}

func (p *SSLPlugin) Update(logger *slog.Logger, value map[string]any) (map[string]any, error) {
	if len(p.Filesystems) == 0 {
		return nil, errors.New("ssl plugin requires at least one filesystem")
	}
	logger.Info("ssl plugin start")
	sslsRaw, ok := value["ssls"]
	if !ok {
		logger.Info("ssl plugin skipped: no ssls")
		return value, nil
	}
	list, ok := sslsRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("ssl plugin expects ssls to be a list, got %T", sslsRaw)
	}
	for i, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("ssl plugin expects ssls[%d] to be a map, got %T", i, item)
		}
		if err := p.handleACME(logger, entry); err != nil {
			return nil, err
		}
		if err := p.replaceField(logger, entry, "cert", "cert"); err != nil {
			return nil, err
		}
		if err := p.replaceField(logger, entry, "key", "key"); err != nil {
			return nil, err
		}
		if err := p.replaceListField(logger, entry, "certs", "certs"); err != nil {
			return nil, err
		}
		if err := p.replaceListField(logger, entry, "keys", "keys"); err != nil {
			return nil, err
		}
		if err := p.replaceNestedField(logger, entry, "client", "ca", "client.ca"); err != nil {
			return nil, err
		}
	}
	logger.Info("ssl plugin complete", "entries", len(list))
	return value, nil
}

func (p *SSLPlugin) replaceField(logger *slog.Logger, entry map[string]any, field string, logField string) error {
	raw, ok := entry[field]
	if !ok {
		return nil
	}
	text, ok := raw.(string)
	if !ok {
		return fmt.Errorf("ssl plugin expects %s to be a string, got %T", logField, raw)
	}
	if !strings.HasPrefix(text, "file://") {
		return nil
	}
	name := strings.TrimPrefix(text, "file://")
	if name == "" {
		return fmt.Errorf("ssl plugin empty file reference in %s", logField)
	}
	content, err := p.readFile(name)
	if err != nil {
		return err
	}
	entry[field] = content
	p.logReplacement(logger, logField, name)
	return nil
}

func (p *SSLPlugin) replaceListField(logger *slog.Logger, entry map[string]any, field string, logField string) error {
	raw, ok := entry[field]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("ssl plugin expects %s to be a list, got %T", logField, raw)
	}
	for i, item := range list {
		text, ok := item.(string)
		if !ok {
			return fmt.Errorf("ssl plugin expects %s[%d] to be a string, got %T", logField, i, item)
		}
		if !strings.HasPrefix(text, "file://") {
			continue
		}
		name := strings.TrimPrefix(text, "file://")
		if name == "" {
			return fmt.Errorf("ssl plugin empty file reference in %s[%d]", logField, i)
		}
		content, err := p.readFile(name)
		if err != nil {
			return err
		}
		list[i] = content
		p.logReplacement(logger, logField, name)
	}
	entry[field] = list
	return nil
}

func (p *SSLPlugin) replaceNestedField(logger *slog.Logger, entry map[string]any, parent, field string, logField string) error {
	raw, ok := entry[parent]
	if !ok {
		return nil
	}
	parentMap, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("ssl plugin expects %s to be a map, got %T", parent, raw)
	}
	return p.replaceField(logger, parentMap, field, logField)
}

func (p *SSLPlugin) logReplacement(logger *slog.Logger, field string, name string) {
	logger.Info("ssl plugin inlined file", "field", field, "file", name)
}

func (p *SSLPlugin) readFile(name string) (string, error) {
	for _, filesystem := range p.Filesystems {
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

func (p *SSLPlugin) entrySNIs(logger *slog.Logger, entry map[string]any) []string {
	raw, ok := entry["snis"]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		logger.Warn("ssl plugin invalid snis for expiry tracking", "type", fmt.Sprintf("%T", raw))
		return nil
	}
	snis := make([]string, 0, len(list))
	for i, item := range list {
		text, ok := item.(string)
		if !ok {
			logger.Warn("ssl plugin invalid sni for expiry tracking", "index", i, "type", fmt.Sprintf("%T", item))
			continue
		}
		if text != "" {
			snis = append(snis, text)
		}
	}
	return snis
}

func (p *SSLPlugin) handleACME(logger *slog.Logger, entry map[string]any) error {
	raw, ok := entry["cert"]
	if !ok {
		return nil
	}
	text, ok := raw.(string)
	if !ok {
		return fmt.Errorf("ssl plugin expects cert to be a string, got %T", raw)
	}
	if !strings.HasPrefix(text, "acme://") {
		return nil
	}
	if p.ACME == nil {
		return errors.New("ssl plugin acme requested but certmagic manager not configured")
	}
	provider := strings.TrimPrefix(text, "acme://")
	if provider == "" {
		return errors.New("ssl plugin empty acme provider")
	}
	snis := p.entrySNIs(logger, entry)
	if len(snis) != 1 {
		return errors.New("ssl plugin acme requires exactly one sni")
	}
	sni := snis[0]
	cert, err := p.ACME.RequestCertificate(context.Background(), testutil.Logger(), provider, sni)
	if err != nil {
		fallback, fallbackErr := p.ACME.FallbackCertificate()
		if fallbackErr != nil {
			return fmt.Errorf("ssl plugin acme failed: %w", err)
		}
		entry["cert"] = string(fallback.CertPEM)
		entry["key"] = string(fallback.KeyPEM)
		logger.Warn("ssl plugin acme failed, using fallback certificate", "provider", provider, "sni", sni, "error", err)
		return nil
	}
	entry["cert"] = string(cert.CertPEM)
	entry["key"] = string(cert.KeyPEM)
	logger.Info("ssl plugin acme certificate loaded", "provider", provider, "sni", sni)
	return nil
}
