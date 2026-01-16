package schema

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/warpcomdev/cuesix/internal/cursor"
)

// handles calls to /validate
type ValidationHandler struct {
	cursor.Lock
	// Public parameters
	Logger     *slog.Logger
	Enumerator *SourcesEnumerator
	BaseURL    string
	Client     *http.Client
	Timeout    time.Duration
	ApiKey     string
	Loose      bool
	Backoff    backoff.BackOff
	// Caches
	cached    NormalizedSchema
	compiled  *jsonschema.Schema
	schemaDoc ParsedSchema
}

func NewValidationHandler(logger *slog.Logger, baseURL, apiKey string, client *http.Client, timeout time.Duration, loose bool, sources *SourcesEnumerator, bo backoff.BackOff) *ValidationHandler {
	return &ValidationHandler{
		Logger:     logger,
		Enumerator: sources,
		BaseURL:    baseURL,
		ApiKey:     apiKey,
		Client:     client,
		Timeout:    timeout,
		Loose:      loose,
		Backoff:    bo,
	}
}

func (vh *ValidationHandler) isCached(w http.ResponseWriter, r *http.Request) bool {
	ts := vh.Enumerator.LastModified()
	etag, lastModified := cacheTag(ts, "sources")
	if notModified(r, ts, etag) {
		applyCacheHeaders(w, etag, lastModified)
		w.WriteHeader(http.StatusNotModified)
		return true
	}
	applyCacheHeaders(w, etag, lastModified)
	return false
}

// @Summary List known source fragments
// @Description <p>Returns the list of discovered fragment paths.</p>
// @Description <p>Every response includes `ETag`, `Last-Modified`, and `Cache-Control: public, max-age=0, must-revalidate`, so clients should reuse those values and revalidate with `If-None-Match` or `If-Modified-Since` to bump into `304 Not Modified` when nothing changed.</p>
// @Tags Schema
// @Produce application/json
// @Header 200 {string} ETag "Current cache tag for the fragment list or content"
// @Header 200 {string} Last-Modified "Last modification timestamp in RFC1123 format"
// @Header 200 {string} Cache-Control "Indicates `public, max-age=0, must-revalidate` so clients always revalidate"
// @Param If-None-Match header string false "Previously returned ETag used to skip unnecessary downloads"
// @Param If-Modified-Since header string false "RFC1123 timestamp to compare against the `Last-Modified` header"
// @Success 200 {array} string "List of available source paths"
// @Failure 404 {string} string "No sources found"
// @Router /sources [get]
func (vh *ValidationHandler) ListSources() http.Handler {
	if vh.Enumerator == nil {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vh.isCached(w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(vh.Enumerator.ListPaths())
	})
}

// @Summary Fetch a single fragment
// @Description <p>Returns the raw YAML payload for a given path.</p>
// @Description <p>Every response includes `ETag`, `Last-Modified`, and `Cache-Control: public, max-age=0, must-revalidate`, so clients should reuse those values and revalidate with `If-None-Match` or `If-Modified-Since` to bump into `304 Not Modified` when nothing changed.</p>
// @Tags Schema
// @Produce application/yaml
// @Header 200 {string} ETag "Current cache tag for the fragment list or content"
// @Header 200 {string} Last-Modified "Last modification timestamp in RFC1123 format"
// @Header 200 {string} Cache-Control "Indicates `public, max-age=0, must-revalidate` so clients always revalidate"
// @Param path path string false "Relative fragment path (e.g., `routes/example.yaml`); omit to list paths."
// @Param If-None-Match header string false "Previously returned ETag used to skip unnecessary downloads"
// @Param If-Modified-Since header string false "RFC1123 timestamp to compare against the `Last-Modified` header"
// @Success 200 {string} string "YAML fragment content"
// @Failure 404 {string} string "Fragment not found"
// @Router /sources/{path} [get]
func (vh *ValidationHandler) GetSource() http.Handler {
	if vh.Enumerator == nil {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if vh.isCached(w, r) {
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		content, ok := vh.Enumerator.Get(path)
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		w.Write(content)
	})
}

// @Summary Validate a cached source fragment with environment overrides
// @Description <p>Validates the fragment referenced by the request path while treating every query parameter as an environment override.</p>
// @Description <p>Any number of query keys/values is allowed; they are applied to the snippet before validation, so clients can push substitutions such as `?DOMAIN=apisix.org` or `?consumer=bob`.</p>
// @Description <p>Example URLs: `/schema/validate/routes/example.yaml?DOMAIN=apisix.org&consumer=bob`.</p>
// @Description <p>Every response includes `ETag`, `Last-Modified`, and `Cache-Control: public, max-age=0, must-revalidate`, so clients should reuse those values and revalidate with `If-None-Match` or `If-Modified-Since` to bump into `304 Not Modified` when nothing changed.</p>
// @Tags Schema
// @Produce application/json
// @Header 200 {string} ETag "Current cache tag for the validation payload"
// @Header 200 {string} Last-Modified "Validation timestamp in RFC1123 format"
// @Header 200 {string} Cache-Control "Indicates `public, max-age=0, must-revalidate` so clients should revalidate"
// @Param path path string true "Relative fragment path to validate"
// @Param If-None-Match header string false "ETag from a prior validation response"
// @Param If-Modified-Since header string false "Timestamp to compare against the validation's Last-Modified"
// @Success 200 {object} ValidationResponse "Validation result"
// @Failure 404 {object} ValidationResponse "Fragment not found"
// @Router /validate/{path} [get]
func (vh *ValidationHandler) ValidateSource() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use parameters to build Etag
		query := r.URL.Query()
		env := make(map[string]string)
		hash := fnv.New128a()
		for _, key := range slices.Sorted(maps.Keys(query)) {
			val := query.Get(key)
			hash.Write([]byte(key))
			hash.Write([]byte(val))
			env[key] = val
		}

		scope := base64.StdEncoding.EncodeToString(hash.Sum(nil))
		ts := vh.Enumerator.LastModified()
		etag, lastModified := cacheTag(ts, scope)
		if notModified(r, ts, etag) {
			applyCacheHeaders(w, etag, lastModified)
			w.WriteHeader(http.StatusNotModified)
			return
		}

		// Get path to validate
		applyCacheHeaders(w, etag, lastModified)
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "__sample__" {
			// DEBUG
			writeValidationJSON(w, http.StatusOK, ValidationResponse{
				Valid: true,
			})
			return
		}
		content, ok := vh.Enumerator.Get(path)
		if !ok {
			writeValidationJSON(w, http.StatusNotFound, ValidationResponse{
				Valid: false,
				Error: fmt.Errorf("Path not found: %s", path),
			})
			return
		}

		// Get the schema
		_, schema, defaults, err := vh.GetSchema(r.Context())
		if err != nil {
			writeValidationJSON(w, http.StatusInternalServerError, ValidationResponse{
				Valid: false,
				Error: fmt.Errorf("Path not found: %s", path),
			})
			return
		}

		// And run validation
		writeValidationJSON(w, http.StatusOK, ValidationProbe{
			Payload: content,
			IsYaml:  true,
			Env:     env,
		}.Validate(schema, defaults))
	})
}

// @Summary Validate an inline payload against the schema
// @Description <p>Posts JSON or YAML content along with optional query params used as environment overrides.</p>
// @Description <p>Any number of query keys/values is allowed; they are applied to the snippet before validation, so clients can push substitutions such as `?DOMAIN=apisix.org` or `?consumer=bob`.</p>
// @Description <p>Example URLs: `/schema/validate/routes/example.yaml?DOMAIN=apisix.org&consumer=bob`.</p>
// @Description <p>Every response includes `ETag`, `Last-Modified`, and `Cache-Control: public, max-age=0, must-revalidate`, so clients should reuse those values and revalidate with `If-None-Match` or `If-Modified-Since` to bump into `304 Not Modified` when nothing changed.</p>
// @Tags Schema
// @Accept application/json
// @Produce application/json
// @Param body body string true "JSON or YAML payload to validate"
// @Success 200 {object} ValidationResponse "Validation result"
// @Failure 400 {object} ValidationResponse "Malformed or empty payload"
// @Router /validate [post]
func (vh *ValidationHandler) ValidateBody() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, schema, defaults, err := vh.GetSchema(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		probe, err := decodeValidationRequest(w, r)
		if err != nil {
			writeValidationJSON(w, http.StatusBadRequest, ValidationResponse{
				Valid: false,
				Error: err,
			})
			return
		}
		writeValidationJSON(w, http.StatusOK, probe.Validate(schema, defaults))
	})
}

// @Summary Retrieve the normalized APISIX JSON Schema
// @Description <p>Returns the cached normalized schema document that backs all validation endpoints.</p>
// @Tags Schema
// @Produce application/schema+json
// @Success 200 {string} string "Normalized JSON Schema document"
// @Router /json/ [get]
func (vh *ValidationHandler) Schema() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _, _, err := vh.GetSchema(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/schema+json")
		w.Write(payload.Normalized)
	})
}

func applyCacheHeaders(w http.ResponseWriter, etag, lastModified string) {
	if etag != "" {
		w.Header().Set("ETag", etag)
	}
	if lastModified != "" {
		w.Header().Set("Last-Modified", lastModified)
	}
	// Encourage clients to revalidate against our cache tags.
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
}

func notModified(r *http.Request, ts time.Time, etag string) bool {
	if etag != "" {
		for _, tag := range strings.Split(r.Header.Get("If-None-Match"), ",") {
			if strings.TrimSpace(tag) == etag || strings.TrimSpace(tag) == "*" {
				return true
			}
		}
	}
	if ims := r.Header.Get("If-Modified-Since"); ims != "" {
		if since, err := http.ParseTime(ims); err == nil && !ts.After(since) {
			return true
		}
	}
	return false
}

func cacheTag(ts time.Time, scope string) (string, string) {
	base := fmt.Sprintf("%d", ts.UnixNano())
	if scope != "" {
		base = base + ":" + scope
	}
	return fmt.Sprintf("\"%s\"", base), ts.UTC().Format(http.TimeFormat)
}

func (vh *ValidationHandler) GetSchema(ctx context.Context) (NormalizedSchema, *jsonschema.Schema, ParsedSchema, error) {
	if strings.TrimSpace(vh.BaseURL) == "" {
		return NormalizedSchema{}, nil, ParsedSchema{}, errors.New("missing apisix base url")
	}
	vh.Lock.Lock()
	defer vh.Unlock()
	cached, compiled, schemaDoc := vh.cached, vh.compiled, vh.schemaDoc
	if cached.Normalized != nil && compiled != nil && schemaDoc.Parsed != nil {
		return cached, compiled, schemaDoc, nil
	}
	client := vh.Client
	if client == nil {
		client = http.DefaultClient
	}
	if vh.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, vh.Timeout)
		defer cancel()
	}
	raw, err := FetchSchemaUntil(ctx, vh.Logger, client, vh.BaseURL, vh.ApiKey, vh.Backoff)
	if err != nil {
		return NormalizedSchema{}, nil, ParsedSchema{}, err
	}
	cached, err = NormalizeSchema(raw, !vh.Loose)
	if err != nil {
		return NormalizedSchema{}, nil, ParsedSchema{}, err
	}
	schemaDoc, compiled, err = Compile(cached)
	if err != nil {
		return NormalizedSchema{}, nil, ParsedSchema{}, err
	}
	vh.cached = cached
	vh.compiled = compiled
	vh.schemaDoc = schemaDoc
	return cached, compiled, schemaDoc, nil
}

func writeValidationJSON(w http.ResponseWriter, status int, payload ValidationResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}
