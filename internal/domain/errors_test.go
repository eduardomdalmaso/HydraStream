package domain_test

import (
	"testing"

	"hydrastream/internal/domain"
)

func TestAppErrorConstructors(t *testing.T) {
	// 1. GPU Error
	gpuErr := domain.NewGPUNotFoundError("gpu_manager", "nvidia-smi not in PATH")
	if gpuErr.Code != domain.ErrCodeGPUNotFound {
		t.Errorf("expected code %s, got %s", domain.ErrCodeGPUNotFound, gpuErr.Code)
	}
	if gpuErr.Severity != domain.SeverityWarning {
		t.Errorf("expected warning severity, got %s", gpuErr.Severity)
	}

	// 2. GOP Anomaly Error
	gopErr := domain.NewGOPAnomalyError("cam_01", 24.5, 2.0, 735)
	if gopErr.Code != domain.ErrCodeGOPAnomaly {
		t.Errorf("expected code %s, got %s", domain.ErrCodeGOPAnomaly, gopErr.Code)
	}
	if gopErr.StreamID != "cam_01" {
		t.Errorf("expected stream_id cam_01, got %s", gopErr.StreamID)
	}

	// 3. Storage Limit Error (Warning vs Critical)
	warnStorage := domain.NewStorageLimitError("cam_02", "/recordings", 91.0, 90.0, false)
	if warnStorage.Severity != domain.SeverityWarning {
		t.Errorf("expected warning, got %s", warnStorage.Severity)
	}

	critStorage := domain.NewStorageLimitError("cam_02", "/recordings", 96.5, 95.0, true)
	if critStorage.Severity != domain.SeverityCritical {
		t.Errorf("expected critical, got %s", critStorage.Severity)
	}

	// 4. Decode Corrupted Error
	decodeErr := domain.NewDecodeCorruptedError("cam_03", "h264", "missing SPS/PPS header")
	if decodeErr.Code != domain.ErrCodeDecodeCorruptedNALU {
		t.Errorf("expected code %s, got %s", domain.ErrCodeDecodeCorruptedNALU, decodeErr.Code)
	}

	// 5. Client Codec Unsupported
	clientErr := domain.NewClientCodecUnsupportedError("cam_04", "h265", "Mozilla/5.0")
	if clientErr.Code != domain.ErrCodeClientCodecUnsupported {
		t.Errorf("expected code %s, got %s", domain.ErrCodeClientCodecUnsupported, clientErr.Code)
	}

	// 6. Error() formatting
	if str := gopErr.Error(); str == "" {
		t.Errorf("expected non-empty string from Error()")
	}
}
