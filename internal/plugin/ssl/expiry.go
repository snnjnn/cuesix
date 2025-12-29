package ssl

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Notifier triggers a compile reload via the dispatcher.
type Notifier interface {
	Notify()
}

// ExpiryManager tracks certificate expirations and triggers reloads when needed.
type ExpiryManager struct {
	warnWindow    time.Duration
	checkInterval time.Duration

	mu            sync.Mutex
	expirations   map[string]time.Time
	lastNotifyDay string
	lastConfigDay string
	notifier      Notifier
}

// NewExpiryManager builds a manager for tracking certificate expirations.
func NewExpiryManager(warnWindow time.Duration, checkInterval time.Duration, notifier Notifier) *ExpiryManager {
	return &ExpiryManager{
		warnWindow:    warnWindow,
		checkInterval: checkInterval,
		expirations:   make(map[string]time.Time),
		notifier:      notifier,
	}
}

// SetNotifier updates the notifier used by the manager.
func (m *ExpiryManager) SetNotifier(notifier Notifier) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.notifier = notifier
	m.mu.Unlock()
}

// ResetForConfig clears tracked expirations and suppresses notifications for today.
func (m *ExpiryManager) ResetForConfig(now time.Time) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.expirations = make(map[string]time.Time)
	m.lastConfigDay = dayKey(now)
	m.mu.Unlock()
}

// RecordSNI registers a certificate expiration for a given SNI.
func (m *ExpiryManager) RecordSNI(sni string, expires time.Time) {
	if m == nil || sni == "" || expires.IsZero() {
		return
	}
	m.mu.Lock()
	if prev, ok := m.expirations[sni]; !ok || expires.After(prev) {
		m.expirations[sni] = expires
	}
	m.mu.Unlock()
}

// Run periodically checks for expiring certificates until the context is canceled.
func (m *ExpiryManager) Run(ctx context.Context, logger *slog.Logger) error {
	if m == nil || m.checkInterval <= 0 {
		return nil
	}
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			m.checkAndNotify(time.Now(), logger)
		}
	}
}

func (m *ExpiryManager) checkAndNotify(now time.Time, logger *slog.Logger) {
	if m == nil || m.warnWindow <= 0 {
		return
	}
	day := dayKey(now)
	m.mu.Lock()
	if m.notifier == nil || day == m.lastConfigDay || day == m.lastNotifyDay {
		m.mu.Unlock()
		return
	}
	count, earliest := m.expiringSoonLocked(now)
	if count == 0 {
		m.mu.Unlock()
		return
	}
	m.lastNotifyDay = day
	notifier := m.notifier
	m.mu.Unlock()

	logger.Info("ssl expiry notification triggered", "count", count, "earliest", earliest.UTC())
	notifier.Notify()
}

func (m *ExpiryManager) expiringSoonLocked(now time.Time) (int, time.Time) {
	var count int
	var earliest time.Time
	for _, expires := range m.expirations {
		if !expires.After(now) {
			continue
		}
		if expires.Sub(now) > m.warnWindow {
			continue
		}
		count++
		if earliest.IsZero() || expires.Before(earliest) {
			earliest = expires
		}
	}
	return count, earliest
}

func dayKey(now time.Time) string {
	return now.UTC().Format("2006-01-02")
}
