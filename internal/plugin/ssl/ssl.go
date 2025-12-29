package ssl

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strings"
	"time"
)

// SSLPlugin inlines certificate files referenced via file:// in ssls entries.
type SSLPlugin struct {
	Filesystems []fs.FS
	Expirations *ExpiryManager
}

func (p *SSLPlugin) Update(logger *slog.Logger, value map[string]any) (map[string]any, error) {
	if len(p.Filesystems) == 0 {
		return nil, errors.New("ssl plugin requires at least one filesystem")
	}
	logger.Info("ssl plugin start")
	if p.Expirations != nil {
		p.Expirations.ResetForConfig(time.Now())
	}
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
		if err := p.replaceField(logger, entry, "cert", true, "cert"); err != nil {
			return nil, err
		}
		if err := p.replaceField(logger, entry, "key", false, "key"); err != nil {
			return nil, err
		}
		if err := p.replaceListField(logger, entry, "certs", true, "certs"); err != nil {
			return nil, err
		}
		if err := p.replaceListField(logger, entry, "keys", false, "keys"); err != nil {
			return nil, err
		}
		if err := p.replaceNestedField(logger, entry, "client", "ca", true, "client.ca"); err != nil {
			return nil, err
		}
	}
	logger.Info("ssl plugin complete", "entries", len(list))
	return value, nil
}

func (p *SSLPlugin) replaceField(logger *slog.Logger, entry map[string]any, field string, track bool, logField string) error {
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
	if track {
		p.trackExpiration(logger, entry, logField, name, content)
	}
	return nil
}

func (p *SSLPlugin) replaceListField(logger *slog.Logger, entry map[string]any, field string, track bool, logField string) error {
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
		if track {
			p.trackExpiration(logger, entry, fmt.Sprintf("%s[%d]", logField, i), name, content)
		}
	}
	entry[field] = list
	return nil
}

func (p *SSLPlugin) replaceNestedField(logger *slog.Logger, entry map[string]any, parent, field string, track bool, logField string) error {
	raw, ok := entry[parent]
	if !ok {
		return nil
	}
	parentMap, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("ssl plugin expects %s to be a map, got %T", parent, raw)
	}
	return p.replaceField(logger, parentMap, field, track, logField)
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

func (p *SSLPlugin) trackExpiration(logger *slog.Logger, entry map[string]any, field string, name string, content string) {
	if p.Expirations == nil {
		return
	}
	snis := p.entrySNIs(logger, entry)
	if len(snis) == 0 {
		return
	}
	notAfter, err := parseCertNotAfter(content)
	if err != nil {
		logger.Warn("ssl plugin failed to parse certificate expiration", "field", field, "file", name, "error", err)
		return
	}
	for _, sni := range snis {
		p.Expirations.RecordSNI(sni, notAfter)
	}
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

func parseCertNotAfter(content string) (time.Time, error) {
	data := []byte(content)
	for len(data) > 0 {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		data = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return time.Time{}, err
		}
		return cert.NotAfter, nil
	}
	return time.Time{}, errors.New("no certificate data")
}
