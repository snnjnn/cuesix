package control

import (
	"net/http"

	httpSwagger "github.com/swaggo/http-swagger/v2"
	_ "github.com/warpcomdev/cuesix/cmd/sixpack/docs"
	"github.com/warpcomdev/cuesix/internal/app"
	"github.com/warpcomdev/cuesix/internal/recorder"
	"github.com/warpcomdev/cuesix/internal/schema"
)

// @title Sixpack Schema Control API
// @version 1.0
// @description Control plane helpers for introspecting the APISIX schema and validating source fragments. The API exposes a few cached read endpoints; clients should revalidate changes by reusing the ETag/Last-Modified headers that the server always emits and honoring `Cache-Control: public, max-age=0, must-revalidate`.
// @BasePath /schema
func RegisterAPI(metricsMux *http.ServeMux, rec *recorder.Recorder, m *schema.Manager) *http.ServeMux {
	metricsMux.Handle("GET /schema/sources", http.StripPrefix("/schema/sources", rec.ListSources()))
	metricsMux.Handle("GET /schema/sources/", http.StripPrefix("/schema/sources", rec.GetSource()))
	metricsMux.Handle("GET /schema/virtualgw/{virtualgw}", rec.GetIndex())
	metricsMux.Handle("GET /schema/virtualgw/{virtualgw}/", http.StripPrefix("/schema/virtualgw", rec.GetConfig()))
	metricsMux.Handle("POST /schema/validate", http.StripPrefix("/schema/validate", m.ValidateBody()))
	metricsMux.Handle("GET /schema/validate/", http.StripPrefix("/schema/validate", rec.ValidateSource(m)))
	metricsMux.Handle("GET /schema/json", m.Schema())
	metricsMux.Handle("/schema/app/echo.html", app.EchoHandler())
	metricsMux.Handle("/schema/app/", http.StripPrefix("/schema/app", app.AppHandler()))
	metricsMux.Handle("/schema/openapi/", httpSwagger.Handler(
		httpSwagger.URL("/schema/openapi/doc.json"),
	))
	return metricsMux
}
