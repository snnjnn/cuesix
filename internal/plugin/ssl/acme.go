package ssl

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/warpcomdev/cuesix/internal/cursor"
)

const DefaultACMERequestTimeout = 10 * time.Second

type ACMETracker interface {
	RequestCertificate(ctx context.Context, providerName string, sni string) (ACMEKey, error)
	Watch(buffer int, topic string) cursor.Owned[ACMECertificate]
}

type ACMEHandler struct {
	// Shared tracker to avoid hitting upstream manager with repeated manage / unmanage requests
	Tracker ACMETracker
	// RequestTimeout bounds the time spent waiting for ACME certificates.
	RequestTimeout time.Duration
	// Record of certificates requested to the tracker by this handler
	Record map[ACMEKey]time.Time
}

func (a ACMEHandler) replaceTargets(logger *slog.Logger, targets []certTargets, record map[ACMEKey]time.Time, fallback Certificate) {
	if len(targets) == 0 {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	targetsBySNI := make(map[string][]certTargets)
	if a.Tracker == nil {
		logger.Error("ssl plugin acme requires acme tracker")
	}
	// First step: collect SNIs
	for _, target := range targets {
		if a.Tracker == nil {
			target.replace(fallback.CertPEM, fallback.KeyPEM)
			continue
		}
		if len(target.snis) != 1 {
			logger.Error("ssl plugin acme requires exactly one sni", "sslid", target.sslId, "snis", target.snis)
			target.replace(fallback.CertPEM, fallback.KeyPEM)
			continue
		}
		sni := target.snis[0]
		targetsBySNI[sni] = append(targetsBySNI[sni], target)
	}
	if len(targetsBySNI) == 0 {
		return
	}
	// Now we will request all the certs, while waiting for notifications
	timeout := a.RequestTimeout
	if timeout <= 0 {
		timeout = DefaultACMERequestTimeout
	}
	cancelCtx, cancelFunc := context.WithTimeout(context.Background(), timeout)
	defer cancelFunc()
	var (
		lock sync.Mutex
		wg   sync.WaitGroup
	)
	certsBySNI := make(map[string]ACMECertificate)
	pendingTargets := make(map[string]struct{})
	for sni := range targetsBySNI {
		pendingTargets[sni] = struct{}{}
	}
	// Keep track of how many pending certificates are there
	// Pending certificates can be cleared if:
	// - A certificate is received
	// - The async certificate request fails
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
		events := a.Tracker.Watch(2*len(targetsBySNI), "")
		defer events.Close()
		close(ready) // signal the main thread
		for cert := range cursor.All(cancelCtx, events.Cursor) {
			if !cert.NotAfter.IsZero() && clearPending(cert.SNI) {
				certsBySNI[cert.SNI] = cert
			}
		}
	})
	// Wait until the watcher is ready, and begin requesting targets
	<-ready
	for sni, targets := range targetsBySNI {
		sniSuccess := false
		for _, target := range targets {
			provider := strings.TrimPrefix(target.cert, ACMEPrefix)
			key, err := a.Tracker.RequestCertificate(cancelCtx, provider, sni)
			if err == nil {
				if record != nil {
					// Track the request.
					// We do not provide a time because we don't have any at this point.
					record[key] = time.Time{}
				}
				sniSuccess = true
				break
			}
			logger.Error("ssl plugin acme request failed", "sslid", target.sslId, "provider", provider, "sni", sni, "err", err)
		}
		if !sniSuccess {
			clearPending(sni)
		}
	}
	wg.Wait()
	// Finally, perform the replacement
	for sni, targets := range targetsBySNI {
		if cert, ok := certsBySNI[sni]; ok {
			if record != nil {
				record[cert.ACMEKey] = cert.NotAfter
			}
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
