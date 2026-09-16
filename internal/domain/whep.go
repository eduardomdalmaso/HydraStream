package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidWHEPOffer   = errors.New("invalid or empty WHEP SDP offer")
	ErrWHEPSessionNotFound = errors.New("WHEP session not found or expired")
)

// WHEPSession represents an active WebRTC HTTP Egress Protocol session.
type WHEPSession struct {
	SessionID        string    `json:"session_id"`
	StreamID         string    `json:"stream_id"`
	SDPOffer         string    `json:"sdp_offer,omitempty"`
	SDPAnswer        string    `json:"sdp_answer,omitempty"`
	Candidates       []string  `json:"candidates,omitempty"`
	UpstreamLocation string    `json:"upstream_location,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
	ExpiresAt        time.Time `json:"expires_at"`
	RemoteAddr       string    `json:"remote_addr,omitempty"`
}


// WHEPAnswer encapsulates the result of a negotiated WHEP egress session.
type WHEPAnswer struct {
	SessionID  string `json:"session_id"`
	StreamID   string `json:"stream_id"`
	SDPAnswer  string `json:"sdp_answer"`
	Location   string `json:"location"`
}

// Validate ensures SDP Offer is valid.
func ValidateWHEPOffer(sdp string) error {
	trimmed := strings.TrimSpace(sdp)
	if trimmed == "" || !strings.Contains(trimmed, "v=0") {
		return ErrInvalidWHEPOffer
	}
	return nil
}
