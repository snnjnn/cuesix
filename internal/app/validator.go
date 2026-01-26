package app

import (
	"bytes"
	"errors"
	"io"
	"net/http"

	"github.com/warpcomdev/sixpack/internal/schema"
)

func decodeValidationRequest(w http.ResponseWriter, r *http.Request) (schema.ValidationProbe, error) {
	query := r.URL.Query()
	env := make(map[string]string)
	for k := range query {
		env[k] = r.URL.Query().Get(k)
	}
	r.Body = http.MaxBytesReader(w, r.Body, schema.SchemaMaxBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return schema.ValidationProbe{}, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return schema.ValidationProbe{}, errors.New("empty payload")
	}
	return schema.ValidationProbe{
		Payload: body,
		IsYaml:  false,
		Env:     env,
	}, nil
}
