package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"net/url"
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
	"github.com/warpcomdev/cuesix/internal/plugin/ssl"
	"github.com/warpcomdev/cuesix/internal/reloader"
	"github.com/warpcomdev/cuesix/internal/validator"
	"golang.org/x/sync/errgroup"
)

type compilerAdapter struct{}

func (compilerAdapter) Compile(logger *slog.Logger, fses ...fs.FS) (map[string]any, error) {
	return compiler.Compile(logger, fses...)
}

type pluginCache struct {
	preRender  plugin.PreRender
	postRender plugin.PostRender
	cache      *cache.Cache
}

func (p *pluginCache) Changed(logger *slog.Logger, value map[string]any) ([]byte, error) {
	logger.Info("pre-render plugins start")
	updated, err := p.preRender.Update(logger, value)
	if err != nil {
		return nil, err
	}
	logger.Info("pre-render plugins complete")
	logger.Info("cache normalization start")
	result, err := p.cache.Changed(logger, updated)
	if result == nil || err != nil {
		return nil, err
	}
	logger.Info("cache normalization complete")
	logger.Info("post-render plugins start")
	output, err := p.postRender.Update(logger, result)
	if err != nil {
		return nil, err
	}
	logger.Info("post-render plugins complete")
	return output, nil
}

type stringSliceFlag struct {
	values []string
	set    bool
}

func (s *stringSliceFlag) String() string {
	return strings.Join(s.values, ",")
}

func (s *stringSliceFlag) Set(value string) error {
	s.set = true
	s.values = append(s.values, value)
	return nil
}

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

	serve := flag.Bool("serve", envBool("CUESIX_SERVE", false), "run HTTP server")
	listenAddr := flag.String("listen", envStringDefault("CUESIX_LISTEN", "127.0.0.1:8080"), "listen address")
	metricsAddr := flag.String("metrics", envStringDefault("CUESIX_METRICS_LISTEN", ":8081"), "metrics listen address (empty to disable)")
	cooldown := flag.Duration("cooldown", envDuration("CUESIX_COOLDOWN", 0), "cooldown duration")
	flag.Var(inputFlag, "input", "input directory (repeatable)")

	apisixHome := flag.String("apisix-home", envStringDefault("CUESIX_APISIX_HOME", "/usr/local/apisix"), "apisix home path")
	mirrorDirFlag := flag.String("mirror-dir", envString("CUESIX_MIRROR_DIR"), "apisix mirror directory (optional)")
	mirrorKeepFlag := flag.Bool("keep-mirror", envBool("CUESIX_KEEP_MIRROR", false), "Do not remove mirror on startup")
	validationTimeout := flag.Duration("validation-timeout", envDuration("CUESIX_VALIDATION_TIMEOUT", 30*time.Second), "timeout for apisix test")

	apisixURL := flag.String("apisix-url", envString("CUESIX_APISIX_URL"), "apisix admin base url")
	dryRun := flag.Bool("dry-run", envBool("CUESIX_DRY_RUN", false), "run pipeline without writing config or reloading apisix")
	apiKey := flag.String("apisix-api-key", envString("CUESIX_APISIX_API_KEY"), "apisix admin api key")
	reloadMethod := flag.String("reload-method", envStringDefault("CUESIX_RELOAD_METHOD", http.MethodPost), "reload HTTP method")
	reloadTimeout := flag.Duration("reload-timeout", envDuration("CUESIX_RELOAD_TIMEOUT", 10*time.Second), "timeout for reload HTTP request")
	retryMax := flag.Int("retry-max", envInt("CUESIX_RETRY_MAX", 0), "reload retry attempts")
	retryInitial := flag.Duration("retry-initial", envDuration("CUESIX_RETRY_INITIAL", 200*time.Millisecond), "reload initial backoff")
	retryMaxDelay := flag.Duration("retry-max-delay", envDuration("CUESIX_RETRY_MAX_DELAY", 2*time.Second), "reload max backoff")
	retryMultiplier := flag.Float64("retry-multiplier", envFloat("CUESIX_RETRY_MULTIPLIER", 2), "reload backoff multiplier")

	enableYAML := flag.Bool("plugin-yaml", envBool("CUESIX_PLUGIN_YAML", false), "enable yaml post-render plugin")
	sslPathsFlag := &stringSliceFlag{}
	if envSSLPaths := envString("CUESIX_PLUGIN_SSL_PATHS"); envSSLPaths != "" {
		sslPathsFlag.values = splitComma(envSSLPaths)
	}
	flag.Var(sslPathsFlag, "plugin-ssl-path", "ssl plugin certificate path (repeatable)")
	enableJQ := flag.Bool("plugin-jq", envBool("CUESIX_PLUGIN_JQ", true), "enable jq post-render plugin")
	jqTimeout := flag.Duration("plugin-jq-timeout", envDuration("CUESIX_PLUGIN_JQ_TIMEOUT", 10*time.Second), "timeout for jq transforms")

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

	fses, err := buildFilesystems(inputFlag.values)
	if err != nil {
		log.Fatalf("input dirs: %v", err)
	}

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	reloadURL := ""
	if apisixURL != nil && *apisixURL != "" {
		reloadURL, err = buildReloadURL(*apisixURL)
		if err != nil {
			logger.Error("invalid apisix url", "error", err)
			os.Exit(1)
		}
	}

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

	disp, err := dispatcher.New(dispatcher.Config{
		Compiler:    compilerAdapter{},
		Cache:       pluginCacheInst,
		Validator:   val,
		Reloader:    reloadTarget,
		Filesystems: fses,
		OutputYAML:  *enableYAML,
		Cooldown:    *cooldown,
	})
	if err != nil {
		logger.Error("dispatcher init failed", "error", err)
		os.Exit(1)
	}

	handler, err := listener.NewHandler(disp)
	if err != nil {
		logger.Error("listener init failed", "error", err)
		os.Exit(1)
	}
	server := buildServer(*listenAddr, handler)

	var metricsServer *http.Server
	if strings.TrimSpace(*metricsAddr) != "" {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())
		metricsServer = buildServer(*metricsAddr, metricsMux)
	}

	var acmeServer *http.Server
	if strings.TrimSpace(*certmagicChallengeAddr) != "" {
		acmeServer = buildServer(*certmagicChallengeAddr, acmeManager.ChallengeHandler(logger))
	}

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
		group.Go(func() error {
			logger.Info("starting acme server", "addr", *certmagicChallengeAddr)
			if err := acmeServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		})
		group.Go(serverShutdown(groupCtx, logger, "acme server", acmeServer, 10*time.Second))
	}

	group.Go(func() error {
		// Keep the dispatcher running until the context is cancelled
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

func serverShutdown(ctx context.Context, logger *slog.Logger, name string, server *http.Server, timeout time.Duration) func() error {
	return func() error {
		<-ctx.Done()
		cancelCtx, cancelFunc := context.WithTimeout(context.Background(), timeout)
		defer cancelFunc()
		if err := server.Shutdown(cancelCtx); err != nil {
			logger.Error("server shutdown error", "server", name, "error", err)
		}
		return nil
	}
}

func buildPreRender(sslPaths []string, acmeWatcher *certmagicmgr.Watcher, fallback certmagicmgr.Certificate) (plugin.PreRender, error) {
	var plugins plugin.PreRenderChain
	if len(sslPaths) > 0 {
		sslFSes, err := buildFilesystems(sslPaths)
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, &ssl.SSLPlugin{
			Filesystems: sslFSes,
			ACME:        acmeWatcher,
			Fallback:    fallback,
		})
	}
	return plugins, nil
}

func buildServer(listenAddr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              listenAddr,
		Handler:           drainBody(handler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func buildPostRender(enableJQ bool, enableYAML bool, jqTimeout time.Duration) (plugin.PostRender, error) {
	var plugins plugin.PostRenderChain
	if enableJQ {
		plugins = append(plugins, &plugin.JQPlugin{Timeout: jqTimeout})
	}
	if enableYAML {
		// YAMLPlugin siempre debe ser el último plugin
		plugins = append(plugins, &plugin.YAMLPlugin{})
	}
	return plugins, nil
}

func drainBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Body != nil {
			_, _ = io.Copy(io.Discard, r.Body)
			_ = r.Body.Close()
		}
		next.ServeHTTP(w, r)
	})
}

func buildFilesystems(paths []string) ([]fs.FS, error) {
	fses := make([]fs.FS, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		clean := filepath.Clean(path)
		if _, err := os.Stat(clean); err != nil {
			return nil, err
		}
		fses = append(fses, os.DirFS(clean))
	}
	return fses, nil
}

func splitComma(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func splitSemicolon(value string) []string {
	parts := strings.Split(value, ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func buildCertmagicProviders(specs []string) ([]certmagicmgr.ProviderConfig, error) {
	if len(specs) == 0 {
		return nil, errors.New("at least one certmagic provider is required")
	}
	providers := make([]certmagicmgr.ProviderConfig, 0, len(specs))
	for _, spec := range specs {
		cfg, err := certmagicmgr.ParseProviderSpec(spec)
		if err != nil {
			return nil, err
		}
		providers = append(providers, cfg)
	}
	return providers, nil
}

func envString(key string) string {
	return os.Getenv(key)
}

func envStringDefault(key, def string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return def
}

func envBool(key string, def bool) bool {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		switch strings.ToLower(value) {
		case "1", "true", "yes", "y", "on":
			return true
		case "0", "false", "no", "n", "off":
			return false
		default:
			return def
		}
	}
	return def
}

func buildReloadURL(base string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(base))
	if err != nil {
		return "", err
	}
	parsed.Path = "/apisix/admin/configs"
	query := parsed.Query()
	query.Set("reload", "true")
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

type dryRunReloader struct{}

func (r *dryRunReloader) Apply(_ context.Context, logger *slog.Logger, payload []byte, useApi bool) error {
	logger.Info("dry-run reload skipped", "bytes", len(payload))
	return nil
}

func envInt(key string, def int) int {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		var parsed int
		if _, err := fmt.Sscanf(value, "%d", &parsed); err == nil {
			return parsed
		}
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		var parsed float64
		if _, err := fmt.Sscanf(value, "%f", &parsed); err == nil {
			return parsed
		}
	}
	return def
}

func envDuration(key string, def time.Duration) time.Duration {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		if parsed, err := time.ParseDuration(value); err == nil {
			return parsed
		}
	}
	return def
}
