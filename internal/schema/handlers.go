package schema

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// @Summary Retrieve the normalized APISIX JSON Schema
// @Description <p>Returns the cached normalized schema document that backs all validation endpoints.</p>
// @Tags Schema
// @Produce application/schema+json
// @Success 200 {string} string "Normalized JSON Schema document"
// @Failure 502 {string} string "Schema backend unavailable"
// @Router /json [get]
func (m *Manager) Schema() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _, _, err := m.GetSchema(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/schema+json")
		w.Write(payload.Normalized)
	})
}

// @Summary Validate an inline payload against the schema
// @Description <p>Posts JSON or YAML content along with optional query params used as environment overrides.</p>
// @Description <p>Any number of query keys/values is allowed; they are applied to the snippet before validation, so clients can push substitutions such as `?DOMAIN=apisix.org` or `?consumer=bob`.</p>
// @Description <p>Example request: `POST /schema/validate?DOMAIN=apisix.org&consumer=bob` with the candidate payload in the request body.</p>
// @Tags Schema
// @Accept application/json,text/plain,application/yaml,text/yaml
// @Produce application/json
// @Param body body string true "JSON or YAML payload to validate"
// @Success 200 {object} ValidationResponse "Validation result"
// @Failure 400 {object} ValidationResponse "Malformed or empty payload"
// @Failure 500 {string} string "Schema backend unavailable"
// @Router /validate [post]
func (m *Manager) ValidateBody() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, schemaInstance, defaults, err := m.GetSchema(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		probe, err := decodeValidationRequest(w, r)
		if err != nil {
			WriteValidationJSON(w, http.StatusBadRequest, ValidationResponse{
				Valid: false,
				Error: err,
			})
			return
		}
		WriteValidationJSON(w, http.StatusOK, probe.Validate(schemaInstance, defaults))
	})
}

func decodeValidationRequest(w http.ResponseWriter, r *http.Request) (ValidationProbe, error) {
	query := r.URL.Query()
	env := make(map[string]string)
	for k := range query {
		env[k] = r.URL.Query().Get(k)
	}
	r.Body = http.MaxBytesReader(w, r.Body, SchemaMaxBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return ValidationProbe{}, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return ValidationProbe{}, errors.New("empty payload")
	}
	return ValidationProbe{
		Payload: body,
		IsYaml:  false,
		Env:     env,
	}, nil
}

// WriteValidationJson writes a ValidationResponse as JSON to the given http.ResponseWriter
func WriteValidationJSON(w http.ResponseWriter, status int, payload ValidationResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(payload)
}
