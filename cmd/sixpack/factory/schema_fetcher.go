package factory

import (
	"context"
	"errors"
	"iter"
	"log/slog"
	"net/http"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/warpcomdev/cuesix/cmd/sixpack/config"
	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/dispatcher"
	"github.com/warpcomdev/cuesix/internal/schema"
)

// SchemaFetcher wraps a base fetcher and validates each snippet against a JSON schema.
type SchemaFetcher struct {
	Fetcher dispatcher.Fetcher
	parsed  schema.ParsedSchema
	schema  *jsonschema.Schema
	Logger  *slog.Logger
}

// Fetch delegates fetching and logs schema-validation failures per snippet.
func (f *SchemaFetcher) Fetch() iter.Seq2[compiler.Snippet, error] {
	logger := f.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if f.schema == nil {
		logger.Warn("schema fetcher missing schema, falling back to default fetch")
		return f.Fetcher.Fetch()
	}
	return func(yield func(compiler.Snippet, error) bool) {
		if f.Fetcher == nil {
			yield(compiler.Snippet{}, errors.New("schema fetcher missing wrapped fetcher"))
			return
		}
		for snippet, err := range f.Fetcher.Fetch() {
			if err != nil {
				if !yield(snippet, err) {
					return
				}
				continue
			}
			data, err := deepcopy(snippet.Data)
			if err != nil {
				logger.Warn("Failed to deep copy data prior to defaults insertion", "err", err)
				if !yield(snippet, nil) {
					return
				}
				continue
			}
			schema.ApplyDefaults(f.parsed, data)
			if resp := schema.Validate(f.schema, data); !resp.Valid {
				// Schema error is not deemed a fatal flaw, just flagged
				logger.Warn("schema validation failed", "source", snippet.Ref.Key(), "error", resp.Error)
				if !yield(snippet, nil) {
					return
				}
				continue
			}
			if !yield(snippet, nil) {
				return
			}
		}
	}
}

// NewSchemaFetcher wraps a base fetcher with schema-aware behavior.
func NewSchemaFetcher(fetcher dispatcher.Fetcher, logger *slog.Logger) (*SchemaFetcher, error) {
	if fetcher == nil {
		return nil, errors.New("schema fetcher requires a base fetcher")
	}
	return &SchemaFetcher{Fetcher: fetcher, Logger: logger}, nil
}

// LoadSchema downloads, normalizes, and compiles the APISIX schema.
func (f *SchemaFetcher) LoadSchema(ctx context.Context, logger *slog.Logger, apiControlCfg config.APIControl) error {
	if f == nil {
		return errors.New("schema fetcher is nil")
	}
	baseURL := strings.TrimSpace(apiControlCfg.ControlURL)
	if baseURL == "" {
		return errors.New("apisix-use-schema requires --apisix-control-url")
	}
	schemaClient := &http.Client{Timeout: apiControlCfg.Timeout}
	raw, err := schema.FetchSchemaUntil(ctx, logger, schemaClient, baseURL, apiControlCfg.APIKey, apiControlCfg.BuildBackoff(true))
	if err != nil {
		return err
	}
	normalized, err := schema.NormalizeSchema(raw, false)
	if err != nil {
		return err
	}
	parsed, compiled, err := schema.Compile(normalized)
	if err != nil {
		return err
	}
	f.schema = compiled
	f.parsed = parsed
	return nil
}

// Does a deep copy of a YAML/JSON-like object graph made of maps, lists, and
// scalar values.
func deepcopy(in any) (any, error) {
	switch typed := in.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			copied, err := deepcopy(value)
			if err != nil {
				return nil, err
			}
			out[key] = copied
		}
		return out, nil
	case []any:
		out := make([]any, len(typed))
		for i, value := range typed {
			copied, err := deepcopy(value)
			if err != nil {
				return nil, err
			}
			out[i] = copied
		}
		return out, nil
	default:
		return typed, nil
	}
}
