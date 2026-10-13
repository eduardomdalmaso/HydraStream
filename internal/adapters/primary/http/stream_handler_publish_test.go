package http_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	httpAdapter "hydrastream/internal/adapters/primary/http"
)

func TestStreamPublishEndpoint(t *testing.T) {
	mux, _, token := setupTestAppWithToken()

	payload := `{"stream_id": "drone_alpha_01", "name": "Patrol Drone"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/streams/publish", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp httpAdapter.StreamPublishResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode publish response: %v", err)
	}

	if resp.StreamID != "drone_alpha_01" {
		t.Errorf("expected stream_id 'drone_alpha_01', got '%s'", resp.StreamID)
	}
	if resp.Status != "waiting_stream" {
		t.Errorf("expected status 'waiting_stream', got '%s'", resp.Status)
	}
	if !strings.HasPrefix(resp.PublishURL, "rtmp://") {
		t.Errorf("expected RTMP publish URL, got '%s'", resp.PublishURL)
	}
	if !strings.HasPrefix(resp.WSPURL, "ws://") {
		t.Errorf("expected WSP URL, got '%s'", resp.WSPURL)
	}
	if resp.SHMPath != "/dev/shm/hydra_drone_alpha_01" {
		t.Errorf("expected SHM path '/dev/shm/hydra_drone_alpha_01', got '%s'", resp.SHMPath)
	}
	if resp.FanoutMode != "zero_copy_1_to_n" {
		t.Errorf("expected fanout_mode 'zero_copy_1_to_n', got '%s'", resp.FanoutMode)
	}
}

func TestStreamPublishAutoGenerateID(t *testing.T) {
	mux, _, token := setupTestAppWithToken()

	payload := `{}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/streams/publish", bytes.NewBufferString(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created, got %d. Body: %s", w.Code, w.Body.String())
	}

	var resp httpAdapter.StreamPublishResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode publish response: %v", err)
	}

	if !strings.HasPrefix(resp.StreamID, "push_") {
		t.Errorf("expected auto-generated ID starting with 'push_', got '%s'", resp.StreamID)
	}
}
