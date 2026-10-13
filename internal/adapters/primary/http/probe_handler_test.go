package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeEndpointValidation(t *testing.T) {
	h := &Handler{}

	// Invalid empty target
	body := `{"url": ""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/streams/probe", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	h.HandleProbeStream(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var probeResp StreamProbeResponse
	if err := json.NewDecoder(w.Body).Decode(&probeResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if probeResp.Online {
		t.Errorf("expected online=false for empty url, got true")
	}
	if probeResp.Error == "" {
		t.Errorf("expected error message for empty url")
	}
}

func TestStreamProbeResponseFields(t *testing.T) {
	resp := StreamProbeResponse{
		Online:         true,
		Codec:          "H.264",
		Resolution:     "1920x1080 Full HD",
		FPS:            30,
		ONVIFAvailable: true,
		ONVIFPort:      80,
		SubstreamURL:   "rtsp://192.168.1.100:554/stream2",
		Profiles:       []string{"Main (1920x1080 @ 30 FPS)", "Sub (640x360 @ 15 FPS)"},
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal StreamProbeResponse: %v", err)
	}

	var parsed StreamProbeResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal StreamProbeResponse: %v", err)
	}

	if !parsed.ONVIFAvailable {
		t.Errorf("expected ONVIFAvailable=true")
	}
	if parsed.ONVIFPort != 80 {
		t.Errorf("expected ONVIFPort=80, got %d", parsed.ONVIFPort)
	}
	if parsed.SubstreamURL != "rtsp://192.168.1.100:554/stream2" {
		t.Errorf("unexpected SubstreamURL: %s", parsed.SubstreamURL)
	}
	if len(parsed.Profiles) != 2 {
		t.Errorf("expected 2 profiles, got %d", len(parsed.Profiles))
	}
}
