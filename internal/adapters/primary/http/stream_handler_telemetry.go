package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"hydrastream/internal/domain"
)

func (h *Handler) handleClusterTopology(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	streamID := r.URL.Query().Get("stream_id")
	topo, err := h.useCase.GetClusterTopology(r.Context(), streamID)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(topo)
}

func (h *Handler) handleControlPanelTelemetry(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	stats, err := h.useCase.GetControlPanelTelemetry(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(stats)
}

func (h *Handler) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	info, err := h.useCase.GetSystemInfo(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(info)
}

func (h *Handler) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (h *Handler) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("READY"))
}

func (h *Handler) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	fmt.Fprintf(w, "# HELP hydrastream_ingest_fps Input FPS per stream\n")
	fmt.Fprintf(w, "# TYPE hydrastream_ingest_fps gauge\n")
	fmt.Fprintf(w, "hydrastream_ingest_fps{stream_id=\"cam_entrance_01\"} 30.0\n")
}

func (h *Handler) handleChaosInject(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	var req domain.ChaosInjection
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
		return
	}

	res, err := h.useCase.InjectChaos(r.Context(), &req)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(res)
}

func (h *Handler) handleChaosReset(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"POST required"}`, http.StatusMethodNotAllowed)
		return
	}

	if err := h.useCase.ResetChaos(r.Context()); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"reset","message":"All chaos injection circuits disarmed and telemetry stabilized."}`))
}
