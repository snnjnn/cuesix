package factory

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/warpcomdev/cuesix/cmd/cuesix/config"
	"github.com/warpcomdev/cuesix/internal/certmagicmgr"
	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
)

type SSLSetup struct {
	Enabled      bool
	FallbackCert ssl.PEMCertificate
	FileManager  ssl.FileManager
	FallbackMgr  ssl.FallbackManager
	Router       ssl.ProviderRouter
	AcmeManager  certmagicmgr.Manager
	AcmeTracker  *ssl.Tracker
	Events       chan ssl.Tracking
}

func NewSSLSetup(logger *slog.Logger, pluginCfg config.Plugins, certmagicCfg config.Certmagic, apisixCfg config.APISIX) (SSLSetup, error) {
	var setup SSLSetup
	if cert, ok, err := pluginCfg.LoadFallbackCertificate(apisixCfg.Home, pluginCfg.EnableSSL); ok {
		if err != nil {
			logger.Error("failed to load fallback certificate", "certPath", pluginCfg.FallbackCert, "keyPath", pluginCfg.FallbackKey, "error", err)
			return setup, err
		}
		setup.FallbackCert = cert
	}
	if pluginCfg.EnableSSL {
		fses, err := BuildFilesystems(pluginCfg.SSLPaths)
		if err != nil {
			return setup, err
		}
		setup.FileManager = ssl.FileManager{Filesystems: fses, Logger: logger}
	}
	setup.FallbackMgr = ssl.FallbackManager{Certificate: setup.FallbackCert}

	if certmagicCfg.Enabled {
		if certmagicCfg.ChallengePort <= 0 {
			return setup, errors.New("certmagic enabled but challenge port is invalid")
		}
		providers, err := buildCertmagicProviders(certmagicCfg.Providers)
		if err != nil {
			return setup, fmt.Errorf("certmagic provider config invalid: %w", err)
		}
		setup.Events = make(chan ssl.Tracking, 32)
		setup.AcmeManager, err = certmagicmgr.NewManager(logger, certmagicmgr.Config{
			Providers:         providers,
			DefaultProvider:   strings.TrimSpace(certmagicCfg.DefaultProvider),
			DataDir:           strings.TrimSpace(certmagicCfg.DataDir),
			CertObtainTimeout: certmagicCfg.Timeout,
			ChallengePort:     certmagicCfg.ChallengePort,
		}, setup.Events, setup.FallbackCert, nil, nil)
		if err != nil {
			close(setup.Events)
			return setup, fmt.Errorf("certmagic init failed: %w", err)
		}
		setup.Enabled = true
	}

	setup.Router = ssl.ProviderRouter{
		FileManager:     setup.FileManager,
		FallbackManager: setup.FallbackMgr,
	}
	if setup.Enabled {
		setup.Router.ACMEManager = adaptedManager{Manager: setup.AcmeManager}
	}
	var err error
	setup.AcmeTracker, err = ssl.NewTracker(logger, setup.Router)
	if err != nil {
		if setup.Events != nil {
			close(setup.Events)
		}
		return setup, fmt.Errorf("live watcher init failed: %w", err)
	}
	return setup, nil
}

type adaptedManager struct {
	certmagicmgr.Manager
}

func (a adaptedManager) ResolveProvider(name string) (ssl.Provider, error) {
	return a.Manager.ResolveProvider(name)
}

// buildCertmagicProviders parses certmagic provider specs.
func buildCertmagicProviders(specs []string) ([]certmagicmgr.ProviderConfig, error) {
	if len(specs) == 0 {
		return nil, errors.New("at least one certmagic provider is required")
	}
	providers := make([]certmagicmgr.ProviderConfig, 0, len(specs))
	for _, spec := range specs {
		cfg, err := certmagicmgr.ParseACMEProviderSpec(spec)
		if err != nil {
			return nil, err
		}
		providers = append(providers, cfg)
	}
	return providers, nil
}
