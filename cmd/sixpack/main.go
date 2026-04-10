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
	"github.com/warpcomdev/cuesix/cmd/sixpack/config"
	"github.com/warpcomdev/cuesix/cmd/sixpack/control"
	"github.com/warpcomdev/cuesix/cmd/sixpack/factory"
	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/cursor"
	"github.com/warpcomdev/cuesix/internal/dispatcher"
	"github.com/warpcomdev/cuesix/internal/listener"
	"github.com/warpcomdev/cuesix/internal/plugin"
	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
	"github.com/warpcomdev/cuesix/internal/recorder"
	"github.com/warpcomdev/cuesix/internal/schema"
	"github.com/warpcomdev/cuesix/internal/sse"
	"github.com/warpcomdev/cuesix/internal/validator"
	"golang.org/x/sync/errgroup"
)

type fullConfig struct {
	Input               config.Input
	Apisix              config.Apisix
	StandaloneValidator config.StandaloneValidator
	Plugins             config.Plugins
	Certmagic           config.Certmagic
	HTTPServer          config.HTTPServer
	Server              config.Server
	Reload              config.Reload
	APIControl          config.APIControl
	ServerSideEvents    config.ServerSideEvents
	Client              config.Client
}

func main() {

	log.SetFlags(log.LstdFlags | log.LUTC)
	w := os.Stderr
	logger := slog.New(
		tint.NewHandler(w, &tint.Options{
			NoColor: !isatty.IsTerminal(w.Fd()),
		}),
	)
	slog.SetDefault(logger)

	var cfg fullConfig

	compileFlags := []flagConfig{
		&cfg.Input,
		&cfg.Apisix,
		&cfg.StandaloneValidator,
		&cfg.Plugins,
		&cfg.Certmagic,
		&cfg.Reload,
		&cfg.APIControl,
	}
	serveFlags := []flagConfig{
		&cfg.Input,
		&cfg.Apisix,
		&cfg.StandaloneValidator,
		&cfg.Plugins,
		&cfg.Certmagic,
		&cfg.Reload,
		&cfg.HTTPServer,
		&cfg.Server,
		&cfg.APIControl,
		&cfg.ServerSideEvents,
	}
	clientFlags := []flagConfig{
		&cfg.Reload,
		&cfg.Apisix,
		&cfg.Client,
		&cfg.HTTPServer,
	}

	app := &cli.Command{
		Name:  "sixpack",
		Usage: "compile APISIX standalone config from fragments",
		Commands: []*cli.Command{
			{
				Name:  "compile",
				Usage: "compile config and write to stdout",
				Flags: func() []cli.Flag {
					return joinFlags(compileFlags...)
				}(),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					applyFlags(cmd, compileFlags...)
					if err := validateFlags(serveFlags...); err != nil {
						return err
					}
					return run(logger, cfg, false)
				},
			},
			{
				Name:  "serve",
				Usage: "run HTTP server and reload APISIX on changes",
				Flags: func() []cli.Flag {
					return joinFlags(serveFlags...)
				}(),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					applyFlags(cmd, serveFlags...)
					if err := validateFlags(serveFlags...); err != nil {
						return err
					}
					return run(logger, cfg, true)
				},
			},
			{
				Name:  "client",
				Usage: "watch a remote sixpack SSE endpoint and apply received configs",
				Flags: func() []cli.Flag {
					return joinFlags(clientFlags...)
				}(),
				Action: func(ctx context.Context, cmd *cli.Command) error {
					applyFlags(cmd, clientFlags...)
					if err := validateFlags(clientFlags...); err != nil {
						return err
					}
					return runClient(logger, cfg)
				},
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func run(logger *slog.Logger, cfg fullConfig, serve bool) error {
	// Build input filesystem views.
	var (
		fses compiler.Input
		err  error
	)
	fses, err = compiler.InputFromPaths(cfg.Input.InputDirs)
	if err != nil {
		return fmt.Errorf("input dirs: %w", err)
	}

	// Configure SSL dependencies
	sslSetup, err := factory.NewSSLSetup(logger, cfg.Plugins, cfg.Certmagic, cfg.Apisix)
	if err != nil {
		logger.Error("SSL setup init failed", "error", err)
		return err
	}

	// Build input, resolver, enumerator, fetcher, and compiler factory
	if cfg.Plugins.EnvFilename != "" {
		fses, err = plugin.NewEnvInput(logger, fses, cfg.Plugins.EnvFilename)
		if err != nil {
			logger.Error("env input init failed", "error", err)
			return err
		}
	}
	var resolver compiler.Resolver = compiler.DefaultResolver{
		VirtualGateway: compiler.FromKey(cfg.Apisix.Virtualgw),
	}
	if cfg.Input.GatewayFromDots {
		// Derive the virtualgateway from directory name. The directory name
		// is split in prefix.suffix, and only prefix is used as virtual gw name.
		resolver = control.GatewayFromDots{
			Resolver: resolver,
		}
	}
	sourceEnumerator, err := recorder.NewSourcesEnumerator(logger, compiler.NewEnumerator(logger, fses, resolver))
	if err != nil {
		logger.Error("enumerator init failed", "error", err)
		return err
	}
	var enumerator compiler.Enumerator = sourceEnumerator
	var (
		fetcherInstance dispatcher.Fetcher = compiler.NewFetcher(logger, enumerator)
		schemaFetcher   *factory.SchemaFetcher
	)
	compFactory := factory.CompilerFactory{
		Logger: logger,
		// Need to enable deepcopy to add labels to the snippets,
		// without each virtualgw overwriting every other vgw
		DeepCopy: cfg.Apisix.EnableLabels,
	}
	if cfg.StandaloneValidator.UseSchema {
		schemaFetcher, err = factory.NewSchemaFetcher(fetcherInstance, logger)
		if err != nil {
			logger.Error("schema fetcher init failed", "error", err)
			return err
		}
		fetcherInstance = schemaFetcher
	}

	// Build plugin pipelines.
	scheduler := factory.NewScheduler()
	serFactory, err := factory.NewSerializer(logger, cfg.Plugins, cfg.Apisix, sslSetup, scheduler)
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
		// are running in compile mode, we don't mind
		if schemaFetcher != nil {
			if err := schemaFetcher.LoadSchema(groupCtx, logger, cfg.APIControl); err != nil {
				logger.Error("schema fetcher failed", "error", err)
				return err
			}
		}
		// Fetch the inputs
		snippets := make([]compiler.Snippet, 0, 16)
		for snippet, err := range fetcherInstance.Fetch() {
			if err != nil {
				logger.Error("fetcher failed", "error", err, "source", snippet.Ref.Key())
				return err
			}
			snippets = append(snippets, snippet)
		}
		// Simulate a dispatcher run: create the instances,
		// merge, and serialize.
		compilerInstance := compFactory.Instance(cfg.Apisix.Virtualgw)
		compilerInstance.Reset()
		merged, err := compilerInstance.Merge(slices.Values(snippets))
		if err != nil {
			logger.Error("compile failed", "error", err)
			return err
		}
		pluginInstance := serFactory.Instance(cfg.Apisix.Virtualgw)
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

	// APISIX validation and recorder setup.
	var validatorInstance factory.SingletonValidator
	if cfg.Reload.DryRun || cfg.APIControl.DeploymentMode != config.StandaloneMode {
		validatorInstance = config.DryRunValidator{}
	} else {
		realValidator, err := cfg.StandaloneValidator.BuildValidator(logger, cfg.Apisix.Home)
		if err != nil {
			logger.Error("build apisix validator failed", "error", err)
			return err
		}
		defer func() {
			if err := realValidator.Cleanup(); err != nil {
				logger.Error("validator cleanup failed", "error", err, "mirrorDir", realValidator.MirrorDir())
			}
		}()
		validatorInstance = realValidator
	}
	validatorFactory := factory.NewHierarchicalValidatorFactory(
		cfg.StandaloneValidator.MaxGatewayDepth,
		factory.NewSingletonValidatorFactory(validatorInstance),
	)
	dataRecorder := recorder.NewRecorder(logger, sourceEnumerator, validatorFactory)

	// Reload action
	reloadTarget, err := cfg.Reload.BuildReloader(logger, cfg.Apisix.Virtualgw, validator.BuildConfigPath(cfg.Apisix.Home, cfg.Apisix.OutputYAML))
	if err != nil {
		logger.Error("failed to build reloader", "error", err)
		return err
	}

	// Capture reloads if SSE plugin is enabled
	var sseReloader *sse.Reloader
	if cfg.ServerSideEvents.KeepAlive > 0 {
		sseReloader = sse.New(logger, reloadTarget)
		reloadTarget = sseReloader
	}

	// Ready manager: intercepts dispatch calls to
	// register successful executions
	maxGatewayDepth := cfg.StandaloneValidator.MaxGatewayDepth
	readyManager := control.NewReadyManager(reloadTarget, scheduler, maxGatewayDepth)

	// Dispatcher wiring.
	disp, err := dispatcher.New(logger, dispatcher.Config{
		Fetcher:           fetcherInstance,
		MergerFactory:     compFactory,
		SerializerFactory: &serFactory,
		ValidatorFactory:  dataRecorder,
		Reloader:          readyManager,
		Scheduler:         scheduler.Must,
		Filesystems:       fses,
		OutputYAML:        cfg.Apisix.OutputYAML,
		Cooldown:          cfg.Input.Cooldown,
	})
	if err != nil {
		logger.Error("dispatcher init failed", "error", err)
		return err
	}
	readyManager.SetDispatcher(disp)

	// Buffer notifications. They will be queued until the dispatcher
	// actually starts, but we allow any sidecar to succeed early.
	// It also starts /live and /ready endpoints
	controlMux, err := listener.NewHandler(readyManager, readyManager)
	srvTimeouts := cfg.HTTPServer.Timeouts()
	if err != nil {
		logger.Error("listener init failed", "error", err)
		return err
	}
	if cfg.ServerSideEvents.KeepAlive > 0 && sseReloader != nil {
		// Add default and per-gateway SSE routes if enabled
		controlMux.Handle("GET /final/full", http.RedirectHandler(fmt.Sprintf("/final/full/%s", cfg.Apisix.Virtualgw), http.StatusTemporaryRedirect))
		controlMux.Handle("GET /final/full/{virtualgw}", sseReloader.HandleFull())
		controlMux.Handle("GET /final/sse", http.RedirectHandler(fmt.Sprintf("/final/sse/%s", cfg.Apisix.Virtualgw), http.StatusTemporaryRedirect))
		controlMux.Handle("GET /final/sse/{virtualgw}", sseReloader.HandleSSE(cfg.ServerSideEvents.KeepAlive))
	}
	server := control.BuildServer(cfg.HTTPServer.ListenAddr, controlMux, srvTimeouts)
	group.Go(func() error {
		logger.Info("starting compile server", "addr", cfg.HTTPServer.ListenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	group.Go(control.ServerShutdown(groupCtx, server, cfg.HTTPServer.ShutdownTimeout))

	// Start other local endpoints
	var metricsServer *http.Server
	if strings.TrimSpace(cfg.Server.MetricsAddr) != "" {
		schemaClient := &http.Client{Timeout: cfg.APIControl.Timeout}
		validationHandler := schema.NewManager(logger, cfg.APIControl.ControlURL, cfg.APIControl.APIKey, schemaClient, cfg.APIControl.Timeout, false, backoff.WithMaxRetries(cfg.APIControl.BuildBackoff(true), 3))
		metricsMux := http.NewServeMux()
		control.RegisterAPI(metricsMux, dataRecorder, validationHandler)
		metricsMux.Handle("GET /schema/virtualgw", readyManager.GatewaysHandler())
		metricsMux.Handle("GET /metrics", promhttp.Handler())
		metricsServer = control.BuildServer(cfg.Server.MetricsAddr, metricsMux, srvTimeouts)
		group.Go(func() error {
			logger.Info("starting metrics server", "addr", cfg.Server.MetricsAddr)
			if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		})
		group.Go(control.ServerShutdown(groupCtx, metricsServer, cfg.HTTPServer.ShutdownTimeout))
	}

	// With local endpoints running, but dispatcher still pending,
	// we check if we are using schema, and block further process
	// until we get the schema.
	if schemaFetcher != nil {
		if err := schemaFetcher.LoadSchema(groupCtx, logger, cfg.APIControl); err != nil {
			logger.Warn("failed to parse schema!", "error", err)
		}
	}

	// Finally, ready the dispatcher
	group.Go(func() error {
		logger.Info("starting dispatcher loop")
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
			logger.Info("starting ssl cleanup loop")
			serFactory.CommitLoop(groupCtx, logger, cfg.Certmagic.CleanupInterval, cfg.Certmagic.UntrackedGrace)
			return nil
		})
		group.Go(func() error {
			logger.Info("starting ssl expiration loop")
			serFactory.ExpireLoop(groupCtx, logger, cfg.Certmagic.CleanupInterval, cfg.Certmagic.ExpiredGrace)
			return nil
		})
	}

	// If configured to auto-trigger, go ahead
	if cfg.Server.AutoTrigger {
		disp.Notify()
	}
	return group.Wait()
}

func runClient(logger *slog.Logger, cfg fullConfig) error {
	reloadTarget, err := cfg.Reload.BuildReloader(logger, cfg.Apisix.Virtualgw, validator.BuildConfigPath(cfg.Apisix.Home, cfg.Apisix.OutputYAML))
	if err != nil {
		return err
	}
	// We are only interested in the readiness of the virtualgw we are subscribed too,
	// we have to make sure to use the proper nesting depth
	maxGatewayDepth := strings.Count(cfg.Apisix.Virtualgw, compiler.VIRTUALGW_SEP)
	readyReloader := control.NewReadyReloader(reloadTarget, maxGatewayDepth)
	reloader := &readyReloader
	httpClient, err := sse.NewHttpClient(cfg.Client.ConnectTimeout, cfg.Client.ReadTimeout)
	sseClient, err := sse.NewClient(
		logger,
		reloader,
		cfg.Apisix.Virtualgw,
		cfg.Client.BaseURL,
		httpClient,
		cfg.Client.ReadTimeout,
		cfg.Client.BuildBackoffFactory(),
	)
	if err != nil {
		return err
	}

	// Capture signals
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	group, groupCtx := errgroup.WithContext(ctx)

	// Launch a mini-server with /live and /ready endpoints
	controlMux, err := listener.NewHandler(nil, reloader)
	readyServer := control.BuildServer(cfg.HTTPServer.ListenAddr, controlMux, cfg.HTTPServer.Timeouts())
	group.Go(func() error {
		logger.Info("starting client ready server", "addr", cfg.HTTPServer.ListenAddr)
		if err := readyServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	group.Go(control.ServerShutdown(groupCtx, readyServer, cfg.HTTPServer.ShutdownTimeout))

	// Launch the SSE client loop
	group.Go(func() error {
		logger.Info("starting sse client", "url", cfg.Client.BaseURL)
		sseClient.Loop(groupCtx, nil)
		return nil
	})
	return group.Wait()
}

type flagConfig interface {
	Flags() []cli.Flag
	Apply(ctx *cli.Command)
	Validate() error
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

func validateFlags(cfgs ...flagConfig) error {
	errs := make([]error, 0, len(cfgs))
	for _, cfg := range cfgs {
		if err := cfg.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}
