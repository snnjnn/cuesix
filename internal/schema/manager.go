package schema

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/warpcondev/cuesix/internal/cursor"
)

// handles calls to /validate
type Manager struct {
	cursor.Lock
	// Public parameters
	Logger  *slog.Logger
	BaseURL string
	Client  *http.Client
	Timeout time.Duration
	ApiKey  string
	Loose   bool
	Backoff backoff.BackOff
	// Caches
	cached    NormalizedSchema
	compiled  *jsonschema.Schema
	schemaDoc ParsedSchema
}

// NewValidationHandler constructs the HTTP validation handler dependencies.
func NewManager(logger *slog.Logger, baseURL, apiKey string, client *http.Client, timeout time.Duration, loose bool, bo backoff.BackOff) *Manager {
	return &Manager{
		Logger:  logger,
		BaseURL: baseURL,
		ApiKey:  apiKey,
		Client:  client,
		Timeout: timeout,
		Loose:   loose,
		Backoff: bo,
	}
}

// GetSchema returns the cached compiled schema or fetches and compiles it.
func (m *Manager) GetSchema(ctx context.Context) (NormalizedSchema, *jsonschema.Schema, ParsedSchema, error) {
	if strings.TrimSpace(m.BaseURL) == "" {
		return NormalizedSchema{}, nil, ParsedSchema{}, errors.New("missing apisix base url")
	}
	m.Lock.Lock()
	defer m.Lock.Unlock()
	cached, compiled, schemaDoc := m.cached, m.compiled, m.schemaDoc
	if cached.Normalized != nil && compiled != nil && schemaDoc.Parsed != nil {
		return cached, compiled, schemaDoc, nil
	}
	client := m.Client
	if client == nil {
		client = http.DefaultClient
	}
	if m.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, m.Timeout)
		defer cancel()
	}
	raw, err := FetchSchemaUntil(ctx, m.Logger, client, m.BaseURL, m.ApiKey, m.Backoff)
	if err != nil {
		return NormalizedSchema{}, nil, ParsedSchema{}, err
	}
	cached, err = NormalizeSchema(raw, !m.Loose)
	if err != nil {
		return NormalizedSchema{}, nil, ParsedSchema{}, err
	}
	schemaDoc, compiled, err = Compile(cached)
	if err != nil {
		return NormalizedSchema{}, nil, ParsedSchema{}, err
	}
	m.cached = cached
	m.compiled = compiled
	m.schemaDoc = schemaDoc
	return cached, compiled, schemaDoc, nil
}
