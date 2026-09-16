package http

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hydrastream/internal/adapters/primary/http/middleware"
	"hydrastream/internal/domain"
	"hydrastream/internal/ports"
)

// FragmentHandler exposes HTTP endpoints for recording fragments ingestion and streaming.
type FragmentHandler struct {
	useCase ports.StreamUseCase
}

// NewFragmentHandler creates a new FragmentHandler instance.
func NewFragmentHandler(uc ports.StreamUseCase) *FragmentHandler {
	return &FragmentHandler{useCase: uc}
}

func (h *FragmentHandler) HandleStreamFragments(w http.ResponseWriter, r *http.Request, streamID string, subParts []string) {
	callerTenant := middleware.GetTenantID(r.Context())
	callerRole := middleware.GetUserRole(r.Context())

	// If a specific fragment ID is provided: /api/v1/streams/{id}/recordings/fragments/{fragment_id}
	if len(subParts) >= 1 && subParts[0] != "" {
		fragmentID := filepath.Base(subParts[0])
		h.handleFragmentByID(w, r, streamID, fragmentID, callerTenant, callerRole)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleListFragments(w, r, streamID)
	case http.MethodPost:
		h.handleUploadFragment(w, r, streamID, callerTenant, callerRole)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

// HandleGeneralRecordingsIngest routes direct /api/v1/recordings/fragments requests
func (h *FragmentHandler) HandleGeneralRecordingsIngest(w http.ResponseWriter, r *http.Request) {
	callerTenant := middleware.GetTenantID(r.Context())
	callerRole := middleware.GetUserRole(r.Context())

	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed, use POST to upload fragments"}`, http.StatusMethodNotAllowed)
		return
	}

	streamID := r.URL.Query().Get("stream_id")
	if streamID == "" {
		streamID = r.URL.Query().Get("camera_id")
	}

	h.handleUploadFragment(w, r, streamID, callerTenant, callerRole)
}


func (h *FragmentHandler) handleUploadFragment(w http.ResponseWriter, r *http.Request, streamID string, callerTenant, callerRole string) {
	if callerRole != "admin" && callerRole != "operator" && callerRole != "superadmin" && callerRole != "system" {
		http.Error(w, `{"error":"forbidden: insufficient role permissions"}`, http.StatusForbidden)
		return
	}

	var data []byte
	var frag domain.RecordingFragment

	// Check if Multipart Form
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/form-data") {
		// Max 100MB per fragment
		if err := r.ParseMultipartForm(100 << 20); err != nil {
			http.Error(w, fmt.Sprintf(`{"error":"failed to parse multipart form: %v"}`, err), http.StatusBadRequest)
			return
		}

		if streamID == "" {
			streamID = r.FormValue("stream_id")
			if streamID == "" {
				streamID = r.FormValue("camera_id")
			}
		}

		frag.ID = r.FormValue("id")
		frag.StreamID = streamID
		frag.TenantID = r.FormValue("tenant_id")
		frag.RecordingMode = r.FormValue("recording_mode")
		frag.Codec = r.FormValue("codec")
		frag.ContentType = r.FormValue("content_type")

		if durStr := r.FormValue("duration_seconds"); durStr != "" {
			if d, err := strconv.ParseFloat(durStr, 64); err == nil {
				frag.DurationSeconds = d
			}
		}

		if stStr := r.FormValue("start_time"); stStr != "" {
			if t, err := time.Parse(time.RFC3339, stStr); err == nil {
				frag.StartTime = t
			}
		}
		if etStr := r.FormValue("end_time"); etStr != "" {
			if t, err := time.Parse(time.RFC3339, etStr); err == nil {
				frag.EndTime = t
			}
		}

		file, header, err := r.FormFile("file")
		if err == nil {
			defer file.Close()
			data, _ = io.ReadAll(file)
			if frag.ContentType == "" && header.Header.Get("Content-Type") != "" {
				frag.ContentType = header.Header.Get("Content-Type")
			}
		}
	} else {
		// Raw binary or JSON
		if streamID == "" {
			streamID = r.URL.Query().Get("stream_id")
			if streamID == "" {
				streamID = r.URL.Query().Get("camera_id")
			}
		}

		frag.StreamID = streamID
		frag.TenantID = r.URL.Query().Get("tenant_id")
		frag.RecordingMode = r.URL.Query().Get("recording_mode")
		frag.Codec = r.URL.Query().Get("codec")
		frag.ContentType = r.Header.Get("Content-Type")

		if durStr := r.URL.Query().Get("duration_seconds"); durStr != "" {
			if d, err := strconv.ParseFloat(durStr, 64); err == nil {
				frag.DurationSeconds = d
			}
		}

		rawBody, err := io.ReadAll(io.LimitReader(r.Body, 100<<20))
		if err == nil && len(rawBody) > 0 {
			// Check if JSON payload with base64 data

			if strings.Contains(frag.ContentType, "application/json") {
				var jsonReq struct {
					domain.RecordingFragment
					Data string `json:"data"`
				}
				if err := json.Unmarshal(rawBody, &jsonReq); err == nil {
					frag = jsonReq.RecordingFragment
					if jsonReq.StreamID != "" {
						frag.StreamID = jsonReq.StreamID
					}
					data = []byte(jsonReq.Data)
				}
			} else {
				data = rawBody
			}
		}
	}

	if !isValidStreamID(frag.StreamID) {
		http.Error(w, `{"error":"invalid stream_id"}`, http.StatusBadRequest)
		return
	}

	if callerRole != "superadmin" && callerRole != "system" {
		frag.TenantID = callerTenant
	} else if frag.TenantID == "" {
		frag.TenantID = callerTenant
	}

	if err := frag.Validate(); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		return
	}

	if err := h.useCase.SaveRecordingFragment(r.Context(), &frag, data); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to save fragment: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "created",
		"fragment": frag,
	})
}

func (h *FragmentHandler) handleListFragments(w http.ResponseWriter, r *http.Request, streamID string) {
	var start, end time.Time
	if s := r.URL.Query().Get("start"); s != "" {
		start, _ = time.Parse(time.RFC3339, s)
	}
	if e := r.URL.Query().Get("end"); e != "" {
		end, _ = time.Parse(time.RFC3339, e)
	}
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	fragments, err := h.useCase.ListRecordingFragments(r.Context(), streamID, start, end, limit)
	if err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"stream_id": streamID,
		"fragments": fragments,
		"count":     len(fragments),
	})
}

func (h *FragmentHandler) handleFragmentByID(w http.ResponseWriter, r *http.Request, streamID, fragmentID string, callerTenant, callerRole string) {
	frag, data, err := h.useCase.GetRecordingFragment(r.Context(), streamID, fragmentID)
	if err != nil {
		if errors.Is(err, domain.ErrFragmentNotFound) {
			http.Error(w, `{"error":"fragment not found"}`, http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		}
		return
	}

	if callerRole != "superadmin" && callerRole != "system" && frag.TenantID != "" && frag.TenantID != callerTenant {
		http.Error(w, `{"error":"fragment not found"}`, http.StatusNotFound)
		return
	}

	if r.Method == http.MethodDelete {
		if callerRole != "admin" && callerRole != "superadmin" {
			http.Error(w, `{"error":"forbidden: insufficient role permissions"}`, http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}


	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	contentType := frag.ContentType
	if contentType == "" {
		contentType = "video/mp4"
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("X-Fragment-ID", frag.ID)
	w.Header().Set("X-Stream-ID", frag.StreamID)
	w.Header().Set("X-Duration-Seconds", fmt.Sprintf("%.2f", frag.DurationSeconds))

	// If file exists on disk, use http.ServeFile for kernel Zero-Copy and Range headers
	if frag.StoragePath != "" {
		if fi, statErr := os.Stat(frag.StoragePath); statErr == nil && !fi.IsDir() {
			file, openErr := os.Open(frag.StoragePath)
			if openErr == nil {
				defer file.Close()
				http.ServeContent(w, r, filepath.Base(frag.StoragePath), fi.ModTime(), file)
				return
			}
		}
	}

	// Serve in-memory bytes
	if len(data) > 0 {
		http.ServeContent(w, r, fmt.Sprintf("%s.mp4", frag.ID), frag.CreatedAt, bytes.NewReader(data))
		return
	}

	http.Error(w, `{"error":"fragment data unavailable"}`, http.StatusNotFound)
}
