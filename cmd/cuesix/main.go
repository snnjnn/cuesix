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
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/warpcomdev/cuesix/internal/cache"
	"github.com/warpcomdev/cuesix/internal/certmagicmgr"
	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/dispatcher"
	"github.com/warpcomdev/cuesix/internal/listener"
	"github.com/warpcomdev/cuesix/internal/plugin"
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

	inputFlag := &stringSliceFlag{}
	if envInputs := envString("CUESIX_INPUT_DIRS"); envInputs != "" {
		inputFlag.values = splitComma(envInputs)
	}

	// Input and runtime mode.
	serve := flag.Bool("serve", envBool("CUESIX_SERVE", false), "run HTTP server")
	listenAddr := flag.String("listen", envStringDefault("CUESIX_LISTEN", "127.0.0.1:8080"), "listen address")
	metricsAddr := flag.String("metrics", envStringDefault("CUESIX_METRICS_LISTEN", ":8081"), "metrics listen address (empty to disable)")
	cooldown := flag.Duration("cooldown", envDuration("CUESIX_COOLDOWN", 0), "cooldown duration")
	flag.Var(inputFlag, "input", "input directory (repeatable)")

	// APISIX paths and validation.
	apisixHome := flag.String("apisix-home", envStringDefault("CUESIX_APISIX_HOME", "/usr/local/apisix"), "apisix home path")
	mirrorDirFlag := flag.String("mirror-dir", envString("CUESIX_MIRROR_DIR"), "apisix mirror directory (optional)")
	mirrorKeepFlag := flag.Bool("keep-mirror", envBool("CUESIX_KEEP_MIRROR", false), "Do not remove mirror on startup")
	validationTimeout := flag.Duration("validation-timeout", envDuration("CUESIX_VALIDATION_TIMEOUT", 30*time.Second), "timeout for apisix test")

	// Reload behavior.
	apisixURL := flag.String("apisix-url", envString("CUESIX_APISIX_URL"), "apisix admin base url")
	dryRun := flag.Bool("dry-run", envBool("CUESIX_DRY_RUN", false), "run pipeline without writing config or reloading apisix")
	apiKey := flag.String("apisix-api-key", envString("CUESIX_APISIX_API_KEY"), "apisix admin api key")
	reloadMethod := flag.String("reload-method", envStringDefault("CUESIX_RELOAD_METHOD", http.MethodPost), "reload HTTP method")
	reloadTimeout := flag.Duration("reload-timeout", envDuration("CUESIX_RELOAD_TIMEOUT", 10*time.Second), "timeout for reload HTTP request")
	retryMax := flag.Int("retry-max", envInt("CUESIX_RETRY_MAX", 0), "reload retry attempts")
	retryInitial := flag.Duration("retry-initial", envDuration("CUESIX_RETRY_INITIAL", 200*time.Millisecond), "reload initial backoff")
	retryMaxDelay := flag.Duration("retry-max-delay", envDuration("CUESIX_RETRY_MAX_DELAY", 2*time.Second), "reload max backoff")
	retryMultiplier := flag.Float64("retry-multiplier", envFloat("CUESIX_RETRY_MULTIPLIER", 2), "reload backoff multiplier")

	// Plugins.
	enableYAML := flag.Bool("plugin-yaml", envBool("CUESIX_PLUGIN_YAML", false), "enable yaml post-render plugin")
	sslPathsFlag := &stringSliceFlag{}
	if envSSLPaths := envString("CUESIX_PLUGIN_SSL_PATHS"); envSSLPaths != "" {
		sslPathsFlag.values = splitComma(envSSLPaths)
	}
	flag.Var(sslPathsFlag, "plugin-ssl-path", "ssl plugin certificate path (repeatable)")
	enableJQ := flag.Bool("plugin-jq", envBool("CUESIX_PLUGIN_JQ", true), "enable jq post-render plugin")
	jqTimeout := flag.Duration("plugin-jq-timeout", envDuration("CUESIX_PLUGIN_JQ_TIMEOUT", 10*time.Second), "timeout for jq transforms")

	// Certmagic.
	certmagicEnabled := flag.Bool("certmagic", envBool("CUESIX_CERTMAGIC", false), "enable certmagic acme manager")
	certmagicDefaultProvider := flag.String("certmagic-default-provider", envString("CUESIX_CERTMAGIC_DEFAULT_PROVIDER"), "certmagic default provider")
	certmagicDataDir := flag.String("certmagic-data-dir", envString("CUESIX_CERTMAGIC_DATA_DIR"), "certmagic data directory")
	certmagicChallengeAddr := flag.String("certmagic-challenge-addr", envString("CUESIX_CERTMAGIC_CHALLENGE_ADDR"), "certmagic HTTP-01 challenge address")
	certmagicTimeout := flag.Duration("certmagic-timeout", envDuration("CUESIX_CERTMAGIC_TIMEOUT", 0), "certmagic default certificate obtain timeout")
	certmagicFallbackCert := flag.String("certmagic-fallback-cert", envString("CUESIX_CERTMAGIC_FALLBACK_CERT"), "fallback certificate path")
	certmagicFallbackKey := flag.String("certmagic-fallback-key", envString("CUESIX_CERTMAGIC_FALLBACK_KEY"), "fallback key path")
	certmagicProvidersFlag := &stringSliceFlag{}
	if envProviders := envString("CUESIX_CERTMAGIC_PROVIDERS"); envProviders != "" {
		for _, spec := range splitSemicolon(envProviders) {
			certmagicProvidersFlag.values = append(certmagicProvidersFlag.values, spec)
		}
	}
	flag.Var(certmagicProvidersFlag, "certmagic-provider", "certmagic provider config (repeatable)")

	flag.Parse()

	if len(inputFlag.values) == 0 {
		log.Fatal("at least one --input or CUESIX_INPUT_DIRS is required")
	}

	// Build input filesystem views.
	fses, err := buildFilesystems(inputFlag.values)
	if err != nil {
		log.Fatalf("input dirs: %v", err)
	}

	// Load fallback certificate for SSL plugin and certmagic.
	var fallbackCert certmagicmgr.Certificate
	if len(sslPathsFlag.values) > 0 || *certmagicEnabled {
		if *certmagicFallbackCert == "" {
			*certmagicFallbackCert = filepath.Join(*apisixHome, "conf", "cert", "ssl_PLACE_HOLDER.crt")
		}
		if *certmagicFallbackKey == "" {
			*certmagicFallbackKey = filepath.Join(*apisixHome, "conf", "cert", "ssl_PLACE_HOLDER.key")
		}
		fallbackCert, err = certmagicmgr.LoadFallbackCertificate(*certmagicFallbackCert, *certmagicFallbackKey)
		if err != nil {
			logger.Error("failed to load fallback certificate", "certPath", *certmagicFallbackCert, "keyPath", *certmagicFallbackKey, "error", err)
			os.Exit(1)
		}
	}

	// Configure certmagic manager + watcher.
	var (
		acmeManager *certmagicmgr.Manager
		acmeWatcher *certmagicmgr.Watcher
		events      chan certmagicmgr.CertEvent
	)
	if *certmagicEnabled {
		if *certmagicChallengeAddr == "" {
			logger.Error("certmagic enabled but challenge address is missing")
			os.Exit(1)
		}
		providers, err := buildCertmagicProviders(certmagicProvidersFlag.values)
		if err != nil {
			logger.Error("certmagic provider config invalid", "error", err)
			os.Exit(1)
		}
		events = make(chan certmagicmgr.CertEvent, 32)
		acmeManager, err = certmagicmgr.NewManager(certmagicmgr.Config{
			Providers:       providers,
			DefaultProvider: strings.TrimSpace(*certmagicDefaultProvider),
			DataDir:         strings.TrimSpace(*certmagicDataDir),
			DefaultTimeout:  *certmagicTimeout,
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
			plugins, pluginErr := buildPreRender(sslPathsFlag.values, acmeWatcher, fallbackCert)
			if pluginErr != nil {
				logger.Error("pre-render plugin init failed", "error", pluginErr)
				os.Exit(1)
			}
			return plugins
		}(),
		cache: &cache.Cache{},
		postRender: func() plugin.PostRender {
			plugins, pluginErr := buildPostRender(*enableJQ, *enableYAML, *jqTimeout)
			if pluginErr != nil {
				logger.Error("post-render plugin init failed", "error", pluginErr)
				os.Exit(1)
			}
			return plugins
		}(),
	}

	// Standalone compile mode.
	if !*serve {
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
	if strings.TrimSpace(*apisixHome) == "" {
		logger.Error("missing apisix home path")
		os.Exit(1)
	}
	mirrorKeep := *mirrorKeepFlag
	mirrorDir := *mirrorDirFlag
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
	val, err := validator.New(*apisixHome, mirrorDir, mirrorKeep, *validationTimeout)
	if err != nil {
		logger.Error("prepare apisix mirror failed", "error", err)
		os.Exit(1)
	}
	configPath := validator.BuildConfigPath(*apisixHome, *enableYAML)

	// Reload target configuration.
	reloadURL := ""
	if apisixURL != nil && *apisixURL != "" {
		reloadURL, err = buildReloadURL(*apisixURL)
		if err != nil {
			logger.Error("invalid apisix url", "error", err)
			os.Exit(1)
		}
	}

	// Wire reloader (or dry-run).
	var reloadTarget dispatcher.Reloader
	if *dryRun {
		reloadTarget = &dryRunReloader{}
	} else {
		reloadTarget = &reloader.Reloader{
			ConfigPath:      configPath,
			ReloadURL:       reloadURL,
			ReloadMethod:    *reloadMethod,
			APIKey:          *apiKey,
			RetryMax:        *retryMax,
			RetryInitial:    *retryInitial,
			RetryMaxDelay:   *retryMaxDelay,
			RetryMultiplier: *retryMultiplier,
			RequestTimeout:  *reloadTimeout,
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
		OutputYAML:  *enableYAML,
		Cooldown:    *cooldown,
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
	server := buildServer(*listenAddr, handler)

	// Metrics server.
	var metricsServer *http.Server
	if strings.TrimSpace(*metricsAddr) != "" {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		metricsServer = buildServer(*metricsAddr, metricsMux)
	}

	// ACME challenge server.
	var acmeServer *http.Server
	if strings.TrimSpace(*certmagicChallengeAddr) != "" {
		acmeServer = buildServer(*certmagicChallengeAddr, acmeManager.ChallengeHandler(logger))
	}

	// Run loop and shutdown wiring.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	group, groupCtx := errgroup.WithContext(ctx)

	// Ready metrics and acme server first, because as soon as
	// I start the other services, I could get an acme request
	if metricsServer != nil {
		group.Go(func() error {
			logger.Info("starting metrics server", "addr", *metricsAddr)
			if err := metricsServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		})
		group.Go(serverShutdown(groupCtx, logger, "metrics server", metricsServer, 10*time.Second))
	}

	if acmeServer != nil {
		// Start the cert watcher
		group.Go(func() error {
			acmeWatcher.RunWatch(groupCtx, logger, 60*time.Second)
			return nil
		})
		// And the acme server
		group.Go(func() error {
			logger.Info("starting acme server", "addr", *certmagicChallengeAddr)
			if err := acmeServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		})
		group.Go(serverShutdown(groupCtx, logger, "acme server", acmeServer, 10*time.Second))
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

	// launch the main service
	group.Go(func() error {
		logger.Info("starting server", "addr", *listenAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	group.Go(serverShutdown(groupCtx, logger, "server", server, 10*time.Second))

	if err := group.Wait(); err != nil {
		logger.Error("server error", "error", err)
	}
}
