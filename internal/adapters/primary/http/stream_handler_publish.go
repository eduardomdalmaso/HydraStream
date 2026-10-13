package http

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"hydrastream/internal/adapters/primary/http/middleware"
	"hydrastream/internal/domain"
)

// StreamPublishRequest payload for requesting an RTMP push ingestion endpoint.
type StreamPublishRequest struct {
	StreamID string `json:"stream_id"`
	Name     string `json:"name,omitempty"`
}

// StreamPublishResponse provides generated ingest and egress endpoints.
type StreamPublishResponse struct {
	StreamID   string `json:"stream_id"`
	Status     string `json:"status"`
	PublishURL string `json:"publish_url"`
	WSPURL     string `json:"wsp_url"`
	SHMPath    string `json:"shm_path"`
	FanoutMode string `json:"fanout_mode"`
	CreatedAt  string `json:"created_at"`
}

func getHostForPublish(r *http.Request) string {
	if envHost := os.Getenv("PUBLIC_HOST"); envHost != "" {
		return envHost
	}
	host := r.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "" || host == "localhost" || host == "127.0.0.1" {
		return "127.0.0.1"
	}
	return host
}

// HandlePublishStream handles generation of RTMP push URLs and registers pending stream.
func (h *Handler) HandlePublishStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	callerRole := middleware.GetUserRole(r.Context())
	callerTenant := middleware.GetTenantID(r.Context())

	if callerRole != "admin" && callerRole != "operator" && callerRole != "superadmin" && callerRole != "system" {
		http.Error(w, `{"error":"forbidden: insufficient role permissions"}`, http.StatusForbidden)
		return
	}

	var req StreamPublishRequest
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	streamID := strings.TrimSpace(req.StreamID)
	if streamID == "" {
		streamID = fmt.Sprintf("push_%d", time.Now().Unix())
	}

	if !isValidStreamID(streamID) {
		http.Error(w, `{"error":"invalid stream_id: must be alphanumeric (1-64 chars)"}`, http.StatusBadRequest)
		return
	}

	host := getHostForPublish(r)
	rtmpPort := os.Getenv("RTMP_PORT")
	if rtmpPort == "" {
		rtmpPort = "1936"
	}
	wspPort := os.Getenv("WSP_PORT")
	if wspPort == "" {
		wspPort = "8086"
	}

	publishURL := fmt.Sprintf("rtmp://%s:%s/live/%s", host, rtmpPort, streamID)
	wspURL := fmt.Sprintf("ws://%s:%s/live/%s", host, wspPort, streamID)
	shmPath := fmt.Sprintf("/dev/shm/hydra_%s", streamID)

	tenantID := callerTenant
	if tenantID == "" {
		tenantID = "default"
	}

	// Register stream in pending state
	st := &domain.Stream{
		TenantID:       tenantID,
		StreamID:       streamID,
		SourceURL:      publishURL,
		DecodingEngine: "native_rtmp_wsp",
		Status:         "waiting_stream",
		Resolution:     "1920x1080",
		Codec:          "h264",
		IngestFPS:      30.0,
		CreatedAt:      time.Now(),
	}
	st.SetDefaults()

	_ = h.useCase.RegisterStream(r.Context(), st)

	resp := StreamPublishResponse{
		StreamID:   streamID,
		Status:     "waiting_stream",
		PublishURL: publishURL,
		WSPURL:     wspURL,
		SHMPath:    shmPath,
		FanoutMode: "zero_copy_1_to_n",
		CreatedAt:  st.CreatedAt.UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(resp)
}
