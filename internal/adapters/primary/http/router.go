package http

import "net/http"

// RegisterRoutes attaches REST API endpoints to a ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// API Endpoints
	mux.HandleFunc("/api/v1/streams", h.handleStreams)
	mux.HandleFunc("/api/v1/streams/", h.handleStreamByID)
	mux.HandleFunc("/api/v1/cluster/topology", h.handleClusterTopology)
	mux.HandleFunc("/api/v1/telemetry/stats", h.handleControlPanelTelemetry)
	mux.HandleFunc("/api/v1/info", h.handleSystemInfo)

	// Swagger Interactive API Docs
	mux.HandleFunc("/swagger/", ServeSwaggerUI)
	mux.HandleFunc("/swagger/doc.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(OpenAPI3Spec))
	})

	// Health & Observability
	mux.HandleFunc("/healthz", h.handleHealthz)
	mux.HandleFunc("/readyz", h.handleReadyz)
	mux.HandleFunc("/metrics", h.handleMetrics)
}
