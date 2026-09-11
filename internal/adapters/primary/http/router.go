package http

import "net/http"

// RegisterRoutes attaches REST API endpoints to a ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Telemetry & Health Handler
	telH := NewTelemetryHandler(h.useCase)

	// Root API Info & Discovery
	mux.HandleFunc("/", telH.HandleRoot)

	// Stream Management Endpoints
	mux.HandleFunc("/api/v1/streams", h.handleStreams)
	mux.HandleFunc("/api/v1/streams/", h.handleStreamByID)
	mux.HandleFunc("/api/v1/cluster/topology", h.handleClusterTopology)
	mux.HandleFunc("/api/v1/info", h.handleSystemInfo)
	mux.HandleFunc("/api/v1/chaos/inject", h.handleChaosInject)
	mux.HandleFunc("/api/v1/chaos/reset", h.handleChaosReset)

	// Telemetry, Health, Hardware, Logs & Error Endpoints
	mux.HandleFunc("/api/v1/health", telH.HandleHealth)
	mux.HandleFunc("/api/v1/telemetry", telH.HandleUnifiedTelemetry)
	mux.HandleFunc("/api/v1/telemetry/hardware", telH.HandleHardware)
	mux.HandleFunc("/api/v1/telemetry/logs", telH.HandleLogs)
	mux.HandleFunc("/api/v1/telemetry/errors", telH.HandleErrors)
	mux.HandleFunc("/api/v1/telemetry/stats", h.handleControlPanelTelemetry)

	// ONVIF Camera Discovery & RTSP Stream Extraction
	onvifH := NewONVIFHandler(h.useCase)
	mux.HandleFunc("/api/v1/onvif/discover", onvifH.HandleDiscover)
	mux.HandleFunc("/api/v1/onvif/probe", onvifH.HandleProbe)
	mux.HandleFunc("/api/v1/onvif/import", onvifH.HandleImport)

	// Swagger Interactive API Docs
	mux.HandleFunc("/swagger/", ServeSwaggerUI)
	mux.HandleFunc("/swagger/doc.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(OpenAPI3Spec))
	})

	// Health & Observability standard probes
	mux.HandleFunc("/healthz", telH.HandleHealthz)
	mux.HandleFunc("/readyz", telH.HandleReadyz)
	mux.HandleFunc("/metrics", h.handleMetrics)
}
