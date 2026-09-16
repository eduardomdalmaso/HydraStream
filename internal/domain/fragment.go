package domain

import (
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidFragmentStreamID = errors.New("invalid or empty stream_id for recording fragment")
	ErrInvalidFragmentTimes    = errors.New("fragment end_time must be after start_time")
	ErrInvalidFragmentDuration = errors.New("fragment duration must be greater than zero")
	ErrFragmentNotFound        = errors.New("recording fragment not found")
)

// RecordingFragment represents a discrete video chunk/segment captured and stored by HydraStream.
type RecordingFragment struct {
	ID              string    `json:"id"`
	TenantID        string    `json:"tenant_id"`
	StreamID        string    `json:"stream_id"`
	RecordingMode   string    `json:"recording_mode"` // "continuous", "motion", "ai_event", "manual"
	StartTime       time.Time `json:"start_time"`
	EndTime         time.Time `json:"end_time"`
	DurationSeconds float64   `json:"duration_seconds"`
	FileSizeBytes   int64     `json:"file_size_bytes"`
	StoragePath     string    `json:"storage_path"`
	ContentType     string    `json:"content_type"` // e.g. "video/mp4", "video/iso.segment", "video/mp2t"
	Codec           string    `json:"codec"`        // "h264", "h265", "aac"
	CreatedAt       time.Time `json:"created_at"`
}

// Validate ensures domain consistency of the recording fragment.
func (f *RecordingFragment) Validate() error {
	if strings.TrimSpace(f.StreamID) == "" {
		return ErrInvalidFragmentStreamID
	}
	if f.TenantID == "" {
		f.TenantID = "00000000-0000-0000-0000-000000000001"
	}
	if f.RecordingMode == "" {
		f.RecordingMode = "motion"
	}
	if !f.StartTime.IsZero() && !f.EndTime.IsZero() && f.EndTime.Before(f.StartTime) {
		return ErrInvalidFragmentTimes
	}
	if f.DurationSeconds <= 0 {
		if !f.StartTime.IsZero() && !f.EndTime.IsZero() && f.EndTime.After(f.StartTime) {
			f.DurationSeconds = f.EndTime.Sub(f.StartTime).Seconds()
		} else {
			return ErrInvalidFragmentDuration
		}
	}
	if f.StartTime.IsZero() {
		f.StartTime = time.Now().Add(-time.Duration(f.DurationSeconds * float64(time.Second)))
	}
	if f.EndTime.IsZero() {
		f.EndTime = f.StartTime.Add(time.Duration(f.DurationSeconds * float64(time.Second)))
	}
	if f.EndTime.Before(f.StartTime) {
		return ErrInvalidFragmentTimes
	}

	if f.ContentType == "" {
		f.ContentType = "video/mp4"
	}
	if f.Codec == "" {
		f.Codec = "h264"
	}
	if f.CreatedAt.IsZero() {
		f.CreatedAt = time.Now().UTC()
	}
	return nil
}
