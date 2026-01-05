package certmagicmgr

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/warpcomdev/cuesix/internal/cursor"
	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
)

type Provider struct {
	cursor.Lock
	adapter CertMagic
	cfg     ProviderConfig
	cache   *certmagic.Cache
	magic   *certmagic.Config
	issuer  *certmagic.ACMEIssuer
	logger  *slog.Logger
}

// RequestCertificate obtains or loads a certificate for the given SNI.
func (p *Provider) RequestCertificate(ctx context.Context, sni string) error {
	if p == nil {
		return errors.New("provider is nil")
	}
	if p.magic == nil {
		return errors.New("provider magic is nil")
	}
	if strings.TrimSpace(sni) == "" {
		return errors.New("sni is required")
	}
	var err error
	p.WithLock(func() {
		err = p.adapter.ManageAsync(ctx, p.magic, []string{sni})
	})
	return err
}

func (p *Provider) Name() string {
	if p == nil {
		return ""
	}
	return p.cfg.Name
}

func (p *Provider) BestMatchFor(_ context.Context, sni string) (ssl.Certificate, bool) {
	if p == nil {
		return ssl.Certificate{}, false
	}
	if p.cache == nil {
		return ssl.Certificate{}, false
	}
	sni = strings.TrimSpace(sni)
	if sni == "" {
		return ssl.Certificate{}, false
	}
	var matches []certmagic.Certificate
	p.WithLock(func() {
		matches = p.adapter.AllMatchingCertificates(p.cache, sni)
	})
	return bestMatchForCandidates(matches, p.logger)
}

// RemoveManaged stops tracking the SNI for future listings.
func (p *Provider) RemoveManaged(_ context.Context, identities ...string) {
	if p == nil || p.magic == nil || p.cache == nil {
		return
	}
	p.WithLock(func() {
		for _, identity := range identities {
			for _, issuer := range p.magic.Issuers {
				c := certmagic.SubjectIssuer{
					Subject:   identity,
					IssuerKey: issuer.IssuerKey(),
				}
				p.adapter.RemoveManaged(p.cache, []certmagic.SubjectIssuer{c})
			}
		}
	})
}

func buildProvider(logger *slog.Logger, cfg Config, providerCfg ProviderConfig, adapter CertMagic, storage Storage, events chan ssl.Tracking) *Provider {
	if logger == nil {
		logger = slog.Default()
	}
	p := &Provider{
		adapter: adapter,
		cfg:     providerCfg,
		logger:  logger,
	}
	configTemplate := *certmagic.NewDefault()
	storage.UpdateConfig(&configTemplate)
	if events != nil {
		configTemplate.OnEvent = func(ctx context.Context, event string, data map[string]any) error {
			logger.Info("certmagic event", "event", event, "data", data)
			if event == "cert_obtained" {
				if sni, ok := data["identifier"].(string); ok {
					select {
					case <-ctx.Done():
						return nil
					case events <- ssl.Tracking{Identity: sni, Provider: p.Name()}:
					default:
					}
				}
			}
			return nil
		}
	}
	p.cache = certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(cert certmagic.Certificate) (*certmagic.Config, error) {
			return p.magic, nil
		},
	})
	p.magic = certmagic.New(p.cache, configTemplate)
	issuerCfg := certmagic.ACMEIssuer{
		CA:     providerCfg.CA,
		Email:  providerCfg.Email,
		Agreed: true,
		// we assume it is apisix who will terminate SSL, not us.
		// SO we need to disable the ALPN challenge.
		DisableTLSALPNChallenge: true,
	}
	if cfg.CertObtainTimeout > 0 {
		issuerCfg.CertObtainTimeout = cfg.CertObtainTimeout
	}
	p.issuer = certmagic.NewACMEIssuer(p.magic, issuerCfg)
	p.magic.Issuers = []certmagic.Issuer{p.issuer}
	return p
}

func MarshalCertificate(cert certmagic.Certificate) (ssl.Certificate, error) {
	if cert.PrivateKey == nil {
		return ssl.Certificate{}, errors.New("private key is nil")
	}
	var certPEM []byte
	for _, der := range cert.Certificate.Certificate {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: der,
		})...)
	}
	if len(certPEM) == 0 {
		return ssl.Certificate{}, errors.New("empty certificate chain")
	}
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		return ssl.Certificate{}, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: key,
	})
	var notAfter time.Time
	if cert.Leaf != nil {
		notAfter = cert.Leaf.NotAfter
	} else {
		var err error
		notAfter, err = leafNotAfter(cert.Certificate.Certificate)
		if err != nil {
			return ssl.Certificate{}, err
		}
	}
	return ssl.Certificate{
		CertPEM:  certPEM,
		KeyPEM:   keyPEM,
		NotAfter: notAfter,
	}, nil
}

func leafNotAfter(chain [][]byte) (time.Time, error) {
	if len(chain) == 0 {
		return time.Time{}, errors.New("missing certificate chain")
	}
	leaf, err := x509.ParseCertificate(chain[0])
	if err != nil {
		return time.Time{}, err
	}
	return leaf.NotAfter, nil
}

func parseLeaf(chain [][]byte) (*x509.Certificate, error) {
	if len(chain) == 0 {
		return nil, errors.New("missing certificate chain")
	}
	return x509.ParseCertificate(chain[0])
}

func bestMatchForCandidates(matches []certmagic.Certificate, logger *slog.Logger) (ssl.Certificate, bool) {
	if logger == nil {
		logger = slog.Default()
	}
	type candidate struct {
		cert     certmagic.Certificate
		notAfter time.Time
	}
	candidates := make([]candidate, 0, len(matches))
	for _, match := range matches {
		if match.Empty() {
			continue
		}
		leaf := match.Leaf
		if leaf == nil {
			parsed, err := parseLeaf(match.Certificate.Certificate)
			if err != nil {
				logger.Error("parse leaf cert", "error", err)
				continue
			}
			leaf = parsed
		}
		candidates = append(candidates, candidate{
			cert:     match,
			notAfter: leaf.NotAfter,
		})
	}
	if len(candidates) == 0 {
		return ssl.Certificate{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].notAfter.After(candidates[j].notAfter)
	})
	for _, cand := range candidates {
		cert, err := MarshalCertificate(cand.cert)
		if err != nil {
			logger.Error("marshal cert", "error", err)
			continue
		}
		return cert, true
	}
	return ssl.Certificate{}, false
}
