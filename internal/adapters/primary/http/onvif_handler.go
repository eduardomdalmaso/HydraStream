package http

import (
	"encoding/json"
	"net/http"
	"strings"

	"hydrastream/internal/domain"
	"hydrastream/internal/ports"
)

// ONVIFHandler exposes REST endpoints for ONVIF IP camera discovery and stream extraction.
type ONVIFHandler struct {
	useCase ports.StreamUseCase
}

// NewONVIFHandler creates an ONVIF HTTP handler.
func NewONVIFHandler(useCase ports.StreamUseCase) *ONVIFHandler {
	return &ONVIFHandler{useCase: useCase}
}

// HandleDiscover scans the local network via WS-Discovery.
func (h *ONVIFHandler) HandleDiscover(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	devices, err := h.useCase.DiscoverONVIFDevices(r.Context())
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	if devices == nil {
		devices = []domain.ONVIFDevice{}
	}
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"count":   len(devices),
		"devices": devices,
	})
}

// HandleProbe connects to a specific ONVIF camera by IP and optional credentials.
func (h *ONVIFHandler) HandleProbe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	var req domain.ONVIFProbeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	dev, err := h.useCase.ProbeONVIFDevice(r.Context(), req)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	_ = json.NewEncoder(w).Encode(dev)
}

// HandleImport registers an ONVIF camera directly into active HydraStream streams.
func (h *ONVIFHandler) HandleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	var body struct {
		StreamID  string  `json:"stream_id"`
		Name      string  `json:"name"`
		TenantID  string  `json:"tenant_id"`
		SourceURL string  `json:"source_url"`
		IngestFPS float64 `json:"ingest_fps"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid request body"})
		return
	}

	streamID := body.StreamID
	if streamID == "" {
		streamID = strings.ToLower(strings.ReplaceAll(body.Name, " ", "_"))
		if streamID == "" {
			streamID = "onvif_cam"
		}
	}

	fps := body.IngestFPS
	if fps <= 0 {
		fps = 30.0
	}

	tenant := body.TenantID
	if tenant == "" {
		tenant = "default"
	}

	stream := &domain.Stream{
		StreamID:  streamID,
		TenantID:  tenant,
		SourceURL: body.SourceURL,
		IngestFPS: fps,
		Status:    "online",
	}
	stream.SetDefaults()

	if err := h.useCase.RegisterStream(r.Context(), stream); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(stream)
}
