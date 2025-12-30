package ssl

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/warpcomdev/cuesix/internal/certmagicmgr"
)

// SSLPlugin resolves cert/key markers and ensures entries never remain invalid.
type SSLPlugin struct {
	Filesystems []fs.FS
	ACME        ACMEManager
	Fallback    certmagicmgr.Certificate
}

// ACMEManager provides access to ACME certificates.
type ACMEManager interface {
	RequestCertificate(ctx context.Context, logger *slog.Logger, providerName string, sni string) error
	Subscribe(buffer int) chan certmagicmgr.Notification
	Unsubscribe(ch chan certmagicmgr.Notification)
	ClearTracking(*slog.Logger)
}

// This function describes a closure that updates a (cert, key) pair
type certUpdater func(cert, key []byte)

// This type describes the cert and key literal
// used in a `ssls` object of apisix (either in the
// `cert`, `key` fields, or the `certs`, `keys` list),
// together with the list of SNIs in that same entry.
type certTargets struct {
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

func (p *SSLPlugin) Update(logger *slog.Logger, value map[string]any) (map[string]any, error) {
	if len(p.Fallback.CertPEM) == 0 || len(p.Fallback.KeyPEM) == 0 {
		return nil, errors.New("ssl plugin requires a fallback certificate")
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
	if len(list) == 0 {
		logger.Info("ssl plugin skipped: empty ssls")
		return value, nil
	}
	targets := map[targetType][]certTargets{
		textTarget: make([]certTargets, 0, len(list)),
		fileTarget: make([]certTargets, 0, len(list)),
		acmeTarget: make([]certTargets, 0, len(list)),
	}
	for i, item := range list {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("ssl plugin expects ssls[%d] to be a map, got %T", i, item)
		}
		snis := p.entrySNIs(entry)
		cert, certOk := entry["cert"]
		key, keyOk := entry["key"]
		if certOk && keyOk {
			certText, certOk := cert.(string)
			keyText, keyOk := key.(string)
			if certOk && keyOk {
				targetType := p.resolveType(cert, key)
				targets[targetType] = append(targets[targetType], certTargets{
					cert: certText,
					key:  keyText,
					snis: snis,
					replace: func(cert, key []byte) {
						entry["cert"] = string(cert)
						entry["key"] = string(key)
					},
				})
			}
		}
		certs, keys := p.certPairs(entry)
		if len(certs) > 0 && len(keys) > 0 {
			entry["certs"] = certs
			entry["keys"] = keys
			for i, certText := range certs {
				keyText := keys[i]
				targetType := p.resolveType(certText, keyText)
				targets[targetType] = append(targets[targetType], certTargets{
					cert: certText,
					key:  keyText,
					snis: snis,
					replace: func(cert, key []byte) {
						certs[i] = string(cert)
						keys[i] = string(key)
					},
				})
			}
		}
	}
	p.replaceTextTargets(logger, targets[textTarget])
	p.replaceFileTargets(logger, targets[fileTarget])
	p.replaceAcmeTargets(logger, targets[acmeTarget])
	logger.Info("ssl plugin complete", "entries", len(list))
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

func (p *SSLPlugin) resolveType(certRef, keyRef any) targetType {
	certStr, ok := certRef.(string)
	if !ok {
		return textTarget
	}
	keyStr, ok := keyRef.(string)
	if !ok {
		return textTarget
	}
	if strings.HasPrefix(certStr, "acme://") {
		return acmeTarget
	}
	if strings.HasPrefix(certStr, "file://") {
		return fileTarget
	}
	if strings.HasPrefix(keyStr, "file://") {
		return fileTarget
	}
	return textTarget
}

func (p *SSLPlugin) replaceTextTargets(logger *slog.Logger, targets []certTargets) {
	// no op, se dejan como están
}

func (p *SSLPlugin) replaceFileTargets(logger *slog.Logger, targets []certTargets) {
	if len(targets) == 0 {
		return
	}
	for _, target := range targets {
		certBytes, certErr := p.resolveValue(target.cert)
		keyBytes, keyErr := p.resolveValue(target.key)
		if certErr != nil {
			logger.Error("ssl plugin failed to resolve cert", "error", certErr)
			certBytes = p.Fallback.CertPEM
		}
		if keyErr != nil {
			logger.Error("ssl plugin failed to resolve key", "error", keyErr)
			keyBytes = p.Fallback.KeyPEM
		}
		target.replace(certBytes, keyBytes)
	}
}

func (p *SSLPlugin) resolveValue(text string) ([]byte, error) {
	if strings.HasPrefix(text, "file://") {
		if len(p.Filesystems) == 0 {
			return nil, errors.New("ssl plugin requires at least one filesystem")
		}
		name := strings.TrimPrefix(text, "file://")
		if name == "" {
			return nil, errors.New("ssl plugin empty file reference")
		}
		content, err := p.readFile(name)
		if err != nil {
			return nil, err
		}
		return []byte(content), nil
	}
	return []byte(text), nil
}

func (p *SSLPlugin) replaceAcmeTargets(logger *slog.Logger, targets []certTargets) {
	validTargets := make(map[string][]certTargets)
	for _, target := range targets {
		if p.ACME == nil {
			logger.Error("ssl plugin acme requires acme manager", "target", target)
			target.replace(p.Fallback.CertPEM, p.Fallback.KeyPEM)
			continue
		}
		if len(target.snis) != 1 {
			logger.Error("ssl plugin acme requires exactly one sni", "target", target)
			target.replace(p.Fallback.CertPEM, p.Fallback.KeyPEM)
			continue
		}
		sni := target.snis[0]
		validTargets[sni] = append(validTargets[sni], target)
	}
	if len(validTargets) == 0 {
		return
	}
	// Clear ACME tracking, since we are about to overwrite it
	p.ACME.ClearTracking(logger)
	cancelCtx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFunc()
	var (
		lock sync.Mutex
		wg   sync.WaitGroup
	)
	validCerts := make(map[string]certmagicmgr.Certificate)
	pendingTargets := make(map[string]struct{})
	for sni := range validTargets {
		pendingTargets[sni] = struct{}{}
	}
	clearPending := func(sni string) bool {
		lock.Lock()
		before := len(pendingTargets)
		delete(pendingTargets, sni)
		after := len(pendingTargets)
		lock.Unlock()
		if after == 0 {
			cancelFunc()
		}
		return before > after
	}
	ready := make(chan struct{}, 1)
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(ready)
		subs := p.ACME.Subscribe(2 * len(validTargets))
		defer p.ACME.Unsubscribe(subs)
		ready <- struct{}{}
		for {
			select {
			case updated, ok := <-subs:
				if !ok {
					return
				}
				if !updated.Cert.NotAfter.IsZero() {
					if clearPending(updated.SNI) {
						validCerts[updated.SNI] = updated.Cert
					}
				}
			case <-cancelCtx.Done():
				return
			}
		}
	}()
	<-ready
	for sni, targets := range validTargets {
		sniSuccess := false
		for _, target := range targets {
			provider := strings.TrimPrefix(target.cert, "acme://")
			err := p.ACME.RequestCertificate(cancelCtx, logger, provider, sni)
			if err == nil {
				sniSuccess = true
				break
			}
			logger.Error("ssl plugin acme request failed", "provider", provider, "sni", sni, "err", err)
		}
		if !sniSuccess {
			clearPending(sni)
		}
	}
	wg.Wait()
	for sni, targets := range validTargets {
		if cert, ok := validCerts[sni]; ok {
			for _, target := range targets {
				target.replace(cert.CertPEM, cert.KeyPEM)
			}
		} else {
			for _, target := range targets {
				target.replace(p.Fallback.CertPEM, p.Fallback.KeyPEM)
			}
		}
	}
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

func (p *SSLPlugin) entrySNIs(entry map[string]any) []string {
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
