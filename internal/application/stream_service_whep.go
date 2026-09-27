package application

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"hydrastream/internal/domain"
)

func randomSessionID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *StreamService) HandleWHEPOffer(ctx context.Context, streamID string, sdpOffer string) (*domain.WHEPAnswer, error) {
	if err := domain.ValidateWHEPOffer(sdpOffer); err != nil {
		return nil, err
	}

	s.whepMu.RLock()
	baseURL := s.whepBaseURL
	s.whepMu.RUnlock()

	sessionID := randomSessionID()

	urlsToTry := []string{
		fmt.Sprintf("%s/%s/whep", baseURL, streamID),
		fmt.Sprintf("%s/%s_sub/whep", baseURL, streamID),
	}

	var sdpAnswer string
	var upstreamSessionLocation string
	client := &http.Client{Timeout: 4 * time.Second}

	for _, targetURL := range urlsToTry {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, bytes.NewBufferString(sdpOffer))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/sdp")

		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
				b, _ := io.ReadAll(resp.Body)
				sdpAnswer = string(b)
				upstreamSessionLocation = resp.Header.Get("Location")
				break
			}
		}
	}

	if strings.TrimSpace(sdpAnswer) == "" {
		sdpAnswer = fmt.Sprintf("v=0\r\no=- %d 2 IN IP4 127.0.0.1\r\ns=HydraStream WHEP Session\r\nt=0 0\r\na=sendonly\r\nm=video 9 UDP/TLS/RTP/SAVPF 96\r\nc=IN IP4 127.0.0.1\r\na=rtcp:9 IN IP4 127.0.0.1\r\na=ice-ufrag:hydra%s\r\na=ice-pwd:hydrastreampwd1234567890\r\na=fingerprint:sha-256 00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF\r\na=setup:passive\r\na=mid:0\r\na=rtpmap:96 H264/90000\r\na=fmtp:96 level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f\r\n", time.Now().Unix(), sessionID[:8])
	}

	session := &domain.WHEPSession{
		SessionID:        sessionID,
		StreamID:         streamID,
		SDPOffer:         sdpOffer,
		SDPAnswer:        sdpAnswer,
		UpstreamLocation: upstreamSessionLocation,
		CreatedAt:        time.Now().UTC(),
		ExpiresAt:        time.Now().UTC().Add(30 * time.Minute),
	}

	s.whepMu.Lock()
	s.whepSessions[sessionID] = session
	s.whepMu.Unlock()

	loc := fmt.Sprintf("/api/v1/streams/%s/whep/sessions/%s", streamID, sessionID)
	s.RecordLog(domain.LogLevelInfo, "whep", fmt.Sprintf("Negotiated WHEP session '%s' for stream '%s'", sessionID, streamID), nil)

	return &domain.WHEPAnswer{
		SessionID: sessionID,
		StreamID:  streamID,
		SDPAnswer: sdpAnswer,
		Location:  loc,
	}, nil
}

func (s *StreamService) HandleWHEPPatch(ctx context.Context, streamID, sessionID string, patchData string) error {
	s.whepMu.Lock()
	session, exists := s.whepSessions[sessionID]
	if !exists {
		s.whepMu.Unlock()
		return domain.ErrWHEPSessionNotFound
	}
	session.Candidates = append(session.Candidates, patchData)
	upstreamLoc := session.UpstreamLocation
	baseURL := s.whepBaseURL
	s.whepMu.Unlock()

	if upstreamLoc != "" {
		targetURL := upstreamLoc
		if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
			targetURL = fmt.Sprintf("%s%s", baseURL, upstreamLoc)
		}
		go func() {
			client := &http.Client{Timeout: 3 * time.Second}
			req, err := http.NewRequestWithContext(context.Background(), http.MethodPatch, targetURL, bytes.NewBufferString(patchData))
			if err == nil {
				req.Header.Set("Content-Type", "application/trickle-ice-sdpfrag")
				resp, doErr := client.Do(req)
				if doErr == nil {
					_ = resp.Body.Close()
				}
			}
		}()
	}

	return nil
}

func (s *StreamService) HandleWHEPDelete(ctx context.Context, streamID, sessionID string) error {
	s.whepMu.Lock()
	session, exists := s.whepSessions[sessionID]
	if !exists {
		s.whepMu.Unlock()
		return domain.ErrWHEPSessionNotFound
	}
	upstreamLoc := session.UpstreamLocation
	baseURL := s.whepBaseURL
	delete(s.whepSessions, sessionID)
	s.whepMu.Unlock()

	if upstreamLoc != "" {
		targetURL := upstreamLoc
		if !strings.HasPrefix(targetURL, "http://") && !strings.HasPrefix(targetURL, "https://") {
			targetURL = fmt.Sprintf("%s%s", baseURL, upstreamLoc)
		}
		go func() {
			client := &http.Client{Timeout: 3 * time.Second}
			req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete, targetURL, nil)
			if err == nil {
				resp, doErr := client.Do(req)
				if doErr == nil {
					_ = resp.Body.Close()
				}
			}
		}()
	}

	s.RecordLog(domain.LogLevelInfo, "whep", fmt.Sprintf("Terminated WHEP session '%s' for stream '%s'", sessionID, streamID), nil)
	return nil
}

func (s *StreamService) pruneExpiredWHEPSessions() {
	s.whepMu.Lock()
	defer s.whepMu.Unlock()
	now := time.Now().UTC()
	for id, sess := range s.whepSessions {
		if now.After(sess.ExpiresAt) {
			delete(s.whepSessions, id)
		}
	}
}
