package ssl

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/warpcomdev/cuesix/internal/cursor"
)

const DefaultACMERequestTimeout = 10 * time.Second

type LiveTracker interface {
	ResolveProvider(providerName string, cache *ProviderCache) (Provider, error)
	Watch(buffer int, topic string) cursor.Owned[Delivery]
}

type LiveHandler struct {
	// Shared tracker to avoid hitting upstream manager with repeated manage / unmanage requests
	Tracker LiveTracker
	// RequestTimeout bounds the time spent waiting for ACME certificates.
	RequestTimeout time.Duration
}

func (a LiveHandler) replaceTargets(ctx context.Context, logger *slog.Logger, targets []certTargets, record map[Tracking]time.Time, fallback Certificate) {
	if len(targets) == 0 {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	targetsById := make(map[Tracking][]certTargets)
	if a.Tracker == nil {
		logger.Error("ssl plugin requires live tracker")
	}
	// First step: collect providers and identities
	var cache ProviderCache
	for _, target := range targets {
		if a.Tracker == nil {
			target.replace(fallback.CertPEM, fallback.KeyPEM)
			continue
		}
		tracking, err := target.tracking()
		if err != nil {
			logger.Error("ssl plugin failed to build tracking", "sslid", target.sslId, "snis", target.snis, "error", err)
		}
		if tracking.Provider == "" {
			target.replace(fallback.CertPEM, fallback.KeyPEM)
			continue
		}
		provider, err := a.Tracker.ResolveProvider(tracking.Provider, &cache)
		if err != nil {
			logger.Error("ssl plugin failed to resolve provider", "sslid", target.sslId, "cert", target.cert, "error", err)
			target.replace(fallback.CertPEM, fallback.KeyPEM)
			continue
		}
		key := Tracking{Provider: provider.Name(), Identity: tracking.Identity}
		targetsById[key] = append(targetsById[key], target)
	}
	if len(targetsById) == 0 {
		logger.Error("ssl plugin live failed to resolve any provider")
		return
	}
	// Now we will request all the certs, while waiting for notifications
	timeout := a.RequestTimeout
	if timeout <= 0 {
		timeout = DefaultACMERequestTimeout
	}
	cancelCtx, cancelFunc := context.WithTimeout(ctx, timeout)
	defer cancelFunc()
	var (
		lock sync.Mutex
		wg   sync.WaitGroup
	)
	certsById := make(map[Tracking]Delivery)
	pendingTargets := make(map[Tracking]struct{})
	for key := range targetsById {
		pendingTargets[key] = struct{}{}
	}
	// Keep track of how many pending certificates are there
	// Pending certificates can be cleared if:
	// - A certificate is received
	// - The async certificate request fails
	clearPending := func(key Tracking) bool {
		lock.Lock()
		before := len(pendingTargets)
		delete(pendingTargets, key)
		after := len(pendingTargets)
		lock.Unlock()
		if after == 0 {
			cancelFunc()
		}
		return before > after
	}
	ready := make(chan struct{}, 1)
	wg.Go(func() {
		events := a.Tracker.Watch(2*len(targetsById), "")
		defer events.Close()
		close(ready) // signal the main thread
		for cert := range cursor.All(cancelCtx, events.Cursor) {
			if !cert.NotAfter.IsZero() && clearPending(cert.Tracking) {
				certsById[cert.Tracking] = cert
			}
		}
	})
	// Wait until the watcher is ready, and begin requesting targets
	<-ready
	for key := range targetsById {
		provider, err := a.Tracker.ResolveProvider(key.Provider, &cache)
		if err == nil {
			if err = provider.RequestCertificate(ctx, key.Identity); err == nil {
				if record != nil {
					// Track the request.
					// We do not provide a time because we don't have any at this point.
					record[key] = time.Time{}
				}
				continue
			}
		}
		logger.Error("ssl plugin acme request failed", "provider", provider, "key", key, "err", err)
		clearPending(key)
	}
	wg.Wait()
	// Finally, perform the replacement
	for key, targets := range targetsById {
		if cert, ok := certsById[key]; ok {
			if record != nil {
				record[cert.Tracking] = cert.NotAfter
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
