package certmagicmgr

import (
	"context"
	"errors"
	"log/slog"
	"maps"
	"sync"
	"time"

	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
)

type trackKey struct {
	provider string
	sni      string
}

type Notification struct {
	Provider string
	SNI      string
	Cert     ssl.Certificate
}

// Watcher tracks certificate updates from certmagic.
type Watcher struct {
	manager    *Manager
	lock       sync.Mutex
	events     chan CertEvent
	track      map[trackKey]trackedCertificate
	watch      map[chan Notification]struct{}
	generation time.Time
}

// NewWatcher builds a watcher for certificate updates.
func NewWatcher(manager *Manager, events chan CertEvent) (*Watcher, error) {
	if events == nil {
		return nil, errors.New("events channel is required")
	}
	return &Watcher{
		manager:    manager,
		events:     events,
		track:      make(map[trackKey]trackedCertificate),
		watch:      make(map[chan Notification]struct{}),
		generation: time.Now(),
	}, nil
}

type trackedCertificate struct {
	ssl.Certificate
	TrackedAt time.Time
}

// RequestCertificate obtains or loads a certificate for the given SNI.
func (w *Watcher) RequestCertificate(ctx context.Context, logger *slog.Logger, providerName string, sni string) error {
	providerView, err := w.manager.resolveProviderView(providerName)
	if err != nil {
		return err
	}
	resolvedName := providerView.Name()
	key := trackKey{provider: resolvedName, sni: sni}
	// Lets first check if the certificate is tracked
	w.withLock(func() {
		if tracked, ok := w.track[key]; ok {
			// Refresh the generation
			tracked.TrackedAt = time.Now()
			w.track[key] = tracked
			// If the certificate is ready, broadcast it
			if !tracked.NotAfter.IsZero() {
				notif := Notification{
					Provider: key.provider,
					SNI:      key.sni,
					Cert:     tracked.Certificate,
				}
				w.notifyAllLocked(notif)
			}
			return
		}
		// If not tracked, lock it before proceeding
		w.track[key] = trackedCertificate{
			Certificate: ssl.Certificate{},
			TrackedAt:   time.Now(),
		}
	})
	err = w.manager.RequestCertificate(ctx, logger, resolvedName, sni)
	if err != nil {
		w.withLock(func() {
			delete(w.track, key)
		})
		return err
	}
	// Let's briefly ask the cache, in case it was
	// added before we started listening.
	best, ok := providerView.BestMatchFor(sni, logger)
	if ok {
		w.withLock(func() {
			w.track[key] = trackedCertificate{
				Certificate: best,
				TrackedAt:   time.Now(),
			}
			w.notifyAllLocked(Notification{
				Provider: key.provider,
				SNI:      key.sni,
				Cert:     best,
			})
		})
	}
	return err
}

// RemoveUntracked stops tracking the SNI for future listings
func (w *Watcher) RemoveUntracked(ctx context.Context, logger *slog.Logger, gracePeriod time.Duration) error {
	candidates := w.collectUntracked(gracePeriod)
	if len(candidates) == 0 {
		return nil
	}
	providerMap := make(map[string]providerView)
	removed := make(map[trackKey]trackedCertificate)
	// Find the provider for each candidate, and remove it.
	for key, tracked := range candidates {
		provider, err := w.providerFor(key.provider, providerMap)
		if err != nil {
			continue
		}
		provider.RemoveManaged(logger, key.sni)
		removed[key] = tracked
	}
	return w.rollbackUnmanaged(ctx, logger, removed)
}

func (w *Watcher) Subscribe(buffer int) chan Notification {
	var ch chan Notification
	w.withLock(func() {
		ch = make(chan Notification, max(buffer, len(w.watch)))
		w.watch[ch] = struct{}{}
	})
	return ch
}

func (w *Watcher) Unsubscribe(ch chan Notification) {
	w.withLock(func() {
		delete(w.watch, ch)
	})
	close(ch)
}

func (w *Watcher) ClearTracking(logger *slog.Logger) {
	w.withLock(func() {
		w.generation = time.Now()
	})
}

// Keeps sending certificate updates to all listeners
func (w *Watcher) RunWatch(ctx context.Context, logger *slog.Logger, refresh time.Duration) {
	ticker := time.NewTicker(refresh)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case certInfo, ok := <-w.events:
			if !ok {
				return
			}
			if certInfo.provider == nil {
				logger.Error("missing provider on cert event", "sni", certInfo.sni)
				continue
			}
			best, ok := certInfo.provider.BestMatchFor(certInfo.sni, logger)
			if !ok {
				continue
			}
			w.withLock(func() {
				key := trackKey{provider: certInfo.provider.Name(), sni: certInfo.sni}
				if tracked, ok := w.track[key]; ok {
					if tracked.NotAfter.IsZero() || tracked.NotAfter.Before(best.NotAfter) {
						tracked.Certificate = best
						w.track[key] = tracked
					}
					return
				}
				// Always notify of real renovations, just in case.
				w.notifyAllLocked(Notification{
					Provider: key.provider,
					SNI:      key.sni,
					Cert:     best,
				})
			})
		case <-ticker.C:
			// Periodically, walk over all certificates, to see
			// if some of them has been fixed.
			// First, snapshot all tracked certificates, to query them
			snapshot := make(map[trackKey]trackedCertificate)
			w.withLock(func() {
				maps.Copy(snapshot, w.track)
			})
			providerMap := make(map[string]providerView)
			for key, cachedCert := range snapshot {
				provider, err := w.providerFor(key.provider, providerMap)
				if err != nil {
					logger.Error("failed to resolve provider", "error", err)
					continue
				}
				// Query the best certificate, and update the snapshot
				// if it is more recent that whatever we had
				best, ok := provider.BestMatchFor(key.sni, logger)
				if !ok {
					continue
				}
				if cachedCert.NotAfter.IsZero() || best.NotAfter.After(cachedCert.NotAfter) {
					cachedCert.Certificate = best
					snapshot[key] = cachedCert
				}
			}
			// Now, refresh the real list with the snapshot
			w.withLock(func() {
				for key, best := range snapshot {
					if tracked, ok := w.track[key]; ok && best.NotAfter.After(tracked.NotAfter) {
						logger.Info("certificate updated", "sni", key.sni, "provider", key.provider)
						tracked.Certificate = best.Certificate
						// Preserve the certificate generation. Do not overwrite
						// using the snapshot genration. Only update cert.
						w.track[key] = tracked
						w.notifyAllLocked(Notification{
							Provider: key.provider,
							SNI:      key.sni,
							Cert:     tracked.Certificate,
						})
					}
				}
			})
		}
	}
}

func (w *Watcher) collectUntracked(gracePeriod time.Duration) map[trackKey]trackedCertificate {
	candidates := make(map[trackKey]trackedCertificate)
	w.withLock(func() {
		deadline := w.generation.Add(-gracePeriod)
		for key, tracked := range w.track {
			if tracked.TrackedAt.Before(deadline) {
				candidates[key] = tracked
				delete(w.track, key)
			}
		}
	})
	return candidates
}

func (w *Watcher) rollbackUnmanaged(ctx context.Context, logger *slog.Logger, removed map[trackKey]trackedCertificate) error {
	rollback := make(map[trackKey]struct{})
	w.withLock(func() {
		for key := range removed {
			if _, ok := w.track[key]; ok {
				rollback[key] = struct{}{}
			}
		}
	})
	err := make([]error, 0, len(rollback))
	for key := range rollback {
		err = append(err, w.manager.RequestCertificate(ctx, logger, key.provider, key.sni))
	}
	return errors.Join(err...)
}

func (w *Watcher) providerFor(name string, cache map[string]providerView) (providerView, error) {
	if provider, ok := cache[name]; ok {
		return provider, nil
	}
	provider, err := w.manager.resolveProviderView(name)
	if err != nil {
		return nil, err
	}
	cache[name] = provider
	return provider, nil
}

func (w *Watcher) withLock(fn func()) {
	w.lock.Lock()
	defer w.lock.Unlock()
	fn()
}

func (w *Watcher) notifyAllLocked(notif Notification) {
	for watcher := range w.watch {
		select {
		case watcher <- notif:
		default:
		}
	}
}
