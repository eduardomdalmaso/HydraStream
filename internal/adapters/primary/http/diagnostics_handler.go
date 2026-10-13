package http

import (
	"net/http"
	"strings"

	"hydrastream/internal/domain"
	"hydrastream/internal/ports"
)

// DiagnosticsHandler manages structured system and stream diagnostic endpoints.
type DiagnosticsHandler struct {
	useCase ports.StreamUseCase
}

// NewDiagnosticsHandler creates a new DiagnosticsHandler instance.
func NewDiagnosticsHandler(useCase ports.StreamUseCase) *DiagnosticsHandler {
	return &DiagnosticsHandler{useCase: useCase}
}

// HandleSystemDiagnostics returns global active system anomalies (GPU, SHM, Storage).
// GET /api/v1/diagnostics
func (dh *DiagnosticsHandler) HandleSystemDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	diagnostics, err := dh.useCase.GetSystemDiagnostics(r.Context())
	if err != nil {
		WriteJSONError(w, http.StatusInternalServerError, &domain.AppError{
			Code:      domain.ErrCodeInternalError,
			Subsystem: "diagnostics",
			Message:   err.Error(),
			Severity:  domain.SeverityError,
		})
		return
	}

	if diagnostics == nil {
		diagnostics = []*domain.AppError{}
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"active_diagnostics_count": len(diagnostics),
		"diagnostics":              diagnostics,
	})
}

// HandleStreamDiagnostics evaluates specific stream health and anomalies.
// GET /api/v1/streams/{id}/diagnostics
func (dh *DiagnosticsHandler) HandleStreamDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	// Extract streamID from URL path: /api/v1/streams/{id}/diagnostics
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) < 4 || parts[0] != "api" || parts[1] != "v1" || parts[2] != "streams" {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}
	streamID := parts[3]

	diag, err := dh.useCase.GetStreamDiagnostics(r.Context(), streamID)
	if err != nil {
		if err == domain.ErrStreamNotFound {
			WriteJSONError(w, http.StatusNotFound, &domain.AppError{
				Code:      "STREAM_NOT_FOUND",
				Subsystem: "ingest",
				StreamID:  streamID,
				Message:   "O fluxo de vídeo solicitado não foi encontrado no sistema.",
				Severity:  domain.SeverityWarning,
			})
			return
		}
		WriteJSONError(w, http.StatusInternalServerError, &domain.AppError{
			Code:      domain.ErrCodeInternalError,
			Subsystem: "diagnostics",
			StreamID:  streamID,
			Message:   err.Error(),
			Severity:  domain.SeverityError,
		})
		return
	}

	if diag != nil {
		// Stream has active anomaly: return 200 OK with anomaly details
		WriteJSON(w, http.StatusOK, map[string]any{
			"status":    "anomaly_detected",
			"stream_id": streamID,
			"anomaly":   diag,
		})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"status":    "healthy",
		"stream_id": streamID,
		"message":   "Fluxo operando nominalmente com GOP estável e decodificação normal.",
	})
}
