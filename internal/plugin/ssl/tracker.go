package ssl

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/warpcomdev/cuesix/internal/cursor"
)

type Tracking struct {
	Provider string
	Identity string
}

type Delivery struct {
	Tracking
	Certificate
}

// Provider exposes provider operations needed by Watcher users
type ProviderView interface {
	Name() string
	BestMatchFor(identity string) (Certificate, bool)
	RequestCertificate(ctx context.Context, identity string) error
}

// Provider exposes provider operations needed by Watcher itself
type Provider interface {
	ProviderView
	RemoveManaged(identity string)
}

// RequestCertificate obtains or loads a certificate for the given Id.
type Manager interface {
	// Gets the internal provider to remove certificates
	ResolveProvider(name string) (Provider, error)
}

type trackedCertificate struct {
	Certificate
	TrackedAt time.Time
}

// Tracker tracks certificate updates from certmagic.
type Tracker struct {
	manager Manager
	logger  *slog.Logger
	track   map[Tracking]trackedCertificate
	cursor.Watcher[Delivery]
}

// NewWatcher builds a watcher for certificate updates.
func NewTracker(logger *slog.Logger, manager Manager) (*Tracker, error) {
	if logger == nil {
		logger = slog.Default()
	}
	tracker := &Tracker{
		manager: manager,
		track:   make(map[Tracking]trackedCertificate),
		logger:  logger,
	}
	tracker.Embedded(func(n Delivery) string {
		return n.Identity
	})
	return tracker, nil
}

func (w *Tracker) WithLock(closure func()) {
	w.Watcher.WithLock(closure)
}

// RequestCertificate obtains or loads a certificate for the given Id.
func (w *Tracker) RequestCertificate(ctx context.Context, providerName string, identity string) (Tracking, error) {
	providerCache := make(map[string]Provider)
	providerView, err := w.providerFor(providerName, providerCache)
	if err != nil {
		return Tracking{}, err
	}
	key := Tracking{Provider: providerView.Name(), Identity: identity}
	// Lets first check if the certificate is tracked
	var (
		tracked trackedCertificate
		found   bool
	)
	w.WithLock(func() {
		if tracked, found = w.track[key]; found {
			// If the certificate is ready, broadcast it
			if !tracked.NotAfter.IsZero() {
				notif := Delivery{
					Tracking:    key,
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
		err = providerView.RequestCertificate(ctx, key.Identity)
		if err != nil {
			w.WithLock(func() {
				delete(w.track, key)
			})
			return Tracking{}, err
		}
	}
	// Owned ot not, if it is not already populated, do a brief
	// peek at the cache to see if an update has arrived
	if tracked.NotAfter.IsZero() {
		w.poll(ctx, providerView, key)
	}
	return key, err
}

// Commit a list of certificates and collect their expiration dates.
// Return the number of dates modified in the map.
func (w *Tracker) Commit(ctx context.Context, logger *slog.Logger, committed map[Tracking]time.Time, gracePeriod time.Duration) int {
	if w == nil {
		return 0
	}
	// First: Poll certificates, to make sure we have up to date expirations
	providerCache := make(map[string]Provider)
	providerCerts := w.sort(providerCache, slices.Collect(maps.Keys(committed))...)
	for providerName, keys := range providerCerts {
		providerView, err := w.providerFor(providerName, providerCache)
		if err != nil {
			logger.Error("failed to resolve provider", "provider", providerName)
			continue
		}
		w.poll(ctx, providerView, keys...)
	}
	// Then, sort tracked entries into committed and remains
	updates := 0
	remains := make([]Tracking, 0, 16)
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
func UpdateLoop(ctx context.Context, logger *slog.Logger, w *Tracker, events cursor.Cursor[Tracking]) {
	providerCache := make(map[string]Provider)
	for certInfo := range cursor.All(ctx, events) {
		if certInfo.Provider == "" {
			logger.Error("missing provider on cert event", "identity", certInfo)
			continue
		}
		provider, err := w.providerFor(certInfo.Provider, providerCache)
		if err != nil {
			logger.Error("failed to resolve provider", "provider", certInfo.Provider, "identity", certInfo)
			continue
		}
		best, ok := provider.BestMatchFor(certInfo.Identity)
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
			w.NotifyAllLocked(ctx, Delivery{
				Tracking:    certInfo,
				Certificate: best,
			})
		})
	}
}

// Sort keys by provider name
func (w *Tracker) sort(providerCache map[string]Provider, keys ...Tracking) map[string][]Tracking {
	byProvider := make(map[string][]Tracking)
	for _, key := range keys {
		providerView, err := w.providerFor(key.Provider, providerCache)
		if err != nil {
			continue
		}
		name := providerView.Name()
		byProvider[name] = append(byProvider[name], key)
	}
	return byProvider
}

// Poll cache for updates to the given certs
func (w *Tracker) poll(ctx context.Context, provider ProviderView, committedKeys ...Tracking) {
	candidates := make(map[Tracking]Certificate)
	for _, key := range committedKeys {
		best, ok := provider.BestMatchFor(key.Identity)
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
			w.NotifyAllLocked(ctx, Delivery{
				Tracking:    key,
				Certificate: best,
			})
		}
	})
}

// Unmanage certificates that have not been committed for long
func (w *Tracker) unmanage(ctx context.Context, providerCache map[string]Provider, remains ...Tracking) {
	if w == nil {
		return
	}
	logger := w.logger
	for _, key := range remains {
		providerView, err := w.providerFor(key.Provider, providerCache)
		if err != nil {
			continue
		}
		providerView.RemoveManaged(key.Identity)
	}
	// Rollback: if someone else added the certs back while we where unmanaging them,
	// we have to make sure we didn't race them to unmanage
	rollbacks := make(map[Tracking]struct{})
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
		if err := providerView.RequestCertificate(ctx, rollback.Identity); err != nil {
			logger.Error("failed to rollback certificate", "identity", rollback.Identity, "provider", rollback.Provider, "error", err)
		}
	}
}

// providerFor return the provider for a given name
func (w *Tracker) providerFor(name string, cache map[string]Provider) (Provider, error) {
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
