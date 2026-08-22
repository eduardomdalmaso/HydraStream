package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"hydrastream/internal/model"
	"hydrastream/internal/store"
)

// Handler wraps the API handlers and dependencies.
type Handler struct {
	Store *store.StreamStore
}

// NewHandler initializes API handler dependencies.
func NewHandler(st *store.StreamStore) *Handler {
	return &Handler{Store: st}
}

// RegisterRoutes attaches REST API endpoints to a ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// API Endpoints
	mux.HandleFunc("/api/v1/streams", h.handleStreams)
	mux.HandleFunc("/api/v1/streams/", h.handleStreamByID)
	mux.HandleFunc("/api/v1/cluster/topology", h.handleClusterTopology)
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

func (h *Handler) handleStreams(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case http.MethodGet:
		searchQuery := r.URL.Query().Get("search")
		tenantFilter := r.URL.Query().Get("tenant")
		page := 1
		limit := 10
		if p := r.URL.Query().Get("page"); p != "" {
			fmt.Sscanf(p, "%d", &page)
		}
		if l := r.URL.Query().Get("limit"); l != "" {
			fmt.Sscanf(l, "%d", &limit)
		}

		streams, total := h.Store.ListStreamsFiltered(searchQuery, tenantFilter, page, limit)
		w.Header().Set("X-Total-Count", fmt.Sprintf("%d", total))
		json.NewEncoder(w).Encode(map[string]interface{}{
			"streams":     streams,
			"total_count": total,
			"page":        page,
			"limit":       limit,
		})

	case http.MethodPost:
		var st model.Stream
		if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
			http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
			return
		}
		if st.StreamID == "" {
			http.Error(w, `{"error":"stream_id is required"}`, http.StatusBadRequest)
			return
		}
		h.Store.AddOrUpdateStream(&st)
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
		st, ok := h.Store.GetStream(streamID)
		if !ok {
			http.Error(w, `{"error":"stream not found"}`, http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(st)
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
		if err := h.Store.UpdateConsumerFPS(streamID, analyticType, req.TargetFPS, req.OutputFormat); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"updated"}`))
		return
	}

	// Direct Stream CRUD
	st, ok := h.Store.GetStream(streamID)
	if !ok {
		http.Error(w, `{"error":"stream not found"}`, http.StatusNotFound)
		return
	}

	switch r.Method {
	case http.MethodGet:
		json.NewEncoder(w).Encode(st)
	case http.MethodDelete:
		h.Store.DeleteStream(streamID)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleSnapshot(w http.ResponseWriter, r *http.Request, streamID string) {
	w.Header().Set("Content-Type", "image/jpeg")
	// Synthetic 1x1 JPEG placeholder buffer for mock server
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

	for i := 0; i < 5; i++ {
		fmt.Fprintf(w, "--frame\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(jpegBytes))
		w.Write(jpegBytes)
		w.Write([]byte("\r\n"))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func (h *Handler) handleClusterTopology(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	topo := model.ClusterTopology{
		StreamID: "cam_entrance_01",
		IngestionNode: model.TopologyNode{
			NodeName:      "k8s-gpu-node-02",
			NodeIP:        "10.0.1.45",
			CPUArch:       "AMD EPYC 7763 64-Core",
			GPUHardware:   "NVIDIA A100-SXM4-80GB",
			DecoderEngine: "NVDEC (GPU 0)",
		},
		ConsumerRoute: []model.ConsumerRouting{
			{
				Analytic:       "yolo_detection",
				TargetNode:     "k8s-gpu-node-02",
				SameNode:       true,
				TransportUsed:  "CUDA_IPC (Zero-Copy VRAM Direct)",
				TargetHardware: "NVIDIA A100-SXM4-80GB",
			},
		},
	}
	json.NewEncoder(w).Encode(topo)
}

func (h *Handler) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	info := model.SystemInfo{
		AppName:       "HydraStream Engine",
		Version:       "1.0.0",
		UptimeSeconds: h.Store.UptimeSeconds(),
		EngineMode:    "nvidia_nvdec",
		GPUDetected:   true,
		GPUModel:      "NVIDIA RTX 4090",
		Features: map[string]bool{
			"posix_shm":  true,
			"cuda_ipc":   true,
			"triton_grpc": true,
		},
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
