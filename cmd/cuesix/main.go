package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/warpcomdev/cuesix/cmd/cuesix/config"
	"github.com/warpcomdev/cuesix/internal/cache"
	"github.com/warpcomdev/cuesix/internal/certmagicmgr"
	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/dispatcher"
	"github.com/warpcomdev/cuesix/internal/listener"
	"github.com/warpcomdev/cuesix/internal/plugin"
	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
	"github.com/warpcomdev/cuesix/internal/reloader"
	"github.com/warpcomdev/cuesix/internal/validator"
	"golang.org/x/sync/errgroup"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	inputCfg := config.Input{}
	serverCfg := config.Server{}
	apisixCfg := config.APISIX{}
	reloadCfg := config.Reload{}
	pluginCfg := config.Plugins{}
	certmagicCfg := config.Certmagic{}

	inputCfg.RegisterFlags(flag.CommandLine)
	serverCfg.RegisterFlags(flag.CommandLine)
	apisixCfg.RegisterFlags(flag.CommandLine)
	reloadCfg.RegisterFlags(flag.CommandLine)
	pluginCfg.RegisterFlags(flag.CommandLine)
	certmagicCfg.RegisterFlags(flag.CommandLine)

	flag.Parse()

	if err := inputCfg.Validate(); err != nil {
		log.Fatal(err)
	}
	if err := certmagicCfg.Validate(); err != nil {
		log.Fatal(err)
	}

	srvTimeouts := serverTimeouts{
		ReadHeaderTimeout: serverCfg.ReadHeaderTimeout,
		ReadTimeout:       serverCfg.ReadTimeout,
		WriteTimeout:      serverCfg.WriteTimeout,
		IdleTimeout:       serverCfg.IdleTimeout,
	}

	// Build input filesystem views.
	fses, err := buildFilesystems(inputCfg.InputDirs)
	if err != nil {
		log.Fatalf("input dirs: %v", err)
	}

	// Load fallback certificate for SSL plugin.
	var fallbackCert ssl.Certificate
	if len(pluginCfg.SSLPaths) > 0 || certmagicCfg.Enabled {
		if pluginCfg.FallbackCert == "" {
			pluginCfg.FallbackCert = filepath.Join(apisixCfg.Home, "conf", "cert", "ssl_PLACE_HOLDER.crt")
		}
		if pluginCfg.FallbackKey == "" {
			pluginCfg.FallbackKey = filepath.Join(apisixCfg.Home, "conf", "cert", "ssl_PLACE_HOLDER.key")
		}
		fallbackCert, err = ssl.LoadFallbackCertificate(pluginCfg.FallbackCert, pluginCfg.FallbackKey)
		if err != nil {
			logger.Error("failed to load fallback certificate", "certPath", pluginCfg.FallbackCert, "keyPath", pluginCfg.FallbackKey, "error", err)
			os.Exit(1)
		}
	}

	// Configure certmagic manager + watcher.
	var (
		acmeManager *certmagicmgr.Manager
		acmeWatcher *certmagicmgr.Watcher
		events      chan certmagicmgr.CertEvent
	)
	if certmagicCfg.Enabled {
		if certmagicCfg.ChallengeAddr == "" {
			logger.Error("certmagic enabled but challenge address is missing")
			os.Exit(1)
		}
		providers, err := buildCertmagicProviders(certmagicCfg.Providers)
		if err != nil {
			logger.Error("certmagic provider config invalid", "error", err)
			os.Exit(1)
		}
		events = make(chan certmagicmgr.CertEvent, 32)
		acmeManager, err = certmagicmgr.NewManager(certmagicmgr.Config{
			Providers:       providers,
			DefaultProvider: strings.TrimSpace(certmagicCfg.DefaultProvider),
			DataDir:         strings.TrimSpace(certmagicCfg.DataDir),
			DefaultTimeout:  certmagicCfg.Timeout,
		}, logger, events)
		if err != nil {
			logger.Error("certmagic init failed", "error", err)
			os.Exit(1)
		}
		acmeWatcher, err = certmagicmgr.NewWatcher(acmeManager, events)
		if err != nil {
			logger.Error("certmagic watcher init failed", "error", err)
			os.Exit(1)
		}
	}

	// Build plugin pipelines.
	pluginCacheInst := &pluginCache{
		preRender: func() plugin.PreRender {
			plugins, pluginErr := buildPreRender(pluginCfg.SSLPaths, acmeWatcher, fallbackCert, pluginCfg.SSLACMETimeout)
			if pluginErr != nil {
				logger.Error("pre-render plugin init failed", "error", pluginErr)
				os.Exit(1)
			}
			return plugins
		}(),
		cache: &cache.Cache{},
		postRender: func() plugin.PostRender {
			plugins, pluginErr := buildPostRender(pluginCfg.EnableJQ, pluginCfg.EnableYAML, pluginCfg.JQTimeout)
			if pluginErr != nil {
				logger.Error("post-render plugin init failed", "error", pluginErr)
				os.Exit(1)
			}
			return plugins
		}(),
	}

	// Standalone compile mode.
	if !inputCfg.Serve {
		merged, err := compiler.Compile(logger, fses...)
		if err != nil {
			logger.Error("compile failed", "error", err)
			os.Exit(1)
		}
		output, err := pluginCacheInst.Changed(logger, merged)
		if err != nil {
			logger.Error("plugin pipeline failed", "error", err)
			os.Exit(1)
		}
		if output == nil {
			logger.Error("unexpected nil output from plugin pipeline")
			os.Exit(1)
		}
		if _, err := os.Stdout.Write(output); err != nil {
			logger.Error("write output failed", "error", err)
			os.Exit(1)
		}
		return
	}

	// APISIX validation and mirror setup.
	if strings.TrimSpace(apisixCfg.Home) == "" {
		logger.Error("missing apisix home path")
		os.Exit(1)
	}
	mirrorKeep := apisixCfg.KeepMirror
	mirrorDir := apisixCfg.MirrorDir
	if mirrorDir == "" {
		tmp, tmpErr := os.MkdirTemp("", "cuesix-apisix-")
		if tmpErr != nil {
			logger.Error("create apisix mirror dir failed", "error", tmpErr)
			os.Exit(1)
		}
		mirrorKeep = true // no need to recreate it
		mirrorDir = tmp
		defer func() {
			if err := os.RemoveAll(mirrorDir); err != nil {
				logger.Error("remove apisix mirror failed", "error", err)
			}
		}()
	}
	val, err := validator.New(apisixCfg.Home, mirrorDir, mirrorKeep, apisixCfg.ValidationTimeout)
	if err != nil {
		logger.Error("prepare apisix mirror failed", "error", err)
		os.Exit(1)
	}
	configPath := validator.BuildConfigPath(apisixCfg.Home, pluginCfg.EnableYAML)

	// Reload target configuration.
	reloadURL := ""
	if reloadCfg.URL != "" {
		reloadURL, err = buildReloadURL(reloadCfg.URL)
		if err != nil {
			logger.Error("invalid apisix url", "error", err)
			os.Exit(1)
		}
	}

	// Wire reloader (or dry-run).
	var reloadTarget dispatcher.Reloader
	if reloadCfg.DryRun {
		reloadTarget = &dryRunReloader{}
	} else {
		reloadTarget = &reloader.Reloader{
			ConfigPath:      configPath,
			ReloadURL:       reloadURL,
			ReloadMethod:    reloadCfg.Method,
			APIKey:          reloadCfg.APIKey,
			RetryMax:        reloadCfg.RetryMax,
			RetryInitial:    reloadCfg.RetryInitial,
			RetryMaxDelay:   reloadCfg.RetryMaxDelay,
			RetryMultiplier: reloadCfg.RetryMultiplier,
			RequestTimeout:  reloadCfg.Timeout,
		}
	}

	// Dispatcher wiring.
	refreshManager := newRefreshManager(reloadTarget)
	disp, err := dispatcher.New(dispatcher.Config{
		Compiler:    compilerAdapter{},
		Cache:       pluginCacheInst,
		Validator:   val,
		Reloader:    refreshManager,
		Filesystems: fses,
		OutputYAML:  pluginCfg.EnableYAML,
		Cooldown:    inputCfg.Cooldown,
	})
	if err != nil {
		logger.Error("dispatcher init failed", "error", err)
		os.Exit(1)
	}
	refreshManager.realDispatcher = disp

	// HTTP handlers.
	handler, err := listener.NewHandler(refreshManager)
	if err != nil {
		logger.Error("listener init failed", "error", err)
		os.Exit(1)
	}
	server := buildServer(serverCfg.ListenAddr, handler, srvTimeouts)

	// Metrics server.
	var metricsServer *http.Server
	if strings.TrimSpace(serverCfg.MetricsAddr) != "" {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		metricsServer = buildServer(serverCfg.MetricsAddr, metricsMux, srvTimeouts)
	}

	// ACME challenge server.
	var acmeServer *http.Server
	if strings.TrimSpace(certmagicCfg.ChallengeAddr) != "" {
		acmeServer = buildServer(certmagicCfg.ChallengeAddr, acmeManager.ChallengeHandler(logger), srvTimeouts)
	}

	// Run loop and shutdown wiring.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	group, groupCtx := errgroup.WithContext(ctx)

	// Ready metrics and acme server first, because as soon as
	// I start the other services, I could get an acme request
	if metricsServer != nil {
		group.Go(func() error {
			logger.Info("starting metrics server", "addr", serverCfg.MetricsAddr)
			if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		})
		group.Go(serverShutdown(groupCtx, logger, "metrics server", metricsServer, serverCfg.ShutdownTimeout))
	}

	if acmeServer != nil {
		// Start the cert watcher
		group.Go(func() error {
			acmeWatcher.RunWatch(groupCtx, logger, certmagicCfg.WatchInterval)
			return nil
		})
		// And the acme server
		group.Go(func() error {
			logger.Info("starting acme server", "addr", certmagicCfg.ChallengeAddr)
			if err := acmeServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		})
		group.Go(serverShutdown(groupCtx, logger, "acme server", acmeServer, serverCfg.ShutdownTimeout))
	}

	// Ready the dispatcher
	group.Go(func() error {
		for {
			if err := disp.Run(groupCtx, logger); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				logger.Error("dispatcher error", "error", err)
				continue
			}
		}
	})

	// Ready the notifiers
	if acmeWatcher != nil {
		// Start the monitor that will reconfigure apisix when
		// certificates are renewed
		group.Go(func() error {
			var err error
			subs := acmeWatcher.Subscribe(32)
			defer acmeWatcher.Unsubscribe(subs)
			refreshManager.Watch(groupCtx, logger, func(ctx context.Context) bool {
				select {
				case <-ctx.Done():
					return true
				case _, ok := <-subs:
					if !ok {
						err = fmt.Errorf("acme watcher channel closed")
						return true
					}
					return false
				}
			})
			return err
		})
	}

	// Launch the acme tracking cleanup
	if acmeWatcher != nil && certmagicCfg.CleanupInterval > 0 && certmagicCfg.ExpiredGrace > 0 && certmagicCfg.UntrackedGrace > 0 {
		group.Go(func() error {
			return sslCleanupLoop(groupCtx, logger, acmeManager, acmeWatcher, certmagicCfg.CleanupInterval, certmagicCfg.ExpiredGrace, certmagicCfg.UntrackedGrace)
		})
	}

	// launch the main service
	group.Go(func() error {
		logger.Info("starting server", "addr", serverCfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	group.Go(serverShutdown(groupCtx, logger, "server", server, serverCfg.ShutdownTimeout))

	if err := group.Wait(); err != nil {
		logger.Error("server error", "error", err)
	}
}
