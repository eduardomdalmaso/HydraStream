package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrStreamNotFound   = errors.New("stream not found")
	ErrInvalidStream    = errors.New("invalid stream data")
	ErrConsumerNotFound = errors.New("consumer not found")
)

// Standard Error Codes for HydraStream
const (
	ErrCodeGPUNotFound                 = "GPU_ACCELERATOR_NOT_FOUND"
	ErrCodeGPUTranscodeUnavailable     = "GPU_TRANSCODE_UNAVAILABLE"
	ErrCodeDecodeCorruptedNALU         = "DECODE_ERROR_CORRUPTED_NALU"
	ErrCodeMissingKeyframe             = "KEYFRAME_MISSING_OR_STARVED"
	ErrCodeGOPAnomaly                  = "CAMERA_ENCODER_GOP_ANOMALY"
	ErrCodeClientCodecUnsupported      = "CLIENT_CODEC_UNSUPPORTED"
	ErrCodeStorageLimitExceeded        = "STORAGE_LIMIT_EXCEEDED"
	ErrCodeRecordingSegmentationFailed = "RECORDING_SEGMENTATION_FAILED"
	ErrCodeRTSPAuthFailed              = "RTSP_AUTH_FAILED"
	ErrCodeRTSPTimeout                 = "RTSP_CONNECTION_TIMEOUT"
	ErrCodeInternalError               = "INTERNAL_SERVER_ERROR"
)

// Severity levels
const (
	SeverityInfo     = "INFO"
	SeverityWarning  = "WARNING"
	SeverityError    = "ERROR"
	SeverityCritical = "CRITICAL"
)

// AppError represents a domain-level structured error with rich diagnostics for external applications.
type AppError struct {
	Code           string         `json:"code"`
	Subsystem      string         `json:"subsystem"`
	StreamID       string         `json:"stream_id,omitempty"`
	Message        string         `json:"message"`
	Severity       string         `json:"severity"`
	Details        map[string]any `json:"details,omitempty"`
	ActionRequired string         `json:"action_required,omitempty"`
	Timestamp      time.Time      `json:"timestamp"`
}

func (e *AppError) Error() string {
	if e.StreamID != "" {
		return fmt.Sprintf("[%s] %s on stream '%s': %s", e.Severity, e.Code, e.StreamID, e.Message)
	}
	return fmt.Sprintf("[%s] %s: %s", e.Severity, e.Code, e.Message)
}

// NewGPUNotFoundError creates an AppError when hardware acceleration is requested but unavailable.
func NewGPUNotFoundError(subsystem, reason string) *AppError {
	return &AppError{
		Code:           ErrCodeGPUNotFound,
		Subsystem:      subsystem,
		Message:        "Aceleração de hardware GPU não foi localizada no host.",
		Severity:       SeverityWarning,
		Details:        map[string]any{"reason": reason, "gpu_detected": false},
		ActionRequired: "INSTALL_NVIDIA_DRIVERS_OR_USE_CPU_FALLBACK",
		Timestamp:      time.Now().UTC(),
	}
}

// NewGOPAnomalyError creates an AppError when GOP/Keyframe interval exceeds expected bounds.
func NewGOPAnomalyError(streamID string, measuredSec, expectedSec float64, missingFrames int) *AppError {
	return &AppError{
		Code:      ErrCodeGOPAnomaly,
		Subsystem: "gop_tracker",
		StreamID:  streamID,
		Message:   fmt.Sprintf("Intervalo de Keyframes (GOP) excessivo medido em %.1fs (esperado <= %.1fs).", measuredSec, expectedSec),
		Severity:  SeverityWarning,
		Details: map[string]any{
			"measured_gop_interval_sec": measuredSec,
			"expected_gop_interval_sec": expectedSec,
			"missing_iframe_count":      missingFrames,
			"suspected_cause":           "SMART_CODEC_OR_VARIABLE_GOP_ENABLED",
		},
		ActionRequired: "DISABLE_SMART_CODEC_OR_SET_GOP_TO_FPS",
		Timestamp:      time.Now().UTC(),
	}
}

// NewStorageLimitError creates an AppError when recording disk threshold is reached.
func NewStorageLimitError(streamID, mountPoint string, usedPct, thresholdPct float64, halted bool) *AppError {
	sev := SeverityWarning
	msg := fmt.Sprintf("Capacidade de armazenamento em %.1f%% atingiu o limite de alerta (%.1f%%).", usedPct, thresholdPct)
	act := "ENABLE_FIFO_PRUNING_OR_EXPAND_STORAGE"
	if halted {
		sev = SeverityCritical
		msg = fmt.Sprintf("Gravação interrompida: Armazenamento em %.1f%% atingiu o teto crítico (%.1f%%).", usedPct, thresholdPct)
		act = "EXPURGE_RECORDINGS_IMMEDIATELY"
	}

	return &AppError{
		Code:      ErrCodeStorageLimitExceeded,
		Subsystem: "recording_engine",
		StreamID:  streamID,
		Message:   msg,
		Severity:  sev,
		Details: map[string]any{
			"mount_point":          mountPoint,
			"used_percentage":      usedPct,
			"threshold_percentage": thresholdPct,
			"recording_halted":     halted,
		},
		ActionRequired: act,
		Timestamp:      time.Now().UTC(),
	}
}

// NewDecodeCorruptedError creates an AppError when stream frames or NALUs arrive corrupted.
func NewDecodeCorruptedError(streamID, codec, reason string) *AppError {
	return &AppError{
		Code:      ErrCodeDecodeCorruptedNALU,
		Subsystem: "demuxer",
		StreamID:  streamID,
		Message:   fmt.Sprintf("Falha de decodificação no fluxo '%s': %s", streamID, reason),
		Severity:  SeverityError,
		Details: map[string]any{
			"codec":  codec,
			"reason": reason,
		},
		ActionRequired: "CHECK_NETWORK_JITTER_OR_CAMERA_ENCODER",
		Timestamp:      time.Now().UTC(),
	}
}

// NewClientCodecUnsupportedError creates an AppError when the client cannot decode the stream codec.
func NewClientCodecUnsupportedError(streamID, codec, userAgent string) *AppError {
	return &AppError{
		Code:      ErrCodeClientCodecUnsupported,
		Subsystem: "wsp_playback",
		StreamID:  streamID,
		Message:   fmt.Sprintf("O cliente não suporta decodificação nativa para o codec '%s'.", codec),
		Severity:  SeverityInfo,
		Details: map[string]any{
			"stream_codec": codec,
			"user_agent":   userAgent,
		},
		ActionRequired: "SWITCH_TO_SUBSTREAM_OR_ENABLE_HEVC_EXTENSION",
		Timestamp:      time.Now().UTC(),
	}
}

