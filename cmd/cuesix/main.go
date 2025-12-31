package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v2"
	"github.com/warpcomdev/cuesix/cmd/cuesix/config"
	"github.com/warpcomdev/cuesix/internal/cache"
	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/cursor"
	"github.com/warpcomdev/cuesix/internal/dispatcher"
	"github.com/warpcomdev/cuesix/internal/listener"
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

	commonFlags := func(inputCfg *config.Input, apisixCfg *config.APISIX, pluginCfg *config.Plugins) []cli.Flag {
		flags := make([]cli.Flag, 0, 16)
		flags = append(flags, inputCfg.Flags()...)
		flags = append(flags, apisixCfg.Flags()...)
		flags = append(flags, pluginCfg.Flags()...)
		return flags
	}
	serveFlags := func(serverCfg *config.Server, reloadCfg *config.Reload, certmagicCfg *config.Certmagic) []cli.Flag {
		flags := make([]cli.Flag, 0, 16)
		flags = append(flags, serverCfg.Flags()...)
		flags = append(flags, reloadCfg.Flags()...)
		flags = append(flags, certmagicCfg.Flags()...)
		return flags
	}

	app := &cli.App{
		Name:  "cuesix",
		Usage: "compile APISIX standalone config from fragments",
		Commands: []*cli.Command{
			{
				Name:  "compile",
				Usage: "compile config and write to stdout",
				Flags: func() []cli.Flag {
					inputCfg := &config.Input{}
					apisixCfg := &config.APISIX{}
					pluginCfg := &config.Plugins{}
					return commonFlags(inputCfg, apisixCfg, pluginCfg)
				}(),
				Action: func(ctx *cli.Context) error {
					inputCfg := config.Input{}
					apisixCfg := config.APISIX{}
					pluginCfg := config.Plugins{}

					inputCfg.Apply(ctx)
					apisixCfg.Apply(ctx)
					pluginCfg.Apply(ctx)

					if err := inputCfg.Validate(); err != nil {
						return err
					}

					return run(logger, inputCfg, config.Server{}, apisixCfg, config.Reload{}, pluginCfg, config.Certmagic{}, false)
				},
			},
			{
				Name:  "serve",
				Usage: "run HTTP server and reload APISIX on changes",
				Flags: func() []cli.Flag {
					inputCfg := &config.Input{}
					apisixCfg := &config.APISIX{}
					pluginCfg := &config.Plugins{}
					certmagicCfg := &config.Certmagic{}
					serverCfg := &config.Server{}
					reloadCfg := &config.Reload{}
					flags := commonFlags(inputCfg, apisixCfg, pluginCfg)
					flags = append(flags, serveFlags(serverCfg, reloadCfg, certmagicCfg)...)
					return flags
				}(),
				Action: func(ctx *cli.Context) error {
					inputCfg := config.Input{}
					apisixCfg := config.APISIX{}
					pluginCfg := config.Plugins{}
					certmagicCfg := config.Certmagic{}
					serverCfg := config.Server{}
					reloadCfg := config.Reload{}

					inputCfg.Apply(ctx)
					apisixCfg.Apply(ctx)
					pluginCfg.Apply(ctx)
					certmagicCfg.Apply(ctx)
					serverCfg.Apply(ctx)
					reloadCfg.Apply(ctx)

					if err := inputCfg.Validate(); err != nil {
						return err
					}
					if err := certmagicCfg.Validate(); err != nil {
						return err
					}

					return run(logger, inputCfg, serverCfg, apisixCfg, reloadCfg, pluginCfg, certmagicCfg, true)
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(logger *slog.Logger, inputCfg config.Input, serverCfg config.Server, apisixCfg config.APISIX, reloadCfg config.Reload, pluginCfg config.Plugins, certmagicCfg config.Certmagic, serve bool) error {
	srvTimeouts := serverCfg.Timeouts()

	// Build input filesystem views.
	fses, err := buildFilesystems(inputCfg.InputDirs)
	if err != nil {
		return fmt.Errorf("input dirs: %w", err)
	}

	// Load fallback certificate for SSL plugin.
	var fallbackCert ssl.Certificate
	enableSSL := pluginCfg.EnableSSL || len(pluginCfg.SSLPaths) > 0 || certmagicCfg.Enabled
	if cert, ok, err := pluginCfg.LoadFallbackCertificate(apisixCfg.Home, enableSSL); ok {
		if err != nil {
			logger.Error("failed to load fallback certificate", "certPath", pluginCfg.FallbackCert, "keyPath", pluginCfg.FallbackKey, "error", err)
			return err
		}
		fallbackCert = cert
	}

	// Configure certmagic manager + watcher.
	acmeSetup, err := newAcmeSetup(logger, certmagicCfg)
	if err != nil {
		logger.Error("certmagic init failed", "error", err)
		return err
	}

	// Build plugin pipelines.
	preRender, err := buildPreRender(pluginCfg, acmeSetup.acmeWatcher, fallbackCert)
	if err != nil {
		logger.Error("pre-render plugin init failed", "error", err)
		return err
	}
	postRender, err := buildPostRender(pluginCfg.EnableJQ, pluginCfg.EnableYAML, pluginCfg.JQTimeout)
	if err != nil {
		logger.Error("post-render plugin init failed", "error", err)
		return err
	}
	pluginCacheInst := &pluginCache{
		preRender:  preRender,
		cache:      &cache.Cache{},
		postRender: postRender,
	}

	// Standalone compile mode.
	if !serve {
		merged, err := compiler.Compile(logger, fses...)
		if err != nil {
			logger.Error("compile failed", "error", err)
			return err
		}
		output, err := pluginCacheInst.Changed(logger, merged)
		if err != nil {
			logger.Error("plugin pipeline failed", "error", err)
			return err
		}
		if output == nil {
			logger.Error("unexpected nil output from plugin pipeline")
			return errors.New("unexpected nil output from plugin pipeline")
		}
		if _, err := os.Stdout.Write(output); err != nil {
			logger.Error("write output failed", "error", err)
			return err
		}
		return nil
	}

	// APISIX validation and mirror setup.
	if strings.TrimSpace(apisixCfg.Home) == "" {
		logger.Error("missing apisix home path")
		return errors.New("missing apisix home path")
	}
	mirrorKeep := apisixCfg.KeepMirror
	mirrorDir := apisixCfg.MirrorDir
	if mirrorDir == "" {
		tmp, tmpErr := os.MkdirTemp("", "cuesix-apisix-")
		if tmpErr != nil {
			logger.Error("create apisix mirror dir failed", "error", tmpErr)
			return tmpErr
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
		return err
	}
	configPath := apisixCfg.ConfigPath(pluginCfg.EnableYAML)

	// Reload target configuration.
	reloadURL, err := reloadCfg.BuildURL()
	if err != nil {
		logger.Error("invalid apisix url", "error", err)
		return err
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
		return err
	}
	refreshManager.realDispatcher = disp

	// HTTP handlers.
	handler, err := listener.NewHandler(refreshManager)
	if err != nil {
		logger.Error("listener init failed", "error", err)
		return err
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

	return group.Wait()
}
