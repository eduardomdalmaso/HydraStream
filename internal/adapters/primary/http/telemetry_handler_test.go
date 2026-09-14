package http_test

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	httpAdapter "hydrastream/internal/adapters/primary/http"
	"hydrastream/internal/adapters/secondary/logger"
	"hydrastream/internal/adapters/secondary/memory"
	"hydrastream/internal/application"
	"hydrastream/internal/domain"
)

func init() {
	os.Setenv("JWT_SECRET", "super-secret-key-that-is-at-least-32-chars-long-for-testing!")
	os.Setenv("SERVICE_API_KEY", "test-hydra-service-api-key")
}

func generateTestToken(tenantID, role string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	payloadData, _ := json.Marshal(map[string]interface{}{
		"user_id":   "test-user-123",
		"tenant_id": tenantID,
		"role":      role,
		"exp":       time.Now().Add(1 * time.Hour).Unix(),
	})
	payload := base64.RawURLEncoding.EncodeToString(payloadData)

	mac := hmac.New(sha256.New, []byte("super-secret-key-that-is-at-least-32-chars-long-for-testing!"))
	mac.Write([]byte(header + "." + payload))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	return fmt.Sprintf("%s.%s.%s", header, payload, sig)
}

func setupTestServer() *http.ServeMux {
	repo := memory.NewStreamRepository()
	ringLogger := logger.NewRingLogger(100)
	svc := application.NewStreamService(repo, nil, nil, ringLogger)
	handler := httpAdapter.NewHandler(svc)

	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	return mux
}

func TestUnauthenticatedRequestBlocked(t *testing.T) {
	mux := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected status 401 Unauthorized, got %d", w.Code)
	}
}

func TestHealthEndpoint(t *testing.T) {
	mux := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken("tenant_alpha", "viewer"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var health domain.SystemHealth
	if err := json.NewDecoder(w.Body).Decode(&health); err != nil {
		t.Fatalf("failed to decode health JSON: %v", err)
	}

	if health.Service != "hydrastream-dataplane" {
		t.Errorf("expected hydrastream-dataplane, got %s", health.Service)
	}
	if health.Status != "healthy" {
		t.Errorf("expected healthy status, got %s", health.Status)
	}
}

func TestHardwareTelemetryEndpoint(t *testing.T) {
	mux := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/hardware", nil)
	req.Header.Set("X-API-Key", "test-hydra-service-api-key")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var hw domain.HardwareTelemetry
	if err := json.NewDecoder(w.Body).Decode(&hw); err != nil {
		t.Fatalf("failed to decode hardware JSON: %v", err)
	}

	if hw.Host.CPUCores <= 0 {
		t.Errorf("expected positive CPU cores, got %d", hw.Host.CPUCores)
	}
	if hw.SHM.MountPoint != "/dev/shm" {
		t.Errorf("expected /dev/shm mount point, got %s", hw.SHM.MountPoint)
	}
}

func TestUnifiedTelemetryEndpoint(t *testing.T) {
	mux := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry", nil)
	req.Header.Set("Authorization", "Bearer "+generateTestToken("tenant_alpha", "viewer"))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var unified domain.UnifiedTelemetry
	if err := json.NewDecoder(w.Body).Decode(&unified); err != nil {
		t.Fatalf("failed to decode unified telemetry JSON: %v", err)
	}

	if unified.Health.Service == "" {
		t.Errorf("expected populated health field")
	}
}

func TestLogsAndErrorsEndpoints(t *testing.T) {
	mux := setupTestServer()
	token := generateTestToken("tenant_alpha", "admin")

	// 1. Ingest a log via POST
	logBody := `{"level":"ERROR","component":"test_ingest","message":"RTSP packet dropped"}`
	postReq := httptest.NewRequest(http.MethodPost, "/api/v1/telemetry/logs", strings.NewReader(logBody))
	postReq.Header.Set("Content-Type", "application/json")
	postReq.Header.Set("Authorization", "Bearer "+token)
	wPost := httptest.NewRecorder()
	mux.ServeHTTP(wPost, postReq)

	if wPost.Code != http.StatusCreated {
		t.Fatalf("expected status 201 on log POST, got %d", wPost.Code)
	}

	// 2. Query logs via GET
	getReq := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/logs?level=ERROR", nil)
	getReq.Header.Set("Authorization", "Bearer "+token)
	wGet := httptest.NewRecorder()
	mux.ServeHTTP(wGet, getReq)

	if wGet.Code != http.StatusOK {
		t.Fatalf("expected status 200 on logs GET, got %d", wGet.Code)
	}

	var logRes domain.LogQueryResult
	if err := json.NewDecoder(wGet.Body).Decode(&logRes); err != nil {
		t.Fatalf("failed to decode logs JSON: %v", err)
	}

	if len(logRes.Logs) == 0 {
		t.Fatalf("expected at least 1 log entry in result")
	}
	if logRes.Logs[0].Component != "test_ingest" {
		t.Errorf("expected test_ingest, got %s", logRes.Logs[0].Component)
	}

	// 3. Query errors summary via GET
	errReq := httptest.NewRequest(http.MethodGet, "/api/v1/telemetry/errors", nil)
	errReq.Header.Set("Authorization", "Bearer "+token)
	wErr := httptest.NewRecorder()
	mux.ServeHTTP(wErr, errReq)

	if wErr.Code != http.StatusOK {
		t.Fatalf("expected status 200 on errors GET, got %d", wErr.Code)
	}

	var errSummary domain.ErrorSummary
	if err := json.NewDecoder(wErr.Body).Decode(&errSummary); err != nil {
		t.Fatalf("failed to decode error summary JSON: %v", err)
	}

	if errSummary.TotalErrors == 0 {
		t.Errorf("expected total errors > 0")
	}
	if errSummary.ErrorsByComponent["test_ingest"] != 1 {
		t.Errorf("expected 1 error for test_ingest, got %d", errSummary.ErrorsByComponent["test_ingest"])
	}
}

func TestRootDiscoveryEndpoint(t *testing.T) {
	mux := setupTestServer()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200 on root, got %d", w.Code)
	}

	var root map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&root); err != nil {
		t.Fatalf("failed to decode root JSON: %v", err)
	}

	if root["service"] != "hydrastream-dataplane" {
		t.Errorf("expected hydrastream-dataplane service, got %v", root["service"])
	}
}
