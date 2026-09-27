package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"

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

	if len(parts) >= 2 && (parts[1] == "snapshot" || parts[1] == "snapshot.jpg") {
		h.handleSnapshot(w, r, streamID)
		return
	}

	if len(parts) >= 2 && parts[1] == "mjpeg" {
		st, _ := h.useCase.GetStream(r.Context(), streamID)
		if st == nil {
			st = &domain.Stream{StreamID: streamID}
		}
		h.handleMJPEG(w, r, st)
		return
	}

	if len(parts) >= 2 && parts[1] == "stats" {
		st, err := checkOwnership()
		if err != nil {
			http.Error(w, `{"error":"stream not found"}`, http.StatusNotFound)
			return
		}
		json.NewEncoder(w).Encode(st)
		return
	}

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

	if len(parts) >= 2 && parts[1] == "whep" {
		var subParts []string
		if len(parts) > 2 {
			subParts = parts[2:]
		}
		h.whepHandler.HandleStreamWHEP(w, r, streamID, subParts)
		return
	}

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
