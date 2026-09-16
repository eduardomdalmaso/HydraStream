package http

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"hydrastream/internal/domain"
	"hydrastream/internal/ports"
)

// WHEPHandler exposes HTTP endpoints for WebRTC HTTP Egress Protocol (WHEP).
type WHEPHandler struct {
	useCase ports.StreamUseCase
}

// NewWHEPHandler creates a new WHEPHandler instance.
func NewWHEPHandler(uc ports.StreamUseCase) *WHEPHandler {
	return &WHEPHandler{useCase: uc}
}

// HandleRootWHEP handles requests to /whep and /whep/
func (h *WHEPHandler) HandleRootWHEP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/whep")
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")

	streamID := ""
	if len(parts) > 0 && parts[0] != "" {
		streamID = filepath.Base(parts[0])
	}
	if streamID == "" {
		streamID = r.URL.Query().Get("stream")
		if streamID == "" {
			streamID = r.URL.Query().Get("channel")
		}
		if streamID == "" {
			streamID = r.URL.Query().Get("stream_id")
		}
	}

	var subParts []string
	if len(parts) > 1 {
		subParts = parts[1:]
	}

	h.DispatchWHEP(w, r, streamID, subParts)
}

// HandleStreamWHEP handles requests to /api/v1/streams/{id}/whep...
func (h *WHEPHandler) HandleStreamWHEP(w http.ResponseWriter, r *http.Request, streamID string, subParts []string) {
	h.DispatchWHEP(w, r, streamID, subParts)
}

// DispatchWHEP dispatches WHEP requests according to HTTP method and session route
func (h *WHEPHandler) DispatchWHEP(w http.ResponseWriter, r *http.Request, streamID string, subParts []string) {
	// Standard WHEP CORS Headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, PATCH, DELETE, GET, HEAD")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, If-Match, Range")
	w.Header().Set("Access-Control-Expose-Headers", "Location, Content-Type, Accept-Post, Link")
	w.Header().Set("Accept-Post", "application/sdp")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 1. Session Operations: .../whep/sessions/{session_id} or .../sessions/{session_id}
	if len(subParts) >= 2 && subParts[0] == "sessions" {
		sessionID := filepath.Base(subParts[1])
		switch r.Method {
		case http.MethodPatch:
			h.handlePatchCandidate(w, r, streamID, sessionID)
		case http.MethodDelete:
			h.handleDeleteSession(w, r, streamID, sessionID)
		default:
			http.Error(w, `{"error":"method not allowed on session"}`, http.StatusMethodNotAllowed)
		}
		return
	}

	// 2. Base WHEP Endpoint
	switch r.Method {
	case http.MethodPost:
		h.handlePostOffer(w, r, streamID)
	case http.MethodGet:
		h.handleGetInfo(w, r, streamID)
	default:
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
	}
}

func (h *WHEPHandler) handlePostOffer(w http.ResponseWriter, r *http.Request, streamID string) {
	if streamID == "" {
		http.Error(w, `{"error":"stream_id required"}`, http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil || len(body) == 0 {
		http.Error(w, `{"error":"SDP offer body required"}`, http.StatusBadRequest)
		return
	}

	sdpOffer := string(body)
	ans, err := h.useCase.HandleWHEPOffer(r.Context(), streamID, sdpOffer)
	if err != nil {
		if errors.Is(err, domain.ErrInvalidWHEPOffer) {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
		} else {
			http.Error(w, fmt.Sprintf(`{"error":"failed to negotiate WHEP: %s"}`, err.Error()), http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/sdp")
	w.Header().Set("Location", ans.Location)
	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(ans.SDPAnswer))
}

func (h *WHEPHandler) handlePatchCandidate(w http.ResponseWriter, r *http.Request, streamID, sessionID string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"invalid candidate body"}`, http.StatusBadRequest)
		return
	}

	if err := h.useCase.HandleWHEPPatch(r.Context(), streamID, sessionID, string(body)); err != nil {
		if errors.Is(err, domain.ErrWHEPSessionNotFound) {
			http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *WHEPHandler) handleDeleteSession(w http.ResponseWriter, r *http.Request, streamID, sessionID string) {
	if err := h.useCase.HandleWHEPDelete(r.Context(), streamID, sessionID); err != nil {
		if errors.Is(err, domain.ErrWHEPSessionNotFound) {
			http.Error(w, `{"error":"session not found"}`, http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusInternalServerError)
		}
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"session_terminated"}`))
}

func (h *WHEPHandler) handleGetInfo(w http.ResponseWriter, _ *http.Request, streamID string) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"protocol":  "WHEP",
		"version":   "RFC draft",
		"stream_id": streamID,
		"endpoint":  fmt.Sprintf("/api/v1/streams/%s/whep", streamID),
		"status":    "ready",
	})
}

