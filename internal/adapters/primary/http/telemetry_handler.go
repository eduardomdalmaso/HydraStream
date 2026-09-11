package http

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"hydrastream/internal/domain"
	"hydrastream/internal/ports"
)

// TelemetryHandler manages health checks, hardware diagnostics and ring-buffer log queries.
type TelemetryHandler struct {
	useCase ports.StreamUseCase
}

// NewTelemetryHandler creates a new TelemetryHandler instance.
func NewTelemetryHandler(uc ports.StreamUseCase) *TelemetryHandler {
	return &TelemetryHandler{useCase: uc}
}

// HandleRoot serves a JSON discovery descriptor for the headless Data Plane engine.
func (th *TelemetryHandler) HandleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	resp := map[string]interface{}{
		"service":     "hydrastream-dataplane",
		"description": "HydraStream Zero-Copy Video Ingest, RFC 2326 TCP Demuxing & Media Relay Data Plane",
		"status":      "online",
		"version":     "1.0.0",
		"endpoints": map[string]string{
			"health":           "/api/v1/health",
			"healthz":          "/healthz",
			"readyz":           "/readyz",
			"telemetry":        "/api/v1/telemetry",
			"hardware":         "/api/v1/telemetry/hardware",
			"logs":             "/api/v1/telemetry/logs",
			"errors":           "/api/v1/telemetry/errors",
			"stats":            "/api/v1/telemetry/stats",
			"streams":          "/api/v1/streams",
			"onvif_discover":   "/api/v1/onvif/discover",
			"onvif_probe":      "/api/v1/onvif/probe",
			"cluster_topology": "/api/v1/cluster/topology",
			"swagger_ui":       "/swagger/",
			"swagger_json":     "/swagger/doc.json",
		},
	}
	json.NewEncoder(w).Encode(resp)
}

// HandleHealth handles GET /api/v1/health returning detailed engine health status.
func (th *TelemetryHandler) HandleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	health, err := th.useCase.GetHealth(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	if health.Status == "unhealthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else {
		w.WriteHeader(http.StatusOK)
	}

	json.NewEncoder(w).Encode(health)
}

// HandleHealthz handles lightweight liveness probes (HTTP 200 OK).
func (th *TelemetryHandler) HandleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"OK","service":"hydrastream-dataplane"}`))
}

// HandleReadyz handles readiness probes.
func (th *TelemetryHandler) HandleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"READY","service":"hydrastream-dataplane"}`))
}

// HandleUnifiedTelemetry handles GET /api/v1/telemetry returning combined system telemetry.
func (th *TelemetryHandler) HandleUnifiedTelemetry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	unified, err := th.useCase.GetUnifiedTelemetry(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(unified)
}

// HandleHardware handles GET /api/v1/telemetry/hardware returning CPU, RAM, GPU NVDEC and SHM stats.
func (th *TelemetryHandler) HandleHardware(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	hw, err := th.useCase.GetHardwareTelemetry(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(hw)
}

// HandleLogs handles GET /api/v1/telemetry/logs (query ring buffer) and POST (ingest log).
func (th *TelemetryHandler) HandleLogs(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		filter := domain.LogFilter{
			Level:     domain.LogLevel(q.Get("level")),
			Component: q.Get("component"),
		}

		if limitStr := q.Get("limit"); limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil {
				filter.Limit = l
			}
		}

		if sinceStr := q.Get("since"); sinceStr != "" {
			if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
				filter.Since = t
			}
		}

		result, err := th.useCase.GetLogs(r.Context(), filter)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(result)

	case http.MethodPost:
		var req struct {
			Level     string                 `json:"level"`
			Component string                 `json:"component"`
			Message   string                 `json:"message"`
			Details   map[string]interface{} `json:"details,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if req.Message == "" {
			http.Error(w, `{"error":"message is required"}`, http.StatusBadRequest)
			return
		}
		if req.Component == "" {
			req.Component = "custom"
		}
		lvl := domain.LogLevelInfo
		if req.Level != "" {
			lvl = domain.LogLevel(req.Level)
		}

		th.useCase.RecordLog(lvl, req.Component, req.Message, req.Details)
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(`{"status":"logged"}`))

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// HandleErrors handles GET /api/v1/telemetry/errors returning aggregated error anomalies.
func (th *TelemetryHandler) HandleErrors(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	summary, err := th.useCase.GetErrorSummary(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(summary)
}
