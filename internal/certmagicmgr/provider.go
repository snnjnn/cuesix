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
	"github.com/warpcomdev/sixpack/internal/cursor"
	"github.com/warpcomdev/sixpack/internal/plugin/ssl"
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
	if p == nil || p.cache == nil || p.adapter == nil {
		return nil, false
	}
	sni = strings.TrimSpace(sni)
	if sni == "" {
		return nil, false
	}
	var matches []certmagic.Certificate
	p.WithLock(func() {
		matches = p.adapter.AllMatchingCertificates(p.cache, sni)
	})
	return bestMatchForCandidates(matches, p.logger)
}

// RemoveManaged stops tracking the SNI for future listings.
func (p *Provider) RemoveManaged(_ context.Context, identities ...string) {
	if p == nil || p.magic == nil || p.cache == nil || p.adapter == nil {
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
			if event != "cert_obtained" {
				logger.Info("certmagic event ignored", "event", event, "provider", p.Name())
				return nil
			}
			sni, ok := data["identifier"].(string)
			if !ok {
				logger.Warn("certmagic event missing identifier", "event", event, "provider", p.Name())
				return nil
			}
			notify := ssl.Tracking{Identity: sni, Provider: p.Name()}
			select {
			case <-ctx.Done():
				return nil
			case events <- notify:
				logger.Info("certmagic event queued", "event", event, "provider", p.Name(), "sni", sni)
			default:
				logger.Warn("certmagic event dropped", "event", event, "provider", p.Name(), "sni", sni)
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
		// Disable TLS-ALPN challenge, keep HTTP challenge enabled
		DisableTLSALPNChallenge: true,
		// Use alternate HTTP port for challenges
		AltHTTPPort: cfg.ChallengePort,
	}
	if cfg.CertObtainTimeout > 0 {
		issuerCfg.CertObtainTimeout = cfg.CertObtainTimeout
	}
	p.issuer = certmagic.NewACMEIssuer(p.magic, issuerCfg)
	p.magic.Issuers = []certmagic.Issuer{p.issuer}
	return p
}

type certmagicWrap struct {
	cert     certmagic.Certificate
	notAfter time.Time
}

func (c certmagicWrap) NotAfterTime() time.Time {
	return c.notAfter
}

func (c certmagicWrap) PEM() (ssl.PEMCertificate, error) {
	return MarshalCertificate(c.cert)
}

func MarshalCertificate(cert certmagic.Certificate) (ssl.PEMCertificate, error) {
	if cert.PrivateKey == nil {
		return ssl.PEMCertificate{}, errors.New("private key is nil")
	}
	var certPEM []byte
	for _, der := range cert.Certificate.Certificate {
		certPEM = append(certPEM, pem.EncodeToMemory(&pem.Block{
			Type:  "CERTIFICATE",
			Bytes: der,
		})...)
	}
	if len(certPEM) == 0 {
		return ssl.PEMCertificate{}, errors.New("empty certificate chain")
	}
	key, err := x509.MarshalPKCS8PrivateKey(cert.PrivateKey)
	if err != nil {
		return ssl.PEMCertificate{}, err
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
			return ssl.PEMCertificate{}, err
		}
	}
	return ssl.PEMCertificate{
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
		if match.PrivateKey == nil {
			logger.Error("missing private key")
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
		return nil, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].notAfter.After(candidates[j].notAfter)
	})
	for _, cand := range candidates {
		return certmagicWrap{cert: cand.cert, notAfter: cand.notAfter}, true
	}
	return nil, false
}
