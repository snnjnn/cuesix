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

// Provider exposes provider operations needed by Watcher itself
type Provider interface {
	Name() string
	BestMatchFor(ctx context.Context, identity string) (Certificate, bool)
	RequestCertificate(ctx context.Context, identity string) error
	RemoveManaged(ctx context.Context, identity ...string)
}

// RequestCertificate obtains or loads a certificate for the given Id.
type Manager interface {
	// Gets the internal provider to remove certificates
	ResolveProvider(name string) (Provider, error)
}

type ProviderCache struct {
	providers map[string]Provider
}

type trackedCertificate struct {
	Certificate
	TrackedAt time.Time
}

type TrackedProvider struct {
	provider Provider
	tracker  *Tracker
}

func (tp TrackedProvider) Name() string {
	return tp.provider.Name()
}

// Poll cache for updates to the given certs
func (tp TrackedProvider) BestMatchFor(ctx context.Context, identity string) (Certificate, bool) {
	return tp.provider.BestMatchFor(ctx, identity)
}

func (tp TrackedProvider) RequestCertificate(ctx context.Context, identity string) error {
	// Lets first check if the certificate is tracked
	var (
		tracked trackedCertificate
		found   bool
	)
	key := Tracking{Provider: tp.provider.Name(), Identity: identity}
	tp.tracker.WithLock(func() {
		if tracked, found = tp.tracker.track[key]; found {
			// If the certificate is ready, broadcast it
			if !tracked.NotAfter.IsZero() {
				notif := Delivery{
					Tracking:    key,
					Certificate: tracked.Certificate,
				}
				tp.tracker.NotifyAllLocked(ctx, notif)
			}
			return
		}
		// If not tracked, lock it before proceeding
		tracked = trackedCertificate{
			Certificate: Certificate{},
			TrackedAt:   time.Now(),
		}
		tp.tracker.track[key] = tracked
	})
	// If the certificate was not found, we owned it by adding an empty one.
	// Ask the provider to create it.
	if !found {
		err := tp.provider.RequestCertificate(ctx, key.Identity)
		if err != nil {
			tp.tracker.WithLock(func() {
				delete(tp.tracker.track, key)
			})
			return err
		}
	}
	// Owned ot not, if it is not already populated, do a brief
	// peek at the cache to see if an update has arrived
	if tracked.NotAfter.IsZero() {
		if best, ok := tp.BestMatchFor(ctx, key.Identity); ok {
			tp.tracker.updateTrack(ctx, key, best, true, false)
		}
	}
	return nil
}

// Unmanage certificates that have not been committed for long
func (tp TrackedProvider) RemoveManaged(ctx context.Context, remains ...string) {
	name := tp.provider.Name()
	tp.tracker.WithLock(func() {
		for _, identity := range remains {
			key := Tracking{Provider: name, Identity: identity}
			delete(tp.tracker.track, key)
		}
	})
	tp.provider.RemoveManaged(ctx, remains...)
	// Rollback: if someone else added the certs back while we where unmanaging them,
	// we have to make sure we didn't race them to unmanage
	rollbacks := make(map[Tracking]struct{})
	tp.tracker.WithLock(func() {
		for _, identity := range remains {
			key := Tracking{Provider: name, Identity: identity}
			if _, ok := tp.tracker.track[key]; ok {
				rollbacks[key] = struct{}{}
			}
		}
	})
	for rollback := range rollbacks {
		logger := tp.tracker.logger
		if err := tp.provider.RequestCertificate(ctx, rollback.Identity); err != nil {
			logger.Error("failed to rollback certificate", "identity", rollback.Identity, "provider", rollback.Provider, "error", err)
		}
	}
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

// providerFor return the provider for a given name
func (w *Tracker) ResolveProvider(name string, cache *ProviderCache) (Provider, error) {
	cacheMap := cache.providers
	if cacheMap == nil {
		cacheMap = make(map[string]Provider)
		cache.providers = cacheMap
	}
	if provider, ok := cacheMap[name]; ok {
		return TrackedProvider{
			provider: provider,
			tracker:  w,
		}, nil
	}
	provider, err := w.manager.ResolveProvider(name)
	if err != nil {
		return TrackedProvider{}, err
	}
	// Save both the given and the normalized name
	cacheMap[provider.Name()] = provider
	cacheMap[name] = provider
	return TrackedProvider{
		provider: provider,
		tracker:  w,
	}, nil
}

// Commit a list of certificates and collect their expiration dates.
// Return the number of dates modified in the map.
func (w *Tracker) Commit(ctx context.Context, logger *slog.Logger, committed map[Tracking]time.Time, gracePeriod time.Duration) int {
	if w == nil {
		return 0
	}
	// First: Poll certificates, to make sure we have up to date expirations
	var providerCache ProviderCache
	providerCerts := w.sort(&providerCache, slices.Collect(maps.Keys(committed))...)
	updates := 0
	for providerName, keys := range providerCerts {
		// Poll providers for updates in certificates
		provider, err := w.ResolveProvider(providerName, &providerCache)
		if err != nil {
			logger.Error("failed to resolve provider", "provider", providerName)
			continue
		}
		polledCerts := make(map[Tracking]Certificate)
		for _, key := range keys {
			if best, ok := provider.BestMatchFor(ctx, key.Identity); ok {
				polledCerts[key] = best
			}
		}
		// Then, sort tracked entries into committed and remains
		remains := make([]string, 0, 16)
		w.WithLock(func() {
			commitDate := time.Now()
			deadline := commitDate.Add(-gracePeriod)
			for key, cert := range w.track {
				// If key is committed, record the date
				if notAfter, ok := committed[key]; ok {
					// Also, take the chance to notify on updates
					notify := false
					if update, ok := polledCerts[key]; ok {
						if update.NotAfter.After(cert.NotAfter) {
							cert.Certificate = update
							notify = true
						}
					}
					if !notAfter.Equal(cert.NotAfter) {
						committed[key] = cert.NotAfter
						updates += 1
					}
					cert.TrackedAt = commitDate
					w.track[key] = cert
					if notify {
						w.NotifyAllLocked(ctx, Delivery{
							Tracking:    key,
							Certificate: cert.Certificate,
						})
					}
				} else {
					// If the cert has not been committed for long, unload it
					if cert.TrackedAt.Before(deadline) {
						delete(w.track, key)
						remains = append(remains, key.Identity)
					}
				}
			}
		})
		// Unmanage all remains
		provider.RemoveManaged(ctx, remains...)
	}
	return updates
}

// UpdateLoop over certificate updates and forward to watchers
func UpdateLoop(ctx context.Context, logger *slog.Logger, w *Tracker, events cursor.Cursor[Tracking]) {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Info("ssl tracker update loop started")
	var providerCache ProviderCache
	for certInfo := range cursor.All(ctx, events) {
		logger.Info("ssl tracker event received", "provider", certInfo.Provider, "identity", certInfo.Identity)
		if certInfo.Provider == "" {
			logger.Error("missing provider on cert event", "identity", certInfo)
			continue
		}
		provider, err := w.ResolveProvider(certInfo.Provider, &providerCache)
		if err != nil {
			logger.Error("failed to resolve provider", "provider", certInfo.Provider, "identity", certInfo)
			continue
		}
		// Update the certificate, and always notify
		if best, ok := provider.BestMatchFor(ctx, certInfo.Identity); ok {
			w.updateTrack(ctx, certInfo, best, true, true)
		} else {
			logger.Info("ssl tracker event missing certificate", "provider", certInfo.Provider, "identity", certInfo.Identity)
		}
	}
}

// Sort keys by provider name
func (w *Tracker) sort(providerCache *ProviderCache, keys ...Tracking) map[string][]Tracking {
	byProvider := make(map[string][]Tracking)
	for _, key := range keys {
		provider, err := w.ResolveProvider(key.Provider, providerCache)
		if err != nil {
			continue
		}
		name := provider.Name()
		byProvider[name] = append(byProvider[name], key)
	}
	return byProvider
}

func (w *Tracker) updateTrack(ctx context.Context, key Tracking, best Certificate, notify, notifyAlways bool) {
	w.WithLock(func() {
		if tracked, ok := w.track[key]; ok {
			if tracked.NotAfter.IsZero() || best.NotAfter.After(tracked.NotAfter) {
				tracked.Certificate = best
				w.track[key] = tracked
				if notify {
					notifyAlways = true
				}
			}
			if notifyAlways {
				w.NotifyAllLocked(ctx, Delivery{
					Tracking:    key,
					Certificate: best,
				})
			}
		}
	})
}
