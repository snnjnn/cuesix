package compiler

import (
	"net/http"
)

// PathVirtualGateway is a helper for handlers with {virtualgw} path parameter
func PathVirtualGateway(w http.ResponseWriter, r *http.Request) (string, bool) {
	if r == nil {
		http.Error(w, "request is nil", http.StatusBadRequest)
		return "", false
	}
	virtualgw := r.PathValue("virtualgw")
	if virtualgw == "" {
		http.Error(w, "missing virtualgw path parameter", http.StatusBadRequest)
		return "", false
	}
	return virtualgw, true
}
