package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/warpcomdev/cuesix/cmd/cuesix/config"
	"github.com/warpcomdev/cuesix/internal/cache"
	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/cursor"
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

	srvTimeouts := serverCfg.Timeouts()

	// Build input filesystem views.
	fses, err := buildFilesystems(inputCfg.InputDirs)
	if err != nil {
		log.Fatalf("input dirs: %v", err)
	}

	// Load fallback certificate for SSL plugin.
	var fallbackCert ssl.Certificate
	if cert, ok, err := pluginCfg.LoadFallbackCertificate(apisixCfg.Home, certmagicCfg.Enabled); ok {
		if err != nil {
			logger.Error("failed to load fallback certificate", "certPath", pluginCfg.FallbackCert, "keyPath", pluginCfg.FallbackKey, "error", err)
			os.Exit(1)
		}
		fallbackCert = cert
	}

	// Configure certmagic manager + watcher.
	acmeSetup, err := newAcmeSetup(logger, certmagicCfg)
	if err != nil {
		logger.Error("certmagic init failed", "error", err)
		os.Exit(1)
	}

	// Build plugin pipelines.
	pluginCacheInst := &pluginCache{
		preRender: func() plugin.PreRender {
			plugins, pluginErr := buildPreRender(pluginCfg.SSLPaths, acmeSetup.acmeWatcher, fallbackCert, pluginCfg.SSLACMETimeout)
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
	configPath := apisixCfg.ConfigPath(pluginCfg.EnableYAML)

	// Reload target configuration.
	reloadURL, err := reloadCfg.BuildURL()
	if err != nil {
		logger.Error("invalid apisix url", "error", err)
		os.Exit(1)
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
		acmeServer = buildServer(certmagicCfg.ChallengeAddr, acmeSetup.acmeManager.ChallengeHandler(logger), srvTimeouts)
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
			acmeSetup.acmeWatcher.RunWatch(groupCtx, logger, certmagicCfg.WatchInterval)
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
	if acmeSetup.acmeWatcher != nil {
		// Start the monitor that will reconfigure apisix when
		// certificates are renewed
		group.Go(func() error {
			acmeCursor := cursor.New(acmeSetup.acmeWatcher, 16)
			defer acmeCursor.Close()
			var err error
			refreshManager.Watch(groupCtx, logger, func(ctx context.Context) bool {
				var cancelled bool
				cancelled, _, err = acmeCursor.Next(ctx)
				return cancelled
			})
			return err
		})
	}

	// Launch the acme tracking cleanup
	if acmeSetup.acmeWatcher != nil && acmeSetup.shouldCleanup(certmagicCfg) {
		group.Go(func() error {
			return acmeSetup.cleanupLoop(groupCtx, logger, certmagicCfg)
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
