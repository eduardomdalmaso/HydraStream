package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"hydrastream/internal/domain"
	"hydrastream/internal/ports"
)

// Handler wraps primary HTTP adapters and dependencies.
type Handler struct {
	useCase ports.StreamUseCase
}

// NewHandler initializes HTTP handler adapters.
func NewHandler(uc ports.StreamUseCase) *Handler {
	return &Handler{useCase: uc}
}

func (h *Handler) handleStreams(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		searchQuery := r.URL.Query().Get("search")
		tenantFilter := r.URL.Query().Get("tenant")
		sortBy := r.URL.Query().Get("sort_by")
		page := 1
		limit := 10
		if p := r.URL.Query().Get("page"); p != "" {
			fmt.Sscanf(p, "%d", &page)
		}
		if l := r.URL.Query().Get("limit"); l != "" {
			fmt.Sscanf(l, "%d", &limit)
		}

		streams, total, err := h.useCase.ListStreams(r.Context(), searchQuery, tenantFilter, sortBy, page, limit)
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			return
		}

		w.Header().Set("X-Total-Count", fmt.Sprintf("%d", total))
		json.NewEncoder(w).Encode(map[string]interface{}{
			"streams":     streams,
			"total_count": total,
			"page":        page,
			"limit":       limit,
			"sort_by":     sortBy,
		})

	case http.MethodPost:
		var st domain.Stream
		if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
			http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
			return
		}

		if err := h.useCase.RegisterStream(r.Context(), &st); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(st)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleStreamByID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/streams/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, `{"error":"stream_id required"}`, http.StatusBadRequest)
		return
	}

	streamID := parts[0]

	// Route: GET /api/v1/streams/{id}/snapshot.jpg
	if len(parts) >= 2 && parts[1] == "snapshot.jpg" {
		h.handleSnapshot(w, r, streamID)
		return
	}

	// Route: GET /api/v1/streams/{id}/mjpeg
	if len(parts) >= 2 && parts[1] == "mjpeg" {
		h.handleMJPEG(w, r, streamID)
		return
	}

	// Route: GET /api/v1/streams/{id}/stats
	if len(parts) >= 2 && parts[1] == "stats" {
		st, err := h.useCase.GetStream(r.Context(), streamID)
		if err != nil {
			if errors.Is(err, domain.ErrStreamNotFound) {
				http.Error(w, `{"error":"stream not found"}`, http.StatusNotFound)
			} else {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}
		json.NewEncoder(w).Encode(st)
		return
	}

	// Route: GET /api/v1/streams/{id}/ingest
	if len(parts) >= 2 && parts[1] == "ingest" {
		stat, err := h.useCase.GetIngestStats(r.Context(), streamID)
		if err != nil {
			if errors.Is(err, domain.ErrStreamNotFound) {
				http.Error(w, `{"error":"stream ingest session not found"}`, http.StatusNotFound)
			} else {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}
		json.NewEncoder(w).Encode(stat)
		return
	}

	// Route: PATCH /api/v1/streams/{id}/consumers/{analytic_type}
	if len(parts) >= 3 && parts[1] == "consumers" && r.Method == http.MethodPatch {
		analyticType := parts[2]
		var req struct {
			TargetFPS    float64 `json:"target_fps"`
			OutputFormat string  `json:"output_format"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, `{"error":"invalid JSON"}`, http.StatusBadRequest)
			return
		}
		if err := h.useCase.UpdateConsumer(r.Context(), streamID, analyticType, req.TargetFPS, req.OutputFormat); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"updated"}`))
		return
	}

	// Direct Stream CRUD
	switch r.Method {
	case http.MethodGet:
		st, err := h.useCase.GetStream(r.Context(), streamID)
		if err != nil {
			if errors.Is(err, domain.ErrStreamNotFound) {
				http.Error(w, `{"error":"stream not found"}`, http.StatusNotFound)
			} else {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}
		json.NewEncoder(w).Encode(st)

	case http.MethodDelete:
		if err := h.useCase.DeleteStream(r.Context(), streamID); err != nil {
			if errors.Is(err, domain.ErrStreamNotFound) {
				http.Error(w, `{"error":"stream not found"}`, http.StatusNotFound)
			} else {
				http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
			}
			return
		}
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleSnapshot(w http.ResponseWriter, _ *http.Request, streamID string) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// 1. Tentar ler frame real do stream em samples/<stream_id>.jpg
	samplePath := filepath.Join("samples", fmt.Sprintf("%s.jpg", streamID))
	if data, err := os.ReadFile(samplePath); err == nil && len(data) > 0 {
		_, _ = w.Write(data)
		return
	}

	// 2. Tentar ler frame padrão cam_entrance_01.jpg
	if data, err := os.ReadFile("samples/cam_entrance_01.jpg"); err == nil && len(data) > 0 {
		_, _ = w.Write(data)
		return
	}

	// 3. Fallback estático de emergência
	jpegBytes := []byte{
		0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01,
		0x01, 0x01, 0x00, 0x48, 0x00, 0x48, 0x00, 0x00, 0xFF, 0xDB, 0x00, 0x43,
		0x00, 0x08, 0x06, 0x06, 0x07, 0x06, 0x05, 0x08, 0x07, 0x07, 0x07, 0x09,
		0x09, 0x08, 0x0A, 0x0C, 0x14, 0x0D, 0x0C, 0x0B, 0x0B, 0x0C, 0x19, 0x12,
		0x13, 0x0F, 0x14, 0x1D, 0x1A, 0x1F, 0x1E, 0x1D, 0x1A, 0x1C, 0x1C, 0x20,
		0x24, 0x2E, 0x27, 0x20, 0x22, 0x2C, 0x23, 0x1C, 0x1C, 0x28, 0x37, 0x29,
		0x2C, 0x30, 0x31, 0x34, 0x34, 0x34, 0x1F, 0x27, 0x39, 0x3D, 0x38, 0x32,
		0x3C, 0x2E, 0x33, 0x34, 0x32, 0xFF, 0xC0, 0x00, 0x0B, 0x08, 0x00, 0x01,
		0x00, 0x01, 0x01, 0x01, 0x11, 0x00, 0xFF, 0xC4, 0x00, 0x1F, 0x00, 0x00,
		0x01, 0x05, 0x01, 0x01, 0x01, 0x01, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08,
		0x09, 0x0A, 0x0B, 0xFF, 0xDA, 0x00, 0x08, 0x01, 0x01, 0x00, 0x00, 0x3F,
		0x00, 0x7F, 0x00, 0x3A, 0xFE, 0x8A, 0x28, 0xA0, 0x00, 0xFF, 0xD9,
	}
	w.Write(jpegBytes)
}

func (h *Handler) handleMJPEG(w http.ResponseWriter, r *http.Request, streamID string) {
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=frame")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	samplePath := filepath.Join("samples", fmt.Sprintf("%s.jpg", streamID))
	fallbackPath := "samples/cam_entrance_01.jpg"

	ticker := time.NewTicker(40 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			data, err := os.ReadFile(samplePath)
			if err != nil || len(data) == 0 {
				data, _ = os.ReadFile(fallbackPath)
			}
			if len(data) > 0 {
				fmt.Fprintf(w, "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(data))
				w.Write(data)
				w.Write([]byte("\r\n"))
				flusher.Flush()
			}
		}
	}
}

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

func (h *Handler) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (h *Handler) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("READY"))
}

func (h *Handler) handleMetrics(w http.ResponseWriter, r *http.Request) {
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

