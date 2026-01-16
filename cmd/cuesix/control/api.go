package control

import (
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"
	_ "github.com/warpcomdev/cuesix/cmd/cuesix/docs"
	"github.com/warpcomdev/cuesix/internal/schema"
)

// @title Cuesix Schema Control API
// @version 1.0
// @description Control plane helpers for introspecting the APISIX schema and validating source fragments. The API exposes a few cached read endpoints; clients should revalidate changes by reusing the ETag/Last-Modified headers that the server always emits and honoring `Cache-Control: public, max-age=0, must-revalidate`.
// @BasePath /schema
func RegisterAPI(metricsMux *http.ServeMux, backend *schema.ValidationHandler) *http.ServeMux {
	metricsMux.Handle("GET /schema/sources", http.StripPrefix("/schema/sources", backend.ListSources()))
	metricsMux.Handle("GET /schema/sources/", http.StripPrefix("/schema/sources", backend.GetSource()))
	metricsMux.Handle("POST /schema/validate", http.StripPrefix("/schema/validate", backend.ValidateBody()))
	metricsMux.Handle("GET /schema/validate/", http.StripPrefix("/schema/validate", backend.ValidateSource()))
	metricsMux.Handle("GET /schema/json/", backend.Schema())
	metricsMux.Handle("/schema/app/", http.StripPrefix("/schema/app", schema.AppHandler()))
	metricsMux.Handle("/schema/openapi/", httpSwagger.Handler(
		httpSwagger.URL("/schema/openapi/doc.json"),
	))
	return metricsMux
}
