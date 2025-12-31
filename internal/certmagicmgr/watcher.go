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
	manager *Manager
	lock    sync.Mutex
	events  chan CertEvent
	track   map[trackKey]ssl.Certificate
	watch   map[chan Notification]struct{}
	cleared bool
}

// NewWatcher builds a watcher for certificate updates.
func NewWatcher(manager *Manager, events chan CertEvent) (*Watcher, error) {
	if events == nil {
		return nil, errors.New("events channel is required")
	}
	return &Watcher{
		manager: manager,
		events:  events,
		track:   make(map[trackKey]ssl.Certificate),
		watch:   make(map[chan Notification]struct{}),
	}, nil
}

// RequestCertificate obtains or loads a certificate for the given SNI.
func (w *Watcher) RequestCertificate(ctx context.Context, logger *slog.Logger, providerName string, sni string) error {
	providerView, err := w.manager.resolveProviderView(providerName)
	if err != nil {
		return err
	}
	resolvedName := providerView.Name()
	key := trackKey{provider: resolvedName, sni: sni}
	w.withLock(func() {
		if _, ok := w.track[key]; !ok {
			w.track[key] = ssl.Certificate{}
		}
	})
	err = w.manager.RequestCertificate(ctx, logger, resolvedName, sni)
	if err != nil {
		w.withLock(func() {
			delete(w.track, key)
		})
		return err
	}
	// The caller is probably waiting for the certificate to be ready.
	// If it's already available, we don't need to wait for the event.
	w.withLock(func() {
		if cached, ok := w.track[key]; ok && !cached.NotAfter.IsZero() {
			notif := Notification{
				Provider: key.provider,
				SNI:      key.sni,
				Cert:     cached,
			}
			w.notifyAllLocked(notif)
		}
	})
	// Otherwise, let's briefly ask the cache, in case it was
	// added before we started listening.
	best, ok := providerView.BestMatchFor(sni, logger)
	if ok {
		w.withLock(func() {
			w.track[key] = best
			w.notifyAllLocked(Notification{
				Provider: key.provider,
				SNI:      key.sni,
				Cert:     best,
			})
		})
	}
	return err
}

// RemoveManaged stops tracking the SNI for future listings.
func (w *Watcher) RemoveManaged(logger *slog.Logger, providerName string, sni string) {
	providerView, err := w.manager.resolveProviderView(providerName)
	if err == nil {
		resolvedName := providerView.Name()
		w.withLock(func() {
			delete(w.track, trackKey{provider: resolvedName, sni: sni})
		})
		w.manager.RemoveManaged(logger, resolvedName, sni)
		return
	}
	w.manager.RemoveManaged(logger, providerName, sni)
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
	var toRemove []trackKey
	w.withLock(func() {
		toRemove = make([]trackKey, 0, len(w.track))
		for key := range w.track {
			toRemove = append(toRemove, key)
		}
		w.track = make(map[trackKey]ssl.Certificate)
		w.cleared = true
	})
	for _, key := range toRemove {
		w.manager.RemoveManaged(logger, key.provider, key.sni)
	}
}

// Keeps sending certificate updates to all listeners
func (w *Watcher) RunWatch(ctx context.Context, logger *slog.Logger, refresh time.Duration) {
	ticker := time.NewTicker(refresh)
	defer ticker.Stop()
	expiredTicker := time.NewTicker(24 * time.Hour)
	defer expiredTicker.Stop()
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
				tracked, ok := w.track[key]
				if !ok || tracked.NotAfter.IsZero() || tracked.NotAfter.Before(best.NotAfter) {
					w.track[key] = best
					w.notifyAllLocked(Notification{
						Provider: key.provider,
						SNI:      key.sni,
						Cert:     best,
					})
				}
			})
		case <-ticker.C:
			// Periodically, walk over al failed certificates, to see
			// if some of them has been fixed
			snapshot := make(map[trackKey]ssl.Certificate)
			w.withLock(func() {
				maps.Copy(snapshot, w.track)
				w.cleared = false
			})
			for key, cachedCert := range snapshot {
				providerView, err := w.manager.resolveProviderView(key.provider)
				if err != nil {
					logger.Error("failed to resolve provider", "error", err)
					continue
				}
				best, ok := providerView.BestMatchFor(key.sni, logger)
				if !ok {
					continue
				}
				if cachedCert.NotAfter.IsZero() || best.NotAfter.After(cachedCert.NotAfter) {
					snapshot[key] = best
				}
			}
			w.withLock(func() {
				if w.cleared {
					return
				}
				for key, best := range snapshot {
					if best.NotAfter.After(w.track[key].NotAfter) {
						logger.Info("certificate updated", "sni", key.sni, "provider", key.provider)
						w.track[key] = best
						w.notifyAllLocked(Notification{
							Provider: key.provider,
							SNI:      key.sni,
							Cert:     best,
						})
					}
				}
			})
		case <-expiredTicker.C:
			if err := w.manager.RemoveExpired(ctx, logger); err != nil {
				logger.Error("failed to remove expired certificates", "error", err)
			}
		}
	}
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
