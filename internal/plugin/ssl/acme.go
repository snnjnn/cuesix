package ssl

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// ACMEManager provides access to ACME certificates.
type ACMEManager interface {
	RequestCertificate(ctx context.Context, logger *slog.Logger, providerName string, sni string) error
	ClearTracking(*slog.Logger)
	Update(ctx context.Context, buffer int, action func(provider, sni string, cert Certificate))
}

type ACMEHandler struct {
	AcmeManager ACMEManager
}

func (a ACMEHandler) replaceTargets(logger *slog.Logger, targets []certTargets, fallback Certificate) {
	targetsBySNI := make(map[string][]certTargets)
	for _, target := range targets {
		if a.AcmeManager == nil {
			logger.Error("ssl plugin acme requires acme manager and tracker", "target", target)
			target.replace(fallback.CertPEM, fallback.KeyPEM)
			continue
		}
		if len(target.snis) != 1 {
			logger.Error("ssl plugin acme requires exactly one sni", "target", target)
			target.replace(fallback.CertPEM, fallback.KeyPEM)
			continue
		}
		sni := target.snis[0]
		targetsBySNI[sni] = append(targetsBySNI[sni], target)
	}
	if len(targetsBySNI) == 0 {
		return
	}
	// Clear ACME tracking, since we are about to overwrite it
	a.AcmeManager.ClearTracking(logger)
	cancelCtx, cancelFunc := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFunc()
	var (
		lock sync.Mutex
		wg   sync.WaitGroup
	)
	certsBySNI := make(map[string]Certificate)
	pendingTargets := make(map[string]struct{})
	for sni := range targetsBySNI {
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
	wg.Go(func() {
		defer close(ready)
		ready <- struct{}{}
		a.AcmeManager.Update(cancelCtx, 2*len(targetsBySNI), func(provider, sni string, cert Certificate) {
			if !cert.NotAfter.IsZero() {
				if clearPending(sni) {
					certsBySNI[sni] = cert
				}
			}
		})
	})
	<-ready
	for sni, targets := range targetsBySNI {
		sniSuccess := false
		for _, target := range targets {
			provider := strings.TrimPrefix(target.cert, acmePrefix)
			err := a.AcmeManager.RequestCertificate(cancelCtx, logger, provider, sni)
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
	for sni, targets := range targetsBySNI {
		if cert, ok := certsBySNI[sni]; ok {
			for _, target := range targets {
				target.replace(cert.CertPEM, cert.KeyPEM)
			}
		} else {
			for _, target := range targets {
				target.replace(fallback.CertPEM, fallback.KeyPEM)
			}
		}
	}
}
