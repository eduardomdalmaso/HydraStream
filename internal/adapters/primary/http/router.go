package http

import (
	"net/http"

	"hydrastream/internal/adapters/primary/http/middleware"
)

// RegisterRoutes attaches REST API endpoints to a ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Telemetry & Health Handler
	telH := NewTelemetryHandler(h.useCase)
	onvifH := NewONVIFHandler(h.useCase)

	// Helper to protect endpoints with Auth and optional RBAC middlewares
	authEndpoint := func(pattern string, fn http.HandlerFunc, roles ...string) {
		var handler http.Handler = http.HandlerFunc(fn)
		if len(roles) > 0 {
			handler = middleware.RequireRole(roles...)(handler)
		}
		handler = middleware.AuthMiddleware(handler)
		mux.Handle(pattern, handler)
	}

	// 1. Public Endpoints (Root Discovery, Health, Metrics, Swagger)
	mux.HandleFunc("/", telH.HandleRoot)
	mux.HandleFunc("/healthz", telH.HandleHealthz)
	mux.HandleFunc("/readyz", telH.HandleReadyz)
	mux.HandleFunc("/metrics", h.handleMetrics)
	mux.HandleFunc("/swagger/", ServeSwaggerUI)
	mux.HandleFunc("/swagger/doc.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(OpenAPI3Spec))
	})

	// 2. Protected Stream Management Endpoints (Auth Required)
	authEndpoint("/api/v1/streams", h.handleStreams)
	authEndpoint("/api/v1/streams/", h.handleStreamByID)
	authEndpoint("/api/v1/streams/probe", h.HandleProbeStream)
	authEndpoint("/api/v1/streams/samples", h.HandleListSamples)
	authEndpoint("/api/v1/cluster/topology", h.handleClusterTopology)
	authEndpoint("/api/v1/info", h.handleSystemInfo)

	// 3. Chaos Injection Endpoints (Admin Required)
	authEndpoint("/api/v1/chaos/inject", h.handleChaosInject, "admin", "superadmin")
	authEndpoint("/api/v1/chaos/reset", h.handleChaosReset, "admin", "superadmin")

	// 4. Protected Telemetry, Hardware & Logs Endpoints
	authEndpoint("/api/v1/health", telH.HandleHealth)
	authEndpoint("/api/v1/telemetry", telH.HandleUnifiedTelemetry)
	authEndpoint("/api/v1/telemetry/hardware", telH.HandleHardware)
	authEndpoint("/api/v1/telemetry/logs", telH.HandleLogs)
	authEndpoint("/api/v1/telemetry/errors", telH.HandleErrors)
	authEndpoint("/api/v1/telemetry/stats", h.handleControlPanelTelemetry)

	// 5. Protected ONVIF Discovery & Ingestion
	authEndpoint("/api/v1/onvif/discover", onvifH.HandleDiscover)
	authEndpoint("/api/v1/onvif/probe", onvifH.HandleProbe)
	authEndpoint("/api/v1/onvif/import", onvifH.HandleImport, "admin", "operator", "superadmin")

	// 6. Direct Recording Fragments Ingestion
	authEndpoint("/api/v1/recordings/fragments", h.fragHandler.HandleGeneralRecordingsIngest, "admin", "operator", "superadmin", "system")

	// 7. WebRTC HTTP Egress Protocol (WHEP) Egress Gateway
	mux.HandleFunc("/whep", h.whepHandler.HandleRootWHEP)
	mux.HandleFunc("/whep/", h.whepHandler.HandleRootWHEP)
}


// WithCORS wraps an http.Handler with universal Cross-Origin Resource Sharing headers.
func WithCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS, HEAD")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Requested-With, Range, Accept, Origin")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Length, Content-Range, Content-Type, X-Total-Count")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
