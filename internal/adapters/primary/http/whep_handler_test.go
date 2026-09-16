package http_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWHEPProtocolEndpoints(t *testing.T) {
	mux, _, token := setupTestAppWithToken()

	// 1. Test OPTIONS on /whep/cam_test_01
	optReq := httptest.NewRequest(http.MethodOptions, "/whep/cam_test_01", nil)
	optRR := httptest.NewRecorder()
	mux.ServeHTTP(optRR, optReq)

	if optRR.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on WHEP OPTIONS, got %d", optRR.Code)
	}
	if optRR.Header().Get("Accept-Post") != "application/sdp" {
		t.Errorf("expected Accept-Post: application/sdp header")
	}

	// 2. Test POST SDP offer on /whep/cam_test_01
	sdpOffer := "v=0\r\no=- 123456 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\na=recvonly\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\n"
	postReq := httptest.NewRequest(http.MethodPost, "/whep/cam_test_01", bytes.NewBufferString(sdpOffer))
	postReq.Header.Set("Content-Type", "application/sdp")
	postRR := httptest.NewRecorder()
	mux.ServeHTTP(postRR, postReq)

	if postRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on WHEP POST, got %d: %s", postRR.Code, postRR.Body.String())
	}
	if postRR.Header().Get("Content-Type") != "application/sdp" {
		t.Errorf("expected application/sdp response header")
	}
	loc := postRR.Header().Get("Location")
	if loc == "" || !strings.Contains(loc, "/whep/sessions/") {
		t.Fatalf("expected valid session Location header, got %s", loc)
	}
	if !strings.Contains(postRR.Body.String(), "v=0") {
		t.Errorf("expected valid SDP answer in body")
	}

	// 3. Test PATCH trickle-ICE on session
	patchReq := httptest.NewRequest(http.MethodPatch, loc, bytes.NewBufferString("candidate:1 1 UDP 2130706431 127.0.0.1 50000 typ host"))
	patchReq.Header.Set("Content-Type", "application/trickle-ice-sdpfrag")
	patchReq.Header.Set("Authorization", "Bearer "+token)
	patchRR := httptest.NewRecorder()
	mux.ServeHTTP(patchRR, patchReq)

	if patchRR.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content on WHEP PATCH, got %d", patchRR.Code)
	}

	// 4. Test DELETE on session
	delReq := httptest.NewRequest(http.MethodDelete, loc, nil)
	delReq.Header.Set("Authorization", "Bearer "+token)
	delRR := httptest.NewRecorder()
	mux.ServeHTTP(delRR, delReq)

	if delRR.Code != http.StatusOK && delRR.Code != http.StatusNoContent {
		t.Fatalf("expected 200/204 on WHEP DELETE, got %d", delRR.Code)
	}

	// 5. Test subroute on /api/v1/streams/cam_test_01/whep
	streamWhepReq := httptest.NewRequest(http.MethodPost, "/api/v1/streams/cam_test_01/whep", bytes.NewBufferString(sdpOffer))
	streamWhepReq.Header.Set("Content-Type", "application/sdp")
	streamWhepReq.Header.Set("Authorization", "Bearer "+token)
	streamWhepRR := httptest.NewRecorder()
	mux.ServeHTTP(streamWhepRR, streamWhepReq)

	if streamWhepRR.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on /api/v1/streams/cam_test_01/whep, got %d", streamWhepRR.Code)
	}
}
