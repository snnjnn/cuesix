package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/warpcomdev/cuesix/cmd/cuesix/config"
	"github.com/warpcomdev/cuesix/internal/certmagicmgr"
)

type acmeSetup struct {
	acmeManager *certmagicmgr.Manager
	acmeWatcher *certmagicmgr.Watcher
	events      chan certmagicmgr.CertEvent
}

func newAcmeSetup(logger *slog.Logger, certmagicCfg config.Certmagic) (acmeSetup, error) {
	var setup acmeSetup
	if !certmagicCfg.Enabled {
		return setup, nil
	}
	if certmagicCfg.ChallengeAddr == "" {
		return setup, errors.New("certmagic enabled but challenge address is missing")
	}
	providers, err := buildCertmagicProviders(certmagicCfg.Providers)
	if err != nil {
		return setup, fmt.Errorf("certmagic provider config invalid: %w", err)
	}
	setup.events = make(chan certmagicmgr.CertEvent, 32)
	setup.acmeManager, err = certmagicmgr.NewManager(certmagicmgr.Config{
		Providers:       providers,
		DefaultProvider: strings.TrimSpace(certmagicCfg.DefaultProvider),
		DataDir:         strings.TrimSpace(certmagicCfg.DataDir),
		DefaultTimeout:  certmagicCfg.Timeout,
	}, logger, setup.events)
	if err != nil {
		close(setup.events)
		return setup, fmt.Errorf("certmagic init failed: %w", err)
	}
	setup.acmeWatcher, err = certmagicmgr.NewWatcher(setup.acmeManager, setup.events)
	if err != nil {
		close(setup.events)
		return setup, fmt.Errorf("certmagic watcher init failed: %w", err)
	}
	return setup, nil
}

func (a acmeSetup) shouldCleanup(cfg config.Certmagic) bool {
	return cfg.Enabled && cfg.CleanupInterval > 0 && cfg.ExpiredGrace > 0 && cfg.UntrackedGrace > 0
}

func (a acmeSetup) cleanupLoop(groupCtx context.Context, logger *slog.Logger, certmagicCfg config.Certmagic) error {
	// Launch the acme tracking cleanup
	ticker := time.NewTicker(certmagicCfg.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-groupCtx.Done():
			return nil
		case <-ticker.C:
			if err := a.acmeWatcher.RemoveUntracked(groupCtx, logger, certmagicCfg.UntrackedGrace); err != nil {
				logger.Error("remove untracked certmagic entries failed", "error", err)
			}
			if err := a.acmeManager.RemoveExpired(groupCtx, logger, certmagicCfg.CleanupInterval, certmagicCfg.ExpiredGrace); err != nil {
				logger.Error("remove expired certificates failed", "error", err)
			}
		}
	}
}
