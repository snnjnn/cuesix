package main

import (
	"context"
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
	"github.com/warpcondev/cuesix/internal/schema"
)

func main() {
	var (
		baseURL string
		apiKey  string
		loose   bool
		timeout time.Duration
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
	flag.DurationVar(&timeout, "timeout", 10*time.Second, "request timeout")
	flag.Parse()

	if strings.TrimSpace(baseURL) == "" {
		fmt.Fprintln(os.Stderr, "missing required --url")
		os.Exit(2)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	client := &http.Client{Timeout: timeout}
	retries := backoff.WithMaxRetries(backoff.NewExponentialBackOff(), 3)
	raw, err := schema.FetchSchemaUntil(ctx, logger, client, baseURL, apiKey, retries)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	payload, err := schema.NormalizeSchema(raw, !loose)
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
