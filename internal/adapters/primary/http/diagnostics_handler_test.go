package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"hydrastream/internal/domain"
)

func TestDiagnosticsHandler(t *testing.T) {
	mux, service, token := setupTestAppWithToken()

	// 1. System diagnostics
	req := httptest.NewRequest(http.MethodGet, "/api/v1/diagnostics", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from system diagnostics, got %d. Body: %s", w.Code, w.Body.String())
	}

	// 2. Stream diagnostics for non-existent stream
	req404 := httptest.NewRequest(http.MethodGet, "/api/v1/streams/non_existent/diagnostics", nil)
	req404.Header.Set("Authorization", "Bearer "+token)
	w404 := httptest.NewRecorder()
	mux.ServeHTTP(w404, req404)

	if w404.Code != http.StatusNotFound {
		t.Fatalf("expected 404 from non-existent stream diagnostics, got %d. Body: %s", w404.Code, w404.Body.String())
	}

	// 3. Register a stream and test diagnostics
	_ = service.RegisterStream(context.Background(), &domain.Stream{
		StreamID:  "cam_test_diag",
		TenantID:  "00000000-0000-0000-0000-000000000001",
		SourceURL: "rtsp://127.0.0.1:554/live",
		Codec:     "h264",
		Status:    "online",
	})

	reqOk := httptest.NewRequest(http.MethodGet, "/api/v1/streams/cam_test_diag/diagnostics", nil)
	reqOk.Header.Set("Authorization", "Bearer "+token)
	wOk := httptest.NewRecorder()
	mux.ServeHTTP(wOk, reqOk)

	if wOk.Code != http.StatusOK {
		t.Fatalf("expected 200 OK from stream diagnostics, got %d. Body: %s", wOk.Code, wOk.Body.String())
	}
}
