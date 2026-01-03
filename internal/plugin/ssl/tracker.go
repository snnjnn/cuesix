package ssl

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/warpcomdev/cuesix/internal/cursor"
)

type ACMEKey struct {
	Provider string
	SNI      string
}

type ACMECertificate struct {
	ACMEKey
	Certificate
}

// ACMEProvider exposes provider operations needed by Watcher.
type ACMEProvider interface {
	Name() string
	BestMatchFor(sni string) (Certificate, bool)
	RequestCertificate(ctx context.Context, sni string) error
	RemoveManaged(sni string)
}

// RequestCertificate obtains or loads a certificate for the given SNI.
type ACMEManager interface {
	// Gets the internal provider to remove certificates
	ResolveProvider(name string) (ACMEProvider, error)
}

type trackedCertificate struct {
	Certificate
	TrackedAt time.Time
}

// Tracker tracks certificate updates from certmagic.
type Tracker struct {
	manager ACMEManager
	logger  *slog.Logger
	track   map[ACMEKey]trackedCertificate
	cursor.Watcher[ACMECertificate]
}

// NewWatcher builds a watcher for certificate updates.
func NewTracker(logger *slog.Logger, manager ACMEManager) (*Tracker, error) {
	if logger == nil {
		logger = slog.Default()
	}
	tracker := &Tracker{
		manager: manager,
		track:   make(map[ACMEKey]trackedCertificate),
		logger:  logger,
	}
	tracker.Embedded(func(n ACMECertificate) string {
		return n.SNI
	})
	return tracker, nil
}

func (w *Tracker) WithLock(closure func()) {
	w.Watcher.WithLock(closure)
}

// RequestCertificate obtains or loads a certificate for the given SNI.
func (w *Tracker) RequestCertificate(ctx context.Context, providerName string, sni string) (ACMEKey, error) {
	providerCache := make(map[string]ACMEProvider)
	providerView, err := w.providerFor(providerName, providerCache)
	if err != nil {
		return ACMEKey{}, err
	}
	key := ACMEKey{Provider: providerView.Name(), SNI: sni}
	// Lets first check if the certificate is tracked
	var (
		tracked trackedCertificate
		found   bool
	)
	w.WithLock(func() {
		if tracked, found = w.track[key]; found {
			// If the certificate is ready, broadcast it
			if !tracked.NotAfter.IsZero() {
				notif := ACMECertificate{
					ACMEKey:     key,
					Certificate: tracked.Certificate,
				}
				w.NotifyAllLocked(ctx, notif)
			}
			return
		}
		// If not tracked, lock it before proceeding
		tracked = trackedCertificate{
			Certificate: Certificate{},
			TrackedAt:   time.Now(),
		}
		w.track[key] = tracked
	})
	// If the certificate was not found, we owned it by adding an empty one.
	// Ask the manager to create it.
	if !found {
		err = providerView.RequestCertificate(ctx, key.SNI)
		if err != nil {
			w.WithLock(func() {
				delete(w.track, key)
			})
			return ACMEKey{}, err
		}
	}
	// Owned ot not, if it is not already populated, do a brief
	// peek at the cache to see if an update has arrived
	if tracked.NotAfter.IsZero() {
		w.poll(ctx, providerCache, key)
	}
	return key, err
}

// Commit a list of certificates and collect their expiration dates.
// Return the number of dates modified in the map.
func (w *Tracker) Commit(ctx context.Context, logger *slog.Logger, committed map[ACMEKey]time.Time, gracePeriod time.Duration) int {
	if w == nil {
		return 0
	}
	// First: Poll certificates, to make sure we have up to date expirations
	providerCache := make(map[string]ACMEProvider)
	w.poll(ctx, providerCache, slices.Collect(maps.Keys(committed))...)
	// Then, sort trackend entries into committed and remains
	updates := 0
	remains := make([]ACMEKey, 0, 16)
	w.WithLock(func() {
		commitDate := time.Now()
		deadline := commitDate.Add(-gracePeriod)
		for key, cert := range w.track {
			// If key is committed, record the date
			if notAfter, ok := committed[key]; ok {
				if !notAfter.Equal(cert.NotAfter) {
					committed[key] = cert.NotAfter
					updates += 1
				}
				cert.TrackedAt = commitDate
				w.track[key] = cert
			} else {
				// If the cert has not been committed for long,
				// unload it
				if cert.TrackedAt.Before(deadline) {
					delete(w.track, key)
					remains = append(remains, key)
				}
			}
		}
	})
	// Unmanage all remains
	w.unmanage(ctx, providerCache, remains...)
	return updates
}

// UpdateLoop over certificate updates and forward to watchers
func UpdateLoop(ctx context.Context, logger *slog.Logger, w *Tracker, events cursor.Cursor[ACMEKey]) {
	providerCache := make(map[string]ACMEProvider)
	for certInfo := range cursor.All(ctx, events) {
		if certInfo.Provider == "" {
			logger.Error("missing provider on cert event", "sni", certInfo)
			continue
		}
		provider, err := w.providerFor(certInfo.Provider, providerCache)
		if err != nil {
			logger.Error("failed to resolve provider", "provider", certInfo.Provider, "sni", certInfo)
			continue
		}
		best, ok := provider.BestMatchFor(certInfo.SNI)
		if !ok {
			continue
		}
		w.WithLock(func() {
			if tracked, ok := w.track[certInfo]; ok {
				if tracked.NotAfter.IsZero() || tracked.NotAfter.Before(best.NotAfter) {
					tracked.Certificate = best
					w.track[certInfo] = tracked
				}
				return
			}
			// Always notify of real renovations, just in case.
			w.NotifyAllLocked(ctx, ACMECertificate{
				ACMEKey:     certInfo,
				Certificate: best,
			})
		})
	}
}

// Poll cache for updates to the given certs
func (w *Tracker) poll(ctx context.Context, providerCache map[string]ACMEProvider, keys ...ACMEKey) {
	candidates := make(map[ACMEKey]Certificate)
	for _, key := range keys {
		providerView, err := w.providerFor(key.Provider, providerCache)
		if err != nil {
			continue
		}
		best, ok := providerView.BestMatchFor(key.SNI)
		if ok {
			candidates[key] = best
		}
	}
	w.WithLock(func() {
		for key, best := range candidates {
			w.track[key] = trackedCertificate{
				Certificate: best,
				TrackedAt:   time.Now(),
			}
			w.NotifyAllLocked(ctx, ACMECertificate{
				ACMEKey:     key,
				Certificate: best,
			})
		}
	})
}

// Unmanage certificates that have not been committed for long
func (w *Tracker) unmanage(ctx context.Context, providerCache map[string]ACMEProvider, remains ...ACMEKey) {
	if w == nil {
		return
	}
	logger := w.logger
	for _, key := range remains {
		providerView, err := w.providerFor(key.Provider, providerCache)
		if err != nil {
			continue
		}
		providerView.RemoveManaged(key.SNI)
	}
	// Rollback: if someone else added the certs back while we where unmanaging them,
	// we have to make sure we didn't race them to unmanage
	rollbacks := make(map[ACMEKey]struct{})
	w.WithLock(func() {
		for _, key := range remains {
			if _, ok := w.track[key]; ok {
				rollbacks[key] = struct{}{}
			}
		}
	})
	for rollback := range rollbacks {
		providerView, err := w.providerFor(rollback.Provider, providerCache)
		if err != nil {
			continue
		}
		if err := providerView.RequestCertificate(ctx, rollback.SNI); err != nil {
			logger.Error("failed to rollback certificate", "sni", rollback.SNI, "provider", rollback.Provider, "error", err)
		}
	}
}

// providerFor return the provider for a given name
func (w *Tracker) providerFor(name string, cache map[string]ACMEProvider) (ACMEProvider, error) {
	if provider, ok := cache[name]; ok {
		return provider, nil
	}
	provider, err := w.manager.ResolveProvider(name)
	if err != nil {
		return nil, err
	}
	cache[name] = provider
	return provider, nil
}
