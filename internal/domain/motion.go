package domain

import (
	"errors"
	"time"
)

var (
	// ErrInvalidGridDimensions indicates invalid cell motion layout.
	ErrInvalidGridDimensions = errors.New("grid dimensions must be between 2x2 and 64x64")
	// ErrInvalidSensitivity indicates sensitivity is out of [0, 100] bounds.
	ErrInvalidSensitivity = errors.New("sensitivity must be between 0 and 100")
)

// CellMotionConfig holds configuration for the cell motion analytics engine.
type CellMotionConfig struct {
	Columns         int     `json:"columns"`
	Rows            int     `json:"rows"`
	Sensitivity     float64 `json:"sensitivity"`      // 0 to 100
	CellThreshold   float64 `json:"cell_threshold"`   // Minimum luminance delta (0-255) per cell
	ActiveRatioMin  float64 `json:"active_ratio_min"` // Minimum fraction of cells triggered (0.0 to 1.0)
	CooldownSeconds int     `json:"cooldown_seconds"` // Post-event recording retention (e.g., 5s)
	PreBufferSec    int     `json:"pre_buffer_sec"`   // Pre-event ring buffer retention (e.g., 5s)
}

// DefaultCellMotionConfig returns standard ONVIF-compliant cell motion parameters.
func DefaultCellMotionConfig() CellMotionConfig {
	return CellMotionConfig{
		Columns:         16,
		Rows:            12,
		Sensitivity:     75.0,
		CellThreshold:   18.0,
		ActiveRatioMin:  0.02, // 2% of cells triggers motion
		CooldownSeconds: 5,
		PreBufferSec:    5,
	}
}

// Validate ensures motion configuration parameters satisfy domain invariants.
func (c *CellMotionConfig) Validate() error {
	if c.Columns < 2 || c.Columns > 64 || c.Rows < 2 || c.Rows > 64 {
		return ErrInvalidGridDimensions
	}
	if c.Sensitivity < 0 || c.Sensitivity > 100 {
		return ErrInvalidSensitivity
	}
	if c.CellThreshold <= 0 {
		c.CellThreshold = 18.0
	}
	if c.ActiveRatioMin <= 0 || c.ActiveRatioMin > 1.0 {
		c.ActiveRatioMin = 0.02
	}
	if c.CooldownSeconds <= 0 {
		c.CooldownSeconds = 5
	}
	if c.PreBufferSec <= 0 {
		c.PreBufferSec = 5
	}
	return nil
}

// MotionEvent represents a state transition in video motion detection.
type MotionEvent struct {
	StreamID        string    `json:"stream_id"`
	IsMotionActive  bool      `json:"is_motion_active"`
	ActiveCells     int       `json:"active_cells"`
	TotalCells      int       `json:"total_cells"`
	ActiveRatio     float64   `json:"active_ratio"`
	Timestamp       time.Time `json:"timestamp"`
	DurationSeconds float64   `json:"duration_seconds,omitempty"`
}
