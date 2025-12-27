package plugin

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

type SSLPlugin struct {
	Filesystems []fs.FS
}

func (p *SSLPlugin) Update(value map[string]any) (map[string]any, error) {
	if len(p.Filesystems) == 0 {
		return nil, errors.New("ssl plugin requires at least one filesystem")
	}
	sslsRaw, ok := value["ssls"]
	if !ok {
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
		if err := p.replaceField(entry, "cert"); err != nil {
			return nil, err
		}
		if err := p.replaceField(entry, "key"); err != nil {
			return nil, err
		}
		if err := p.replaceListField(entry, "certs"); err != nil {
			return nil, err
		}
		if err := p.replaceListField(entry, "keys"); err != nil {
			return nil, err
		}
		if err := p.replaceNestedField(entry, "client", "ca"); err != nil {
			return nil, err
		}
	}
	return value, nil
}

func (p *SSLPlugin) replaceField(entry map[string]any, field string) error {
	raw, ok := entry[field]
	if !ok {
		return nil
	}
	text, ok := raw.(string)
	if !ok {
		return fmt.Errorf("ssl plugin expects %s to be a string, got %T", field, raw)
	}
	if !strings.HasPrefix(text, "file://") {
		return nil
	}
	name := strings.TrimPrefix(text, "file://")
	if name == "" {
		return fmt.Errorf("ssl plugin empty file reference in %s", field)
	}
	content, err := p.readFile(name)
	if err != nil {
		return err
	}
	entry[field] = content
	return nil
}

func (p *SSLPlugin) replaceListField(entry map[string]any, field string) error {
	raw, ok := entry[field]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return fmt.Errorf("ssl plugin expects %s to be a list, got %T", field, raw)
	}
	for i, item := range list {
		text, ok := item.(string)
		if !ok {
			return fmt.Errorf("ssl plugin expects %s[%d] to be a string, got %T", field, i, item)
		}
		if !strings.HasPrefix(text, "file://") {
			continue
		}
		name := strings.TrimPrefix(text, "file://")
		if name == "" {
			return fmt.Errorf("ssl plugin empty file reference in %s[%d]", field, i)
		}
		content, err := p.readFile(name)
		if err != nil {
			return err
		}
		list[i] = content
	}
	entry[field] = list
	return nil
}

func (p *SSLPlugin) replaceNestedField(entry map[string]any, parent, field string) error {
	raw, ok := entry[parent]
	if !ok {
		return nil
	}
	parentMap, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("ssl plugin expects %s to be a map, got %T", parent, raw)
	}
	return p.replaceField(parentMap, field)
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
