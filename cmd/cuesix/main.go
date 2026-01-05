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

	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v3"
	"github.com/warpcomdev/cuesix/cmd/cuesix/config"
	"github.com/warpcomdev/cuesix/cmd/cuesix/factory"
	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/cursor"
	"github.com/warpcomdev/cuesix/internal/dispatcher"
	"github.com/warpcomdev/cuesix/internal/listener"
	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
	"golang.org/x/sync/errgroup"
)

func main() {
	log.SetFlags(log.LstdFlags | log.LUTC)
	w := os.Stderr
	logger := slog.New(
		tint.NewHandler(w, &tint.Options{
			NoColor: !isatty.IsTerminal(w.Fd()),
		}),
	)
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

	app := &cli.Command{
		Name:  "cuesix",
		Usage: "compile APISIX standalone config from fragments",
		Commands: []*cli.Command{
			{
				Name:  "compile",
				Usage: "compile config and write to stdout",
				Flags: func() []cli.Flag {
					inputCfg := &config.Input{}
					apisixCfg := &config.APISIX{}
					pluginsConfig := &config.Plugins{}
					return commonFlags(inputCfg, apisixCfg, pluginsConfig)
				}(),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					inputCfg := config.Input{}
					apisixCfg := config.APISIX{}
					pluginsConfig := config.Plugins{}

					inputCfg.Apply(cmd)
					apisixCfg.Apply(cmd)
					pluginsConfig.Apply(cmd)

					if err := inputCfg.Validate(); err != nil {
						return err
					}

					return run(logger, inputCfg, config.Server{}, apisixCfg, config.Reload{}, pluginsConfig, config.Certmagic{}, false)
				},
			},
			{
				Name:  "serve",
				Usage: "run HTTP server and reload APISIX on changes",
				Flags: func() []cli.Flag {
					inputCfg := &config.Input{}
					apisixCfg := &config.APISIX{}
					pluginsConfig := &config.Plugins{}
					certmagicConfig := &config.Certmagic{}
					serverConfig := &config.Server{}
					reloadConfig := &config.Reload{}
					flags := commonFlags(inputCfg, apisixCfg, pluginsConfig)
					flags = append(flags, serveFlags(serverConfig, reloadConfig, certmagicConfig)...)
					return flags
				}(),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					inputCfg := config.Input{}
					apisixCfg := config.APISIX{}
					pluginsConfig := config.Plugins{}
					certmagicConfig := config.Certmagic{}
					serverConfig := config.Server{}
					reloadConfig := config.Reload{}

					inputCfg.Apply(cmd)
					apisixCfg.Apply(cmd)
					pluginsConfig.Apply(cmd)
					certmagicConfig.Apply(cmd)
					serverConfig.Apply(cmd)
					reloadConfig.Apply(cmd)

					if err := inputCfg.Validate(); err != nil {
						return err
					}
					if err := certmagicConfig.Validate(); err != nil {
						return err
					}

					return run(logger, inputCfg, serverConfig, apisixCfg, reloadConfig, pluginsConfig, certmagicConfig, true)
				},
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(logger *slog.Logger, inputCfg config.Input, serverCfg config.Server, apisixCfg config.APISIX, reloadCfg config.Reload, pluginCfg config.Plugins, certmagicCfg config.Certmagic, serve bool) error {

	// Build input filesystem views.
	fses, err := factory.BuildFilesystems(inputCfg.InputDirs)
	if err != nil {
		return fmt.Errorf("input dirs: %w", err)
	}

	// Configure SSL dependencies
	sslSetup, err := factory.NewSSLSetup(logger, pluginCfg, certmagicCfg, apisixCfg)
	if err != nil {
		logger.Error("SSL setup init failed", "error", err)
		return err
	}

	// Build Compiler factory
	compFactory := factory.CompilerFactory{
		Logger: logger,
	}

	// Build plugin pipelines.
	scheduler := factory.NewScheduler()
	serFactory, err := factory.NewSerializer(logger, pluginCfg, sslSetup, scheduler)
	if err != nil {
		logger.Error("serializer factory failed", "error", err)
		return err
	}

	// Standalone compile mode.
	if !serve {
		merged, err := compiler.Compile(logger, fses...)
		if err != nil {
			logger.Error("compile failed", "error", err)
			return err
		}
		pluginInstance := serFactory.Instance()
		pluginInstance.Reset()
		output, err := pluginInstance.Serialize(merged)
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
	validator, err := apisixCfg.BuildValidator(logger)
	if err != nil {
		logger.Error("build apisix validator failed", "error", err)
		return err
	}
	defer func() {
		if err := validator.Cleanup(); err != nil {
			logger.Error("validator cleanup failed", "error", err, "mirrorDir", validator.MirrorDir())
		}
	}()
	valFactory := factory.ValidatorFactory{
		Validator: validator,
	}

	// Wire reloader (or dry-run).
	reloadTarget, err := reloadCfg.BuildReloader(logger, apisixCfg, pluginCfg)
	if err != nil {
		logger.Error("failed to build reloader", "error", err)
		return err
	}

	// Dispatcher wiring.
	readyManager := newReadyManager(reloadTarget, scheduler)
	disp, err := dispatcher.New(logger, dispatcher.Config{
		Fetcher: factory.CompilerFactory{},
		MergerFactory: func() dispatcher.Merger {
			return compFactory.Instance()
		},
		SerializerFactory: func() dispatcher.Serializer {
			return serFactory.Instance()
		},
		ValidatorFactory: func() dispatcher.Validator {
			return valFactory.Instance()
		},
		Reloader:    readyManager,
		Scheduler:   scheduler.Must,
		Filesystems: fses,
		OutputYAML:  pluginCfg.EnableYAML,
		Cooldown:    inputCfg.Cooldown,
	})
	if err != nil {
		logger.Error("dispatcher init failed", "error", err)
		return err
	}
	readyManager.realDispatcher = disp

	// HTTP handlers.
	handler, err := listener.NewHandler(readyManager)
	srvTimeouts := serverCfg.Timeouts()
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
		group.Go(serverShutdown(groupCtx, metricsServer, serverCfg.ShutdownTimeout))
	}

	if sslSetup.AcmeTracker != nil {
		// Start the cert watcher
		group.Go(func() error {
			if sslSetup.Events != nil {
				ssl.UpdateLoop(groupCtx, logger, sslSetup.AcmeTracker, cursor.Channel(sslSetup.Events))
			}
			return nil
		})
	}

	// Ready the dispatcher
	group.Go(func() error {
		for {
			if err := disp.Run(groupCtx); err != nil {
				if errors.Is(err, context.Canceled) {
					return nil
				}
				logger.Error("dispatcher error", "error", err)
				continue
			}
		}
	})

	// Ready the notifiers
	if sslSetup.Enabled {
		// Launch the acme tracking cleanup
		group.Go(func() error {
			serFactory.CommitLoop(groupCtx, logger, certmagicCfg.CleanupInterval, certmagicCfg.UntrackedGrace)
			return nil
		})
		// Start the monitor that will reconfigure apisix when
		// certificates are renewed
		group.Go(func() error {
			serFactory.ExpireLoop(groupCtx, logger, certmagicCfg.CleanupInterval, certmagicCfg.ExpiredGrace)
			return nil
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
	group.Go(serverShutdown(groupCtx, server, serverCfg.ShutdownTimeout))

	if serverCfg.AutoTrigger {
		disp.Notify()
	}
	return group.Wait()
}
