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
	"slices"
	"strings"
	"syscall"

	"github.com/cenkalti/backoff/v4"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v3"
	"github.com/warpcomdev/sixpack/cmd/sixpack/config"
	"github.com/warpcomdev/sixpack/cmd/sixpack/control"
	"github.com/warpcomdev/sixpack/cmd/sixpack/factory"
	"github.com/warpcomdev/sixpack/internal/app"
	"github.com/warpcomdev/sixpack/internal/compiler"
	"github.com/warpcomdev/sixpack/internal/cursor"
	"github.com/warpcomdev/sixpack/internal/dispatcher"
	"github.com/warpcomdev/sixpack/internal/listener"
	"github.com/warpcomdev/sixpack/internal/plugin"
	"github.com/warpcomdev/sixpack/internal/plugin/ssl"
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

	var (
		inputCfg        = config.Input{}
		apisixCfg       = config.APISIX{}
		pluginsConfig   = config.Plugins{}
		certmagicConfig = config.Certmagic{}
		serverConfig    = config.Server{}
		reloadConfig    = config.Reload{}
		apiControlCfg   = config.APIControl{}
	)

	app := &cli.Command{
		Name:  "sixpack",
		Usage: "compile APISIX standalone config from fragments",
		Commands: []*cli.Command{
			{
				Name:  "compile",
				Usage: "compile config and write to stdout",
				Flags: func() []cli.Flag {
					return joinFlags(&inputCfg, &apisixCfg, &pluginsConfig, &certmagicConfig, &reloadConfig, &apiControlCfg)
				}(),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					applyFlags(cmd, &inputCfg, &apisixCfg, &pluginsConfig, &certmagicConfig, &reloadConfig, &apiControlCfg)
					return run(logger, inputCfg, serverConfig, apisixCfg, reloadConfig, pluginsConfig, certmagicConfig, apiControlCfg, false)
				},
			},
			{
				Name:  "serve",
				Usage: "run HTTP server and reload APISIX on changes",
				Flags: func() []cli.Flag {
					return joinFlags(&inputCfg, &apisixCfg, &pluginsConfig, &certmagicConfig, &reloadConfig, &serverConfig, &apiControlCfg)
				}(),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					applyFlags(cmd, &inputCfg, &apisixCfg, &pluginsConfig, &certmagicConfig, &reloadConfig, &serverConfig, &apiControlCfg)
					return run(logger, inputCfg, serverConfig, apisixCfg, reloadConfig, pluginsConfig, certmagicConfig, apiControlCfg, true)
				},
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(logger *slog.Logger, inputCfg config.Input, serverCfg config.Server, apisixCfg config.APISIX, reloadCfg config.Reload, pluginCfg config.Plugins, certmagicCfg config.Certmagic, apiControlCfg config.APIControl, serve bool) error {

	// Validate common configs
	if err := inputCfg.Validate(); err != nil {
		return err
	}
	if err := certmagicCfg.Validate(); err != nil {
		return err
	}
	if err := pluginCfg.Validate(); err != nil {
		return err
	}

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

	// Build enumerator, fetcher, and compiler factory
	sourceEnumerator := app.NewSourcesEnumerator(logger, nil)
	var enumerator compiler.Enumerator = sourceEnumerator
	if pluginCfg.EnvFilename != "" {
		enumerator = plugin.NewEnvEnumerator(logger, enumerator, pluginCfg.EnvFilename)
	}
	var (
		fetcherInstance dispatcher.Fetcher = compiler.NewFetcher(logger, enumerator)
		schemaFetcher   *factory.SchemaFetcher
	)
	compFactory := factory.CompilerFactory{
		Logger: logger,
	}
	if apisixCfg.UseSchema {
		schemaFetcher, err = factory.NewSchemaFetcher(fetcherInstance, logger)
		if err != nil {
			logger.Error("schema fetcher init failed", "error", err)
			return err
		}
		fetcherInstance = schemaFetcher
	}

	// Build plugin pipelines.
	scheduler := factory.NewScheduler()
	serFactory, err := factory.NewSerializer(logger, pluginCfg, sslSetup, scheduler)
	if err != nil {
		logger.Error("serializer factory failed", "error", err)
		return err
	}

	// Start the work group and the ssl event tracker, in case we enabled certmagic.
	// This allows us to get certs either in serve or compile mode, as long
	// as we receive challenges.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Allow external cancellation of the whole errgroup
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	group, groupCtx := errgroup.WithContext(cancelCtx)
	if sslSetup.AcmeTracker != nil {
		group.Go(func() error {
			if sslSetup.Events != nil {
				ssl.UpdateLoop(groupCtx, logger, sslSetup.AcmeTracker, cursor.Channel(sslSetup.Events))
			}
			return nil
		})
	}

	// Standalone compile mode.
	if !serve {
		defer func() {
			cancelFunc()
			group.Wait()
		}()
		// Build the fetcher. This might block early, but since we
		// are running in comnpile mode, we don't mind
		if schemaFetcher != nil {
			if err := schemaFetcher.LoadSchema(groupCtx, logger, apiControlCfg); err != nil {
				logger.Error("schema fetcher failed", "error", err)
				return err
			}
		}
		// Fetch the inputs
		snippets := make([]compiler.Snippet, 0, 16)
		for snippet, err := range fetcherInstance.Fetch(fses...) {
			if err != nil {
				logger.Error("fetcher failed", "error", err, "path", snippet.Path)
				return err
			}
			snippets = append(snippets, snippet)
		}
		// Simulate a dispatcher run: create the instances,
		// merge, and serialize.
		compilerInstance := compFactory.Instance()
		compilerInstance.Reset()
		merged, err := compilerInstance.Merge(slices.Values(snippets))
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
		// Dump the pipeline result
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

	// Reload action
	reloadTarget, err := reloadCfg.BuildReloader(logger, apisixCfg, pluginCfg)
	if err != nil {
		logger.Error("failed to build reloader", "error", err)
		return err
	}

	// Ready manager: intercepts dispatch calls to
	// register successful executions
	readyManager := newReadyManager(reloadTarget, scheduler)

	// Dispatcher wiring.
	disp, err := dispatcher.New(logger, dispatcher.Config{
		Fetcher: fetcherInstance,
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

	// Buffer notifications. They will be queued until the dispatcher
	// actually starts, but we allow any sidecar to succeed early.
	// It also starts /live and /ready endpoints
	handler, err := listener.NewHandler(readyManager)
	srvTimeouts := serverCfg.Timeouts()
	if err != nil {
		logger.Error("listener init failed", "error", err)
		return err
	}
	server := buildServer(serverCfg.ListenAddr, handler, srvTimeouts)
	group.Go(func() error {
		logger.Info("starting compile server", "addr", serverCfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	group.Go(serverShutdown(groupCtx, server, serverCfg.ShutdownTimeout))

	// Start other local endpoints
	var metricsServer *http.Server
	if strings.TrimSpace(serverCfg.MetricsAddr) != "" {
		schemaClient := &http.Client{Timeout: apiControlCfg.Timeout}
		validationHandler := app.NewValidationHandler(logger, apiControlCfg.URL, apiControlCfg.APIKey, schemaClient, apiControlCfg.Timeout, false, sourceEnumerator, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 3))
		metricsMux := http.NewServeMux()
		control.RegisterAPI(metricsMux, validationHandler)
		metricsMux.Handle("GET /metrics", promhttp.Handler())
		metricsServer = buildServer(serverCfg.MetricsAddr, metricsMux, srvTimeouts)
		group.Go(func() error {
			logger.Info("starting metrics server", "addr", serverCfg.MetricsAddr)
			if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		})
		group.Go(serverShutdown(groupCtx, metricsServer, serverCfg.ShutdownTimeout))
	}

	// With local endpoints running, but dispatcher still pending,
	// we check if we are using schema, and block further process
	// until we get the schema.
	if schemaFetcher != nil {
		if err := schemaFetcher.LoadSchema(groupCtx, logger, apiControlCfg); err != nil {
			logger.Warn("failed to parse schema!", "error", err)
		}
	}

	// Finally, ready the dispatcher
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

	// Ready the cleanup loops
	if sslSetup.Enabled {
		group.Go(func() error {
			serFactory.CommitLoop(groupCtx, logger, certmagicCfg.CleanupInterval, certmagicCfg.UntrackedGrace)
			return nil
		})
		group.Go(func() error {
			serFactory.ExpireLoop(groupCtx, logger, certmagicCfg.CleanupInterval, certmagicCfg.ExpiredGrace)
			return nil
		})
	}

	// If configured to auto-trigger, go ahead
	if serverCfg.AutoTrigger {
		disp.Notify()
	}
	return group.Wait()
}

type flagConfig interface {
	Flags() []cli.Flag
	Apply(ctx *cli.Command)
}

func joinFlags(cfgs ...flagConfig) []cli.Flag {
	flags := make([]cli.Flag, 0, 16)
	for _, cfg := range cfgs {
		flags = append(flags, cfg.Flags()...)
	}
	return flags
}

func applyFlags(cmd *cli.Command, cfgs ...flagConfig) {
	for _, cfg := range cfgs {
		cfg.Apply(cmd)
	}
}
