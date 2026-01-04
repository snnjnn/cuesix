package ssl

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"slices"
	"strings"
	"time"
)

// ACMECertificate contains an ACME certificate
type Certificate struct {
	CertPEM  []byte
	KeyPEM   []byte
	NotAfter time.Time
}

// SSLPlugin resolves cert/key markers and ensures entries never remain invalid.
type SSLPlugin struct {
	Fallback Certificate
	TextHandler
	FileHandler
	LiveHandler
	Logger *slog.Logger
}

// This function describes a closure that updates a (cert, key) pair
type certUpdater func(cert, key []byte)

// This type describes the cert and key literal
// used in a `ssls` object of apisix (either in the
// `cert`, `key` fields, or the `certs`, `keys` list),
// together with the list of SNIs in that same entry.
type certTargets struct {
	sslId   string
	cert    string
	key     string
	snis    []string
	replace certUpdater
}

// This describes types of ssl replacements supported
type targetType int

const (
	textTarget targetType = iota
	fileTarget
	acmeTarget
)

const (
	ACMEPrefix = "acme://"
	FilePrefix = "file://"
)

func (p *SSLPlugin) Update(ctx context.Context, value map[string]any, record map[Tracking]time.Time) (map[string]any, error) {
	if p == nil {
		return nil, errors.New("ssl plugin is nil")
	}
	if len(p.Fallback.CertPEM) == 0 || len(p.Fallback.KeyPEM) == 0 {
		return nil, errors.New("ssl plugin requires a fallback certificate")
	}
	if value == nil {
		return nil, errors.New("value map is nil")
	}
	logger := p.Logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("ssl plugin start")
	sslsRaw, ok := value["ssls"]
	if !ok {
		logger.Info("ssl plugin skipped: no ssls")
		return value, nil
	}
	entries, ok := sslsRaw.([]any)
	if !ok {
		return nil, fmt.Errorf("ssl plugin expects ssls to be a list, got %T", sslsRaw)
	}
	if len(entries) == 0 {
		logger.Info("ssl plugin skipped: empty ssls")
		return value, nil
	}
	targets, err := p.collectTargets(entries)
	if err != nil {
		return nil, err
	}
	p.TextHandler.replaceTargets(logger, targets[textTarget], p.Fallback)
	p.FileHandler.replaceTargets(logger, targets[fileTarget], p.Fallback)
	p.LiveHandler.replaceTargets(ctx, logger, targets[acmeTarget], record, p.Fallback)
	logger.Info("ssl plugin complete", "entries", len(entries))
	return value, nil
}

func asStringSlice(input any) []string {
	stringList, ok := input.([]string)
	if !ok {
		anyList, ok := input.([]any)
		if !ok {
			return nil
		}
		stringList = make([]string, len(anyList))
		for i, item := range anyList {
			stringList[i], ok = item.(string)
			if !ok {
				return nil
			}
		}
	}
	for idx, item := range stringList {
		stringList[idx] = strings.TrimSpace(item)
	}
	return stringList
}

func (p *SSLPlugin) certPairs(entry map[string]any) ([]string, []string) {
	if p == nil || entry == nil {
		return nil, nil
	}
	certsRaw, certsOk := entry["certs"]
	keysRaw, keysOk := entry["keys"]
	if !certsOk || !keysOk {
		return nil, nil
	}
	certsList := asStringSlice(certsRaw)
	keysList := asStringSlice(keysRaw)
	if len(keysList) != len(certsList) {
		return nil, nil
	}
	if len(keysList) == 0 {
		return nil, nil
	}
	return certsList, keysList
}

func (p *SSLPlugin) collectTargets(entries []any) (map[targetType][]certTargets, error) {
	if p == nil {
		return nil, errors.New("ssl plugin is nil")
	}
	targets := map[targetType][]certTargets{
		textTarget: make([]certTargets, 0, len(entries)),
		fileTarget: make([]certTargets, 0, len(entries)),
		acmeTarget: make([]certTargets, 0, len(entries)),
	}
	for i, item := range entries {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("ssl plugin expects ssls[%d] to be a map, got %T", i, item)
		}
		p.collectEntryTargets(entry, targets)
	}
	return targets, nil
}

func (p *SSLPlugin) collectEntryTargets(entry map[string]any, targets map[targetType][]certTargets) {
	if p == nil || entry == nil || targets == nil {
		return
	}
	var sslIdString string
	sslId, ok := entry["id"]
	if ok {
		if sslIdString, ok = sslId.(string); !ok {
			sslIdString = fmt.Sprintf("%v", sslId)
		}
	}
	snis := p.entrySNIs(entry)
	p.collectSinglePair(entry, sslIdString, snis, targets)
	p.collectListPairs(entry, sslIdString, snis, targets)
}

func (p *SSLPlugin) collectSinglePair(entry map[string]any, id string, snis []string, targets map[targetType][]certTargets) {
	if p == nil || entry == nil || targets == nil {
		return
	}
	cert, certOk := entry["cert"]
	key, keyOk := entry["key"]
	if !certOk || !keyOk {
		return
	}
	certText, certOk := cert.(string)
	keyText, keyOk := key.(string)
	if !certOk || !keyOk {
		return
	}
	targetKind := resolveTargetType(certText, keyText)
	targets[targetKind] = append(targets[targetKind], certTargets{
		sslId: id,
		cert:  certText,
		key:   keyText,
		snis:  snis,
		replace: func(cert, key []byte) {
			entry["cert"] = string(cert)
			entry["key"] = string(key)
		},
	})
}

func (p *SSLPlugin) collectListPairs(entry map[string]any, id string, snis []string, targets map[targetType][]certTargets) {
	if p == nil || entry == nil || targets == nil {
		return
	}
	certs, keys := p.certPairs(entry)
	if len(certs) == 0 || len(keys) == 0 {
		return
	}
	entry["certs"] = certs
	entry["keys"] = keys
	for idx, certText := range certs {
		keyText := keys[idx]
		targetKind := resolveTargetType(certText, keyText)
		index := idx
		targets[targetKind] = append(targets[targetKind], certTargets{
			sslId: id,
			cert:  certText,
			key:   keyText,
			snis:  snis,
			replace: func(cert, key []byte) {
				certs[index] = string(cert)
				keys[index] = string(key)
			},
		})
	}
}

func resolveTargetType(certText, keyText string) targetType {
	if strings.HasPrefix(certText, ACMEPrefix) {
		return acmeTarget
	}
	if strings.HasPrefix(certText, FilePrefix) {
		return fileTarget
	}
	if strings.HasPrefix(keyText, FilePrefix) {
		return fileTarget
	}
	return textTarget
}

func (p *SSLPlugin) entrySNIs(entry map[string]any) []string {
	if p == nil || entry == nil {
		return nil
	}
	raw, ok := entry["snis"]
	if !ok {
		return nil
	}
	snis := make(map[string]struct{})
	for _, item := range asStringSlice(raw) {
		if item != "" {
			snis[item] = struct{}{}
		}
	}
	return slices.Collect(maps.Keys(snis))
}

func LoadFallbackCertificate(certPath string, keyPath string) (Certificate, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return Certificate{}, fmt.Errorf("read fallback cert: %w", err)
	}
	if len(certPEM) == 0 {
		return Certificate{}, errors.New("fallback cert is empty")
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return Certificate{}, fmt.Errorf("read fallback key: %w", err)
	}
	if len(keyPEM) == 0 {
		return Certificate{}, errors.New("fallback key is empty")
	}
	notAfter, err := parseCertNotAfter(certPEM)
	if err != nil {
		return Certificate{}, fmt.Errorf("parse fallback cert: %w", err)
	}
	return Certificate{CertPEM: certPEM, KeyPEM: keyPEM, NotAfter: notAfter}, nil
}

func parseCertNotAfter(certPEM []byte) (time.Time, error) {
	rest := certPEM
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return time.Time{}, err
		}
		return cert.NotAfter, nil
	}
	return time.Time{}, errors.New("fallback cert missing certificate block")
}
