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
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/lmittmann/tint"
	"github.com/mattn/go-isatty"
	"github.com/warpcomdev/sixpack/cmd/sixpack/control"
	_ "github.com/warpcomdev/sixpack/cmd/sixpack/docs"
	"github.com/warpcomdev/sixpack/internal/app"
	"github.com/warpcomdev/sixpack/internal/schema"
)

func main() {
	var (
		baseURL string
		apiKey  string
		loose   bool
		serve   bool
		listen  string
	)

	log.SetFlags(log.LstdFlags | log.LUTC)
	w := os.Stderr
	logger := slog.New(
		tint.NewHandler(w, &tint.Options{
			NoColor: !isatty.IsTerminal(w.Fd()),
		}),
	)
	slog.SetDefault(logger)

	flag.StringVar(&baseURL, "url", "", "APISIX base URL (for example: http://127.0.0.1:9090)")
	flag.StringVar(&apiKey, "api-key", "", "APISIX admin API key (optional)")
	flag.BoolVar(&loose, "loose", false, "emit loose schema (keep APISIX ID flexibility)")
	flag.BoolVar(&serve, "serve", false, "serve schema at /schema and playground at /schema/app")
	flag.StringVar(&listen, "listen", "127.0.0.1:8082", "listen address for --serve")
	flag.Parse()

	if strings.TrimSpace(baseURL) == "" {
		fmt.Fprintln(os.Stderr, "missing required --url")
		os.Exit(2)
	}

	// @title Sixpack Schema Control API
	// @version 1.0
	// @description Control plane helpers for introspecting the APISIX schema and validating source fragments. The API exposes a few cached read endpoints; clients should revalidate changes by reusing the ETag/Last-Modified headers that the server always emits and honoring `Cache-Control: public, max-age=0, must-revalidate`.
	// @BasePath /
	if serve {
		schemaClient := &http.Client{Timeout: 10 * time.Second}
		schemaMux := app.NewValidationHandler(logger, baseURL, apiKey, schemaClient, 10*time.Second, false, nil, backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 3))
		mux := http.NewServeMux()
		control.RegisterAPI(mux, schemaMux)
		server := &http.Server{
			Addr:              listen,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		}
		fmt.Fprintf(os.Stderr, "serving schema on http://%s\n", listen)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(1)
		}
		return
	}

	var (
		payload schema.NormalizedSchema
		err     error
	)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bo := backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 3)
	payload, err = buildSchema(ctx, logger, http.DefaultClient, baseURL, apiKey, bo, !loose)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}

	if _, err := os.Stdout.Write(payload.Normalized); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	if len(payload.Normalized) == 0 || payload.Normalized[len(payload.Normalized)-1] != '\n' {
		fmt.Fprintln(os.Stdout)
	}
}

// BuildSchema fetches the APISIX schema and returns a normalized JSON schema payload.
func buildSchema(ctx context.Context, logger *slog.Logger, client *http.Client, baseURL, apiKey string, backoffCfg backoff.BackOff, strict bool) (schema.NormalizedSchema, error) {
	raw, err := schema.FetchSchemaUntil(ctx, logger, client, baseURL, apiKey, backoffCfg)
	if err != nil {
		return schema.NormalizedSchema{}, err
	}
	return schema.NormalizeSchema(raw, strict)
}
