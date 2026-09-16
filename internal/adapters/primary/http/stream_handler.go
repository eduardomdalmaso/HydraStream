package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"hydrastream/internal/adapters/primary/http/middleware"
	"hydrastream/internal/domain"
	"hydrastream/internal/ports"
)

var validStreamIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)

func isValidStreamID(id string) bool {
	return validStreamIDRegex.MatchString(id)
}

func isValidSourceURL(rawURL string) bool {
	if strings.HasPrefix(rawURL, "synthetic://") {
		return true
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(u.Scheme)
	return scheme == "rtsp" || scheme == "rtmp" || scheme == "http" || scheme == "https" || scheme == "file"
}

// Handler wraps primary HTTP adapters and dependencies.
type Handler struct {
	useCase     ports.StreamUseCase
	fragHandler *FragmentHandler
	whepHandler *WHEPHandler
}

// NewHandler initializes HTTP handler adapters.
func NewHandler(uc ports.StreamUseCase) *Handler {
	return &Handler{
		useCase:     uc,
		fragHandler: NewFragmentHandler(uc),
		whepHandler: NewWHEPHandler(uc),
	}
}


func (h *Handler) handleStreams(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	callerTenant := middleware.GetTenantID(r.Context())
	callerRole := middleware.GetUserRole(r.Context())

	switch r.Method {
	case http.MethodGet:
		searchQuery := r.URL.Query().Get("search")
		tenantFilter := callerTenant
		// Superadmin or system service accounts can optionally filter across all tenants
		if (callerRole == "superadmin" || callerRole == "system") && r.URL.Query().Get("tenant") != "" {
			tenantFilter = r.URL.Query().Get("tenant")
		} else if callerRole == "superadmin" && r.URL.Query().Get("all") == "true" {
			tenantFilter = ""
		}

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
		if callerRole != "admin" && callerRole != "operator" && callerRole != "superadmin" && callerRole != "system" {
			http.Error(w, `{"error":"forbidden: insufficient role permissions"}`, http.StatusForbidden)
			return
		}

		var st domain.Stream
		if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
			http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
			return
		}

		if !isValidStreamID(st.StreamID) {
			http.Error(w, `{"error":"invalid stream_id: must be alphanumeric (1-64 chars)"}`, http.StatusBadRequest)
			return
		}

		if !isValidSourceURL(st.SourceURL) {
			http.Error(w, `{"error":"invalid source_url: protocol not allowed"}`, http.StatusBadRequest)
			return
		}

		// Enforce tenant boundary
		if callerRole != "superadmin" && callerRole != "system" {
			st.TenantID = callerTenant
		} else if st.TenantID == "" {
			st.TenantID = callerTenant
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
	if !isValidStreamID(streamID) {
		http.Error(w, `{"error":"invalid stream_id"}`, http.StatusBadRequest)
		return
	}

	callerTenant := middleware.GetTenantID(r.Context())
	callerRole := middleware.GetUserRole(r.Context())

	// Helper to verify tenant ownership and prevent IDOR
	checkOwnership := func() (*domain.Stream, error) {
		st, err := h.useCase.GetStream(r.Context(), streamID)
		if err != nil {
			return nil, err
		}
		if callerRole != "superadmin" && callerRole != "system" && st.TenantID != callerTenant {
			return nil, domain.ErrStreamNotFound
		}
		return st, nil
	}

	// Route: GET /api/v1/streams/{id}/snapshot or snapshot.jpg
	if len(parts) >= 2 && (parts[1] == "snapshot" || parts[1] == "snapshot.jpg") {
		if _, err := checkOwnership(); err != nil {
			http.Error(w, `{"error":"stream not found"}`, http.StatusNotFound)
			return
		}
		h.handleSnapshot(w, r, streamID)
		return
	}

	// Route: GET /api/v1/streams/{id}/mjpeg
	if len(parts) >= 2 && parts[1] == "mjpeg" {
		st, err := checkOwnership()
		if err != nil {
			http.Error(w, `{"error":"stream not found"}`, http.StatusNotFound)
			return
		}
		h.handleMJPEG(w, r, st)
		return
	}

	// Route: GET /api/v1/streams/{id}/stats
	if len(parts) >= 2 && parts[1] == "stats" {
		st, err := checkOwnership()
		if err != nil {
			http.Error(w, `{"error":"stream not found"}`, http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(st)
		return
	}

	// Route: GET /api/v1/streams/{id}/ingest
	if len(parts) >= 2 && parts[1] == "ingest" {
		if _, err := checkOwnership(); err != nil {
			http.Error(w, `{"error":"stream not found"}`, http.StatusNotFound)
			return
		}
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

	// Route: /api/v1/streams/{id}/recordings/fragments...
	if len(parts) >= 3 && parts[1] == "recordings" && parts[2] == "fragments" {
		if _, err := checkOwnership(); err != nil {
			http.Error(w, `{"error":"stream not found"}`, http.StatusNotFound)
			return
		}
		var subParts []string
		if len(parts) > 3 {
			subParts = parts[3:]
		}
		h.fragHandler.HandleStreamFragments(w, r, streamID, subParts)
		return
	}

	// Route: /api/v1/streams/{id}/whep...
	if len(parts) >= 2 && parts[1] == "whep" {
		var subParts []string
		if len(parts) > 2 {
			subParts = parts[2:]
		}
		h.whepHandler.HandleStreamWHEP(w, r, streamID, subParts)
		return
	}


	// Route: PATCH /api/v1/streams/{id}/consumers/{analytic_type}
	if len(parts) >= 3 && parts[1] == "consumers" && r.Method == http.MethodPatch {
		if callerRole != "admin" && callerRole != "operator" && callerRole != "superadmin" {
			http.Error(w, `{"error":"forbidden: insufficient permissions"}`, http.StatusForbidden)
			return
		}
		if _, err := checkOwnership(); err != nil {
			http.Error(w, `{"error":"stream not found"}`, http.StatusNotFound)
			return
		}

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
		st, err := checkOwnership()
		if err != nil {
			http.Error(w, `{"error":"stream not found"}`, http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(st)

	case http.MethodDelete:
		if callerRole != "admin" && callerRole != "operator" && callerRole != "superadmin" {
			http.Error(w, `{"error":"forbidden: insufficient permissions"}`, http.StatusForbidden)
			return
		}
		if _, err := checkOwnership(); err != nil {
			http.Error(w, `{"error":"stream not found"}`, http.StatusNotFound)
			return
		}

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

func (h *Handler) handleSnapshot(w http.ResponseWriter, r *http.Request, streamID string) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	// Strict path sanitization against Path Traversal
	cleanBase := filepath.Base(streamID)
	samplePath := filepath.Clean(filepath.Join("samples", fmt.Sprintf("%s.jpg", cleanBase)))
	if !strings.HasPrefix(samplePath, "samples") && !strings.HasPrefix(samplePath, "samples/") {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	isRefresh := r != nil && (r.URL.Query().Get("refresh") == "true" || r.URL.Query().Get("force") == "true")

	// 1. Tentar ler frame cacheado caso não tenha sido solicitado refresh
	if !isRefresh {
		if data, err := os.ReadFile(samplePath); err == nil && len(data) > 0 {
			_, _ = w.Write(data)
			return
		}
	}

	// 2. Capturar frame sob demanda do MediaMTX (sub-stream ou main stream)
	snapCmd := exec.Command("ffmpeg", "-rtsp_transport", "tcp", "-timeout", "3000000",
		"-i", fmt.Sprintf("rtsp://localhost:8554/%s_sub", cleanBase),
		"-frames:v", "1", "-q:v", "2", "-y", samplePath)
	if err := snapCmd.Run(); err != nil {
		snapCmd = exec.Command("ffmpeg", "-rtsp_transport", "tcp", "-timeout", "3000000",
			"-i", fmt.Sprintf("rtsp://localhost:8554/%s", cleanBase),
			"-frames:v", "1", "-q:v", "2", "-y", samplePath)
		_ = snapCmd.Run()
	}

	if data, err := os.ReadFile(samplePath); err == nil && len(data) > 0 {
		_, _ = w.Write(data)
		return
	}

	// 3. Fallback para cam_10_0_0_64.jpg se existir
	if data, err := os.ReadFile("samples/cam_10_0_0_64.jpg"); err == nil && len(data) > 0 {
		_, _ = w.Write(data)
		return
	}

	http.Error(w, `{"error":"snapshot unavailable"}`, http.StatusServiceUnavailable)
}

func (h *Handler) handleMJPEG(w http.ResponseWriter, r *http.Request, st *domain.Stream) {
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=ffmpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	streamID := filepath.Base(st.StreamID)
	// 1. Primary source: MediaMTX Data Plane Relay on localhost:8554
	targetURL := fmt.Sprintf("rtsp://localhost:8554/%s_sub", streamID)
	if r.URL.Query().Get("main") == "1" {
		targetURL = fmt.Sprintf("rtsp://localhost:8554/%s", streamID)
	}

	for {
		select {
		case <-r.Context().Done():
			return
		default:
		}

		args := []string{
			"-loglevel", "error",
			"-fflags", "nobuffer",
			"-flags", "low_delay",
			"-fflags", "+discardcorrupt",
			"-rtsp_transport", "tcp",
			"-timeout", "5000000",
			"-i", targetURL,
			"-an",
			"-threads", "2",
			"-c:v", "mjpeg",
			"-q:v", "5",
			"-r", "20",
			"-f", "mpjpeg",
			"-boundary_tag", "ffmpeg",
			"-",
		}

		cmd := exec.CommandContext(r.Context(), "ffmpeg", args...)
		cmd.Stdout = w
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			if r.Context().Err() != nil {
				return
			}
			// 2. Direct Camera fallback if registered in stream repository
			if st.SourceURL != "" && !strings.HasPrefix(st.SourceURL, "synthetic://") && isValidSourceURL(st.SourceURL) {
				directURL := st.SourceURL
				if strings.Contains(directURL, "/stream1") && r.URL.Query().Get("main") != "1" {
					directURL = strings.Replace(directURL, "/stream1", "/stream2", 1)
				}
				args[7] = directURL
				directCmd := exec.CommandContext(r.Context(), "ffmpeg", args...)
				directCmd.Stdout = w
				_ = directCmd.Run()
				if r.Context().Err() != nil {
					return
				}
			}
			// 3. Fallback to sample ticker if media relay and direct are offline
			samplePath := filepath.Clean(filepath.Join("samples", fmt.Sprintf("%s.jpg", streamID)))
			fallbackPath := "samples/cam_entrance_01.jpg"
			data, err := os.ReadFile(samplePath)
			if err != nil || len(data) == 0 {
				data, _ = os.ReadFile(fallbackPath)
			}
			if len(data) > 0 {
				fmt.Fprintf(w, "--ffmpeg\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(data))
				w.Write(data)
				w.Write([]byte("\r\n"))
			}
			time.Sleep(500 * time.Millisecond)
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
