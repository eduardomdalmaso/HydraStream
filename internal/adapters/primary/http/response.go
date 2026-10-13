package http

import (
	"encoding/json"
	"net/http"
	"time"

	"hydrastream/internal/domain"
)

// StandardJSONError wraps domain.AppError for standardized HTTP JSON responses.
type StandardJSONError struct {
	Error *domain.AppError `json:"error"`
}

// WriteJSON writes a successful JSON response with given status code.
func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

// WriteJSONError writes a standardized domain error response with JSON payload.
func WriteJSONError(w http.ResponseWriter, status int, appErr *domain.AppError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if appErr.Timestamp.IsZero() {
		appErr.Timestamp = time.Now().UTC()
	}

	_ = json.NewEncoder(w).Encode(StandardJSONError{
		Error: appErr,
	})
}
