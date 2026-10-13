package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	httpAdapter "hydrastream/internal/adapters/primary/http"
	"hydrastream/internal/adapters/secondary/memory"
	"hydrastream/internal/application"
	"hydrastream/internal/domain"
)

func setupTestAppWithToken() (*http.ServeMux, *application.StreamService, string) {
	repo := memory.NewStreamRepository()
	fragRepo := memory.NewFragmentRepository("test_recordings")
	service := application.NewStreamService(repo, nil, nil)
	service.SetFragmentRepository(fragRepo)

	token := generateTestToken("00000000-0000-0000-0000-000000000001", "admin")

	// Pre-register a stream
	_ = service.RegisterStream(context.Background(), &domain.Stream{
		StreamID:  "cam_test_01",
		TenantID:  "00000000-0000-0000-0000-000000000001",
		SourceURL: "synthetic://test",
	})

	handler := httpAdapter.NewHandler(service)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux, service, token
}

func TestFragmentUploadAndDownload(t *testing.T) {
	mux, _, token := setupTestAppWithToken()

	// 1. Upload fragment via multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("stream_id", "cam_test_01")
	_ = writer.WriteField("recording_mode", "motion")
	_ = writer.WriteField("duration_seconds", "15.5")
	_ = writer.WriteField("start_time", time.Now().Add(-16*time.Second).Format(time.RFC3339))
	_ = writer.WriteField("end_time", time.Now().Format(time.RFC3339))

	part, _ := writer.CreateFormFile("file", "segment01.mp4")
	fakeVideoData := []byte("FAKE_MP4_VIDEO_HEADER_AND_FRAMES_DATA_123456789")
	_, _ = part.Write(fakeVideoData)
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/streams/cam_test_01/recordings/fragments", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status 201 Created on fragment upload, got %d: %s", rr.Code, rr.Body.String())
	}

	var uploadResp struct {
		Status   string                   `json:"status"`
		Fragment domain.RecordingFragment `json:"fragment"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &uploadResp); err != nil {
		t.Fatalf("failed to decode upload response: %v", err)
	}

	fragID := uploadResp.Fragment.ID
	if fragID == "" {
		t.Fatalf("expected non-empty fragment ID")
	}

	// 2. List fragments
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/streams/cam_test_01/recordings/fragments", nil)
	listReq.Header.Set("Authorization", "Bearer "+token)
	listRR := httptest.NewRecorder()
	mux.ServeHTTP(listRR, listReq)

	if listRR.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK on listing fragments, got %d", listRR.Code)
	}

	var listResp struct {
		Count     int                        `json:"count"`
		Fragments []domain.RecordingFragment `json:"fragments"`
	}
	_ = json.Unmarshal(listRR.Body.Bytes(), &listResp)
	if listResp.Count < 1 {
		t.Fatalf("expected at least 1 fragment listed, got %d", listResp.Count)
	}

	// 3. Download fragment
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/streams/cam_test_01/recordings/fragments/"+fragID, nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	getRR := httptest.NewRecorder()
	mux.ServeHTTP(getRR, getReq)

	if getRR.Code != http.StatusOK {
		t.Fatalf("expected status 200 OK on getting fragment, got %d", getRR.Code)
	}
	if !bytes.Equal(getRR.Body.Bytes(), fakeVideoData) {
		t.Fatalf("downloaded fragment data mismatch")
	}

	// 4. Test Timezone Queries (RFC3339 with offset -03:00, SQL format, etc.)
	tzReq := httptest.NewRequest(http.MethodGet, "/api/v1/streams/cam_test_01/recordings/fragments?start=2026-01-01T00:00:00-03:00&end=2030-01-01T00:00:00Z", nil)
	tzReq.Header.Set("Authorization", "Bearer "+token)
	tzRR := httptest.NewRecorder()
	mux.ServeHTTP(tzRR, tzReq)
	if tzRR.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on timezone query, got %d", tzRR.Code)
	}

	sqlReq := httptest.NewRequest(http.MethodGet, "/api/v1/streams/cam_test_01/recordings/fragments?start=2026-01-01%2000:00:00", nil)
	sqlReq.Header.Set("Authorization", "Bearer "+token)
	sqlRR := httptest.NewRecorder()
	mux.ServeHTTP(sqlRR, sqlReq)
	if sqlRR.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on SQL timestamp format query, got %d", sqlRR.Code)
	}
}

