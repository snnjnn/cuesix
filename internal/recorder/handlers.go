package recorder

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"maps"
	"net/http"
	"slices"
	"strings"

	"github.com/warpcomdev/cuesix/internal/cache"
	"github.com/warpcomdev/cuesix/internal/compiler"
	"github.com/warpcomdev/cuesix/internal/schema"
	"go.yaml.in/yaml/v4"
)

const (
	// EtagScope defines a default scope assigned to E-Tags for client side cache
	SourceETagScope = "sources"
	ConfigETagScope = "configs"
)

// @Summary Fetch index of (object kind, id) => list of paths
// @Description <p>Returns an index of (object kind, id) => list of sources that contain the object</p>
// @Description <p>Every response includes `ETag`, `Last-Modified`, and `Cache-Control: public, max-age=0, must-revalidate`, so clients should reuse those values and revalidate with `If-None-Match` or `If-Modified-Since` to bump into `304 Not Modified` when nothing changed.</p>
// @Tags Config
// @Produce application/json
// @Header 200 {string} ETag "Current cache tag for the fragment list or content"
// @Header 200 {string} Last-Modified "Last modification timestamp in RFC1123 format"
// @Header 200 {string} Cache-Control "Indicates `public, max-age=0, must-revalidate` so clients always revalidate"
// @Param If-None-Match header string false "Previously returned ETag used to skip unnecessary downloads"
// @Param If-Modified-Since header string false "RFC1123 timestamp to compare against the `Last-Modified` header"
// @Param virtualgw path string true "Virtual gateway name"
// @Success 200 {object} map[string]map[string]Descriptor "Index of (kind id) to list of paths"
// @Failure 417 {string} string "No config committed"
// @Failure 400 {string} string "No virtual gateway specified"
// @Router /virtualgw/{virtualgw} [get]
func (rec *Recorder) GetIndex() http.Handler {
	if rec.Enumerator == nil {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastModified := rec.Enumerator.LastModified()
		if cache.Reply(lastModified, ConfigETagScope, w, r) {
			return
		}
		var index map[string]map[string]Descriptor
		virtualgw, ok := compiler.PathVirtualGateway(w, r)
		if !ok {
			return
		}
		instance, ok := rec.instances[virtualgw]
		if !ok {
			w.WriteHeader(http.StatusExpectationFailed)
			return
		}
		instance.WithLock(func() {
			index = instance.index
		})
		if index == nil {
			w.WriteHeader(http.StatusExpectationFailed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		marshaler := json.NewEncoder(w)
		marshaler.Encode(index)
	})
}

// @Summary Fetch a single config object
// @Description <p>Returns the raw YAML payload for a given config object.</p>
// @Description <p>Every response includes `ETag`, `Last-Modified`, and `Cache-Control: public, max-age=0, must-revalidate`, so clients should reuse those values and revalidate with `If-None-Match` or `If-Modified-Since` to bump into `304 Not Modified` when nothing changed.</p>
// @Tags Config
// @Produce application/yaml
// @Header 200 {string} ETag "Current cache tag for the fragment list or content"
// @Header 200 {string} Last-Modified "Last modification timestamp in RFC1123 format"
// @Header 200 {string} Cache-Control "Indicates `public, max-age=0, must-revalidate` so clients always revalidate"
// @Param virtualgw path string true "Virtual gateway name"
// @Param path path string true "Object path in the format {kind}/{id} (e.g., `routes/123`)"
// @Param If-None-Match header string false "Previously returned ETag used to skip unnecessary downloads"
// @Param If-Modified-Since header string false "RFC1123 timestamp to compare against the `Last-Modified` header"
// @Success 200 {string} string "YAML fragment content"
// @Failure 404 {string} string "Fragment not found"
// @Router /virtualgw/{virtualgw}/{path} [get]
func (rec *Recorder) GetConfig() http.Handler {
	if rec.Enumerator == nil {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		part := strings.SplitN(path, "/", 3)
		if len(part) != 3 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		virtualgw := part[0]
		lastModified := rec.Enumerator.LastModified()
		if cache.Reply(lastModified, ConfigETagScope, w, r) {
			return
		}
		var snippets map[string]compiler.Snippet
		var index map[string]map[string]Descriptor
		instance, ok := rec.instances[virtualgw]
		if !ok {
			w.WriteHeader(http.StatusExpectationFailed)
			return
		}
		instance.WithLock(func() {
			snippets = instance.snippets
			index = instance.index
		})
		if index == nil || snippets == nil {
			w.WriteHeader(http.StatusExpectationFailed)
			return
		}
		configs, ok := index[part[1]]
		if !ok || configs == nil || len(configs) == 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		desc, ok := configs[part[2]]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		paths := desc.Paths
		if len(paths) == 0 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		selection := make([]compiler.Snippet, 0, len(paths))
		for _, path := range desc.Paths {
			snippet, ok := snippets[path]
			if ok {
				selection = append(selection, snippet)
			}
		}
		result, err := compiler.Merge(rec.Logger, slices.Values(selection))
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(fmt.Appendf(nil, "Failed to merge snippets: %v", err))
			return
		}
		var encoded bytes.Buffer
		encoder := yaml.NewEncoder(&encoded)
		encoder.SetIndent(2)
		err = encoder.Encode(result)
		closeErr := encoder.Close()
		if err == nil {
			err = closeErr
		}
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write(fmt.Appendf(nil, "Failed to encode merged snippets: %v", err))
			return
		}
		w.Header().Set("Content-Type", "application/yaml")
		w.WriteHeader(http.StatusOK)
		w.Write(encoded.Bytes())
	})
}

// @Summary List known source fragments
// @Description <p>Returns discovered source fragments as a map of source key to leaf virtual gateway.</p>
// @Description <p>Every response includes `ETag`, `Last-Modified`, and `Cache-Control: public, max-age=0, must-revalidate`, so clients should reuse those values and revalidate with `If-None-Match` or `If-Modified-Since` to bump into `304 Not Modified` when nothing changed.</p>
// @Tags Source
// @Produce application/json
// @Header 200 {string} ETag "Current cache tag for the fragment list or content"
// @Header 200 {string} Last-Modified "Last modification timestamp in RFC1123 format"
// @Header 200 {string} Cache-Control "Indicates `public, max-age=0, must-revalidate` so clients always revalidate"
// @Param If-None-Match header string false "Previously returned ETag used to skip unnecessary downloads"
// @Param If-Modified-Since header string false "RFC1123 timestamp to compare against the `Last-Modified` header"
// @Success 200 {object} map[string]string "Map of source key to leaf virtual gateway"
// @Router /sources [get]
func (rec *Recorder) ListSources() http.Handler {
	if rec.Enumerator == nil {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastModified := rec.Enumerator.LastModified()
		if cache.Reply(lastModified, SourceETagScope, w, r) {
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(rec.Enumerator.SourceMap())
	})
}

// @Summary Fetch a single fragment
// @Description <p>Returns the raw YAML payload for a given source key.</p>
// @Description <p>Every response includes `ETag`, `Last-Modified`, and `Cache-Control: public, max-age=0, must-revalidate`, so clients should reuse those values and revalidate with `If-None-Match` or `If-Modified-Since` to bump into `304 Not Modified` when nothing changed.</p>
// @Tags Source
// @Produce application/yaml
// @Header 200 {string} ETag "Current cache tag for the fragment list or content"
// @Header 200 {string} Last-Modified "Last modification timestamp in RFC1123 format"
// @Header 200 {string} Cache-Control "Indicates `public, max-age=0, must-revalidate` so clients always revalidate"
// @Param path path string true "Opaque source key returned by `GET /sources`"
// @Param If-None-Match header string false "Previously returned ETag used to skip unnecessary downloads"
// @Param If-Modified-Since header string false "RFC1123 timestamp to compare against the `Last-Modified` header"
// @Success 200 {string} string "YAML fragment content"
// @Failure 404 {string} string "Fragment not found"
// @Router /sources/{path} [get]
func (rec *Recorder) GetSource() http.Handler {
	if rec.Enumerator == nil {
		return http.NotFoundHandler()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastModified := rec.Enumerator.LastModified()
		if cache.Reply(lastModified, SourceETagScope, w, r) {
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/")
		content, ok := rec.Enumerator.Get(path)
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
// @Description <p>Example URLs: `/schema/validate/&lt;source-key&gt;?DOMAIN=apisix.org&consumer=bob`.</p>
// @Description <p>Every response includes `ETag`, `Last-Modified`, and `Cache-Control: public, max-age=0, must-revalidate`, so clients should reuse those values and revalidate with `If-None-Match` or `If-Modified-Since` to bump into `304 Not Modified` when nothing changed.</p>
// @Tags Schema
// @Produce application/json
// @Header 200 {string} ETag "Current cache tag for the validation payload"
// @Header 200 {string} Last-Modified "Validation timestamp in RFC1123 format"
// @Header 200 {string} Cache-Control "Indicates `public, max-age=0, must-revalidate` so clients should revalidate"
// @Param path path string true "Opaque source key to validate"
// @Param If-None-Match header string false "ETag from a prior validation response"
// @Param If-Modified-Since header string false "Timestamp to compare against the validation's Last-Modified"
// @Success 200 {object} schema.ValidationResponse "Validation result"
// @Failure 404 {object} schema.ValidationResponse "Fragment not found"
// @Router /validate/{path} [get]
func (rec *Recorder) ValidateSource(manager *schema.Manager) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

		// Use parameters to build a deterministic scope for the etag
		query := r.URL.Query()
		env := make(map[string]string)
		hash := fnv.New128a()
		for _, key := range slices.Sorted(maps.Keys(query)) {
			val := query.Get(key)
			// Encode key/value boundaries to avoid ambiguous concatenations.
			fmt.Fprintf(hash, "%d:%s=%d:%s;", len(key), key, len(val), val)
			env[key] = val
		}
		scope := base64.StdEncoding.EncodeToString(hash.Sum(nil))
		ts := rec.Enumerator.LastModified()
		if cache.Reply(ts, scope, w, r) {
			return
		}

		// Get path to validate
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "__sample__" {
			// DEBUG
			schema.WriteValidationJSON(w, http.StatusOK, schema.ValidationResponse{
				Valid: true,
			})
			return
		}
		content, ok := rec.Enumerator.Get(path)
		if !ok {
			schema.WriteValidationJSON(w, http.StatusNotFound, schema.ValidationResponse{
				Valid: false,
				Error: fmt.Errorf("Path not found: %s", path),
			})
			return
		}

		// Get the schema
		_, schemaInstance, defaults, err := manager.GetSchema(r.Context())
		if err != nil {
			schema.WriteValidationJSON(w, http.StatusInternalServerError, schema.ValidationResponse{
				Valid: false,
				Error: fmt.Errorf("Path not found: %s", path),
			})
			return
		}

		// And run validation
		schema.WriteValidationJSON(w, http.StatusOK, schema.ValidationProbe{
			Payload: content,
			IsYaml:  true,
			Env:     env,
		}.Validate(schemaInstance, defaults))
	})
}
