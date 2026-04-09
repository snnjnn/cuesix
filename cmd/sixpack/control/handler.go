package control

import (
	"encoding/json"
	"maps"
	"net/http"
)

// @Summary List virtual gateway readiness
// @Description <p>Returns a JSON object mapping each known virtual gateway to its current readiness state.</p>
// @Tags Ready
// @Produce application/json
// @Success 200 {object} map[string]bool "Virtual gateway readiness map"
// @Router /virtualgw [get]
func (reloader *ReadyReloader) GatewaysHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gateways := make(map[string]bool)
		reloader.WithLock(func() {
			maps.Copy(gateways, reloader.ready)
		})
		w.Header().Add("content-type", "application/json")
		w.WriteHeader(http.StatusOK)
		encoder := json.NewEncoder(w)
		encoder.Encode(gateways)
	})
}
