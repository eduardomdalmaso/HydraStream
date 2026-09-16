package domain_test

import (
	"testing"
	"time"

	"hydrastream/internal/domain"
)

func TestRecordingFragmentValidate(t *testing.T) {
	frag := &domain.RecordingFragment{
		StreamID:        "cam_01",
		DurationSeconds: 10,
	}

	if err := frag.Validate(); err != nil {
		t.Fatalf("expected valid fragment, got %v", err)
	}
	if frag.TenantID == "" {
		t.Errorf("expected default tenant ID")
	}
	if frag.StartTime.IsZero() || frag.EndTime.IsZero() {
		t.Errorf("expected generated start/end times")
	}

	invalid := &domain.RecordingFragment{}
	if err := invalid.Validate(); err != domain.ErrInvalidFragmentStreamID {
		t.Errorf("expected ErrInvalidFragmentStreamID, got %v", err)
	}

	invalidTimes := &domain.RecordingFragment{
		StreamID:  "cam_01",
		StartTime: time.Now(),
		EndTime:   time.Now().Add(-10 * time.Second),
	}
	if err := invalidTimes.Validate(); err != domain.ErrInvalidFragmentTimes {
		t.Errorf("expected ErrInvalidFragmentTimes, got %v", err)
	}
}

func TestWHEPOfferValidation(t *testing.T) {
	err := domain.ValidateWHEPOffer("v=0\r\no=- 0 0 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n")
	if err != nil {
		t.Errorf("expected valid SDP offer, got %v", err)
	}

	errInvalid := domain.ValidateWHEPOffer("invalid sdp text")
	if errInvalid != domain.ErrInvalidWHEPOffer {
		t.Errorf("expected ErrInvalidWHEPOffer, got %v", errInvalid)
	}
}
