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
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/warpcomdev/cuesix/internal/cache"
	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/dispatcher"
	"github.com/warpcomdev/cuesix/internal/listener"
	"github.com/warpcomdev/cuesix/internal/plugin"
	"github.com/warpcomdev/cuesix/internal/reloader"
	"github.com/warpcomdev/cuesix/internal/validator"
)

type compilerAdapter struct{}

func (compilerAdapter) Compile(fses ...fs.FS) (map[string]any, error) {
	return compiler.Compile(fses...)
}

type pluginCache struct {
	plugins plugin.Plugin
	cache   *cache.Cache
}

func (p *pluginCache) Changed(value map[string]any) (string, error) {
	updated, err := p.plugins.Update(value)
	if err != nil {
		return "", err
	}
	return p.cache.Changed(updated)
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
	listenAddr := flag.String("listen", envStringDefault("CUESIX_LISTEN", ":8080"), "listen address")
	cooldown := flag.Duration("cooldown", envDuration("CUESIX_COOLDOWN", 0), "cooldown duration")
	flag.Var(inputFlag, "input", "input directory (repeatable)")

	configPath := flag.String("apisix-config", envString("CUESIX_APISIX_CONFIG"), "apisix config path")
	apisixURL := flag.String("apisix-url", envString("CUESIX_APISIX_URL"), "apisix admin reload url")
	apiKey := flag.String("apisix-api-key", envString("CUESIX_APISIX_API_KEY"), "apisix admin api key")
	reloadMethod := flag.String("reload-method", envStringDefault("CUESIX_RELOAD_METHOD", http.MethodPost), "reload HTTP method")

	retryMax := flag.Int("retry-max", envInt("CUESIX_RETRY_MAX", 0), "reload retry attempts")
	retryInitial := flag.Duration("retry-initial", envDuration("CUESIX_RETRY_INITIAL", 200*time.Millisecond), "reload initial backoff")
	retryMaxDelay := flag.Duration("retry-max-delay", envDuration("CUESIX_RETRY_MAX_DELAY", 2*time.Second), "reload max backoff")
	retryMultiplier := flag.Float64("retry-multiplier", envFloat("CUESIX_RETRY_MULTIPLIER", 2), "reload backoff multiplier")
	sslPathsFlag := &stringSliceFlag{}
	if envSSLPaths := envString("CUESIX_PLUGIN_SSL_PATHS"); envSSLPaths != "" {
		sslPathsFlag.values = splitComma(envSSLPaths)
	}
	flag.Var(sslPathsFlag, "plugin-ssl-path", "ssl plugin certificate path (repeatable)")

	flag.Parse()

	if len(inputFlag.values) == 0 {
		log.Fatal("at least one --input or CUESIX_INPUT_DIRS is required")
	}

	fses, err := buildFilesystems(inputFlag.values)
	if err != nil {
		log.Fatalf("input dirs: %v", err)
	}

	if !*serve {
		merged, err := compiler.Compile(fses...)
		if err != nil {
			logger.Error("compile failed", "error", err)
			os.Exit(1)
		}
		plugins, err := buildPlugins(sslPathsFlag.values)
		if err != nil {
			logger.Error("plugin init failed", "error", err)
			os.Exit(1)
		}
		updated, err := plugins.Update(merged)
		if err != nil {
			logger.Error("plugin update failed", "error", err)
			os.Exit(1)
		}
		output, err := cache.MarshalDeterministicYAML(updated)
		if err != nil {
			logger.Error("render yaml failed", "error", err)
			os.Exit(1)
		}
		if _, err := os.Stdout.Write(output); err != nil {
			logger.Error("write output failed", "error", err)
			os.Exit(1)
		}
		return
	}

	if *configPath == "" {
		logger.Error("missing apisix config path")
		os.Exit(1)
	}
	if *apisixURL == "" {
		logger.Error("missing apisix url")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	disp, err := dispatcher.New(dispatcher.Config{
		Compiler:   compilerAdapter{},
		Cache: &pluginCache{
			plugins: func() plugin.Plugin {
				plugins, pluginErr := buildPlugins(sslPathsFlag.values)
				if pluginErr != nil {
					logger.Error("plugin init failed", "error", pluginErr)
					os.Exit(1)
				}
				return plugins
			}(),
			cache:   &cache.Cache{},
		},
		Validator:  validator.New(nil),
		Reloader: &reloader.Reloader{
			ConfigPath:      *configPath,
			ReloadURL:       *apisixURL,
			ReloadMethod:    *reloadMethod,
			APIKey:          *apiKey,
			RetryMax:        *retryMax,
			RetryInitial:    *retryInitial,
			RetryMaxDelay:   *retryMaxDelay,
			RetryMultiplier: *retryMultiplier,
			Logger:          logger,
		},
		Filesystems: fses,
		Cooldown:    *cooldown,
		Logger:      logger,
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
	server := &http.Server{
		Addr:              *listenAddr,
		Handler:           drainBody(handler),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		for {
			if err := disp.Run(ctx); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				logger.Error("dispatcher error", "error", err)
				continue
			}
		}
	}()
	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			logger.Error("server error", "error", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", "error", err)
	}
}

func buildPlugins(sslPaths []string) (plugin.Plugin, error) {
	var plugins plugin.Chain
	if len(sslPaths) > 0 {
		sslFSes, err := buildFilesystems(sslPaths)
		if err != nil {
			return nil, err
		}
		plugins = append(plugins, &plugin.SSLPlugin{Filesystems: sslFSes})
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
