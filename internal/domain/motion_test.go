package domain_test

import (
	"testing"

	"hydrastream/internal/domain"
)

func TestCellMotionConfig_Validate(t *testing.T) {
	cfg := domain.DefaultCellMotionConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected default config to be valid, got: %v", err)
	}

	invalidGrid := cfg
	invalidGrid.Columns = 1
	if err := invalidGrid.Validate(); err != domain.ErrInvalidGridDimensions {
		t.Errorf("expected ErrInvalidGridDimensions, got: %v", err)
	}

	invalidSens := cfg
	invalidSens.Sensitivity = 150.0
	if err := invalidSens.Validate(); err != domain.ErrInvalidSensitivity {
		t.Errorf("expected ErrInvalidSensitivity, got: %v", err)
	}
}
