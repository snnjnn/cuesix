package factory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"iter"
	"log/slog"
	"net/http"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/warpcomdev/sixpack/cmd/sixpack/config"
	"github.com/warpcomdev/sixpack/internal/compiler"
	"github.com/warpcomdev/sixpack/internal/dispatcher"
	"github.com/warpcomdev/sixpack/internal/schema"
)

// SchemaFetcher wraps a base fetcher and validates each snippet against a JSON schema.
type SchemaFetcher struct {
	Fetcher dispatcher.Fetcher
	parsed  schema.ParsedSchema
	schema  *jsonschema.Schema
	Logger  *slog.Logger
}

func (f *SchemaFetcher) Fetch(fses ...fs.FS) iter.Seq2[compiler.Snippet, error] {
	logger := f.Logger
	if logger == nil {
		logger = slog.Default()
	}
	if f.schema == nil {
		logger.Warn("schema fetcher missing schema, falling back to default fetch")
		return f.Fetcher.Fetch(fses...)
	}
	return func(yield func(compiler.Snippet, error) bool) {
		if f.Fetcher == nil {
			yield(compiler.Snippet{}, errors.New("schema fetcher missing wrapped fetcher"))
			return
		}
		for snippet, err := range f.Fetcher.Fetch(fses...) {
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
				logger.Warn("schema validation failed", "path", snippet.Path, "error", resp.Error)
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

func NewSchemaFetcher(fetcher dispatcher.Fetcher, logger *slog.Logger) (*SchemaFetcher, error) {
	if fetcher == nil {
		return nil, errors.New("schema fetcher requires a base fetcher")
	}
	return &SchemaFetcher{Fetcher: fetcher, Logger: logger}, nil
}

func (f *SchemaFetcher) LoadSchema(ctx context.Context, logger *slog.Logger, apiControlCfg config.APIControl) error {
	if f == nil {
		return errors.New("schema fetcher is nil")
	}
	baseURL := strings.TrimSpace(apiControlCfg.URL)
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

// Does a deep copy of an object by serializing to json
// and then deserializing again
func deepcopy(in any) (any, error) {
	buffer := bytes.NewBuffer(make([]byte, 0, 4096))
	encoder := json.NewEncoder(buffer)
	if err := encoder.Encode(in); err != nil {
		return nil, err
	}
	var result any
	decoder := json.NewDecoder(buffer)
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}
