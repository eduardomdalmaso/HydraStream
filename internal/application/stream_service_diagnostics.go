package application

import (
	"context"
	"fmt"
	"strings"
	"time"

	"hydrastream/internal/adapters/secondary/gpu"
	"hydrastream/internal/domain"
)

// GetStreamDiagnostics evaluates active health and anomalies for a specific stream.
func (s *StreamService) GetStreamDiagnostics(ctx context.Context, streamID string) (*domain.AppError, error) {
	st, err := s.repo.FindByID(ctx, streamID)
	if err != nil {
		return nil, domain.ErrStreamNotFound
	}

	// 1. Check offline / reconnecting state
	if st.Status == "reconnecting" || st.Status == "offline" {
		return &domain.AppError{
			Code:      domain.ErrCodeRTSPTimeout,
			Subsystem: "ingest",
			StreamID:  streamID,
			Message:   fmt.Sprintf("Fluxo '%s' offline ou em processo de reconexão.", streamID),
			Severity:  domain.SeverityError,
			Details: map[string]any{
				"source_url":     st.SourceURL,
				"resource_score": st.ResourceScore,
				"status":         st.Status,
			},
			ActionRequired: "VERIFY_CAMERA_NETWORK_AND_CREDENTIALS",
			Timestamp:      time.Now().UTC(),
		}, nil
	}

	// 2. Check H.265 stream without GPU acceleration
	isH265 := strings.EqualFold(st.Codec, "h265") || strings.EqualFold(st.Codec, "hevc")
	hwInfo := gpu.DetectHardware()
	if isH265 && !hwInfo.Detected {
		return &domain.AppError{
			Code:      domain.ErrCodeGPUTranscodeUnavailable,
			Subsystem: "gpu_acceleration",
			StreamID:  streamID,
			Message:   "Stream configurado em H.265/HEVC operando em servidor sem acelerador GPU dedicado.",
			Severity:  domain.SeverityWarning,
			Details: map[string]any{
				"codec":        st.Codec,
				"gpu_detected": false,
				"resolution":   st.Resolution,
				"mode":         "passthrough_only",
			},
			ActionRequired: "USE_CLIENT_PASSTHROUGH_OR_ENABLE_H264_SUBSTREAM",
			Timestamp:      time.Now().UTC(),
		}, nil
	}

	// 3. Check low health score / GOP anomaly
	if st.ResourceScore > 0 && st.ResourceScore < 60.0 {
		return domain.NewGOPAnomalyError(streamID, 15.0, 2.0, 450), nil
	}

	// Stream healthy
	return nil, nil
}

// GetSystemDiagnostics collects system-wide active diagnostic alerts.
func (s *StreamService) GetSystemDiagnostics(ctx context.Context) ([]*domain.AppError, error) {
	var diagnostics []*domain.AppError

	// 1. GPU Acceleration Diagnostic
	hwInfo := gpu.DetectHardware()
	if !hwInfo.Detected {
		diagnostics = append(diagnostics, domain.NewGPUNotFoundError(
			"gpu_manager",
			"Placa de vídeo NVIDIA não encontrada ou driver nvidia-smi ausente no sistema operacional.",
		))
	}

	// 2. POSIX Shared Memory (/dev/shm) Space Check
	shmTele := gpu.GetSHMTelemetry()
	if shmTele.OccupancyPct > 90.0 {
		diagnostics = append(diagnostics, domain.NewStorageLimitError(
			"system_wide",
			shmTele.MountPoint,
			shmTele.OccupancyPct,
			90.0,
			shmTele.OccupancyPct >= 98.0,
		))
	}

	return diagnostics, nil
}
