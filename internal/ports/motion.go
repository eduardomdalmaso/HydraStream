package ports

import (
	"hydrastream/internal/domain"
)

// MotionDetector defines the port for analyzing frame streams and detecting physical motion.
type MotionDetector interface {
	// AnalyzeFrame analyzes an image (Grayscale, RGB or YUV planar buffer) and returns whether motion occurred.
	AnalyzeFrame(streamID string, frameData []byte, width, height int, channels int) (*domain.MotionEvent, error)
	// SetConfig updates runtime motion detection parameters for a given stream.
	SetConfig(streamID string, config domain.CellMotionConfig) error
	// GetConfig retrieves current motion detection configuration.
	GetConfig(streamID string) domain.CellMotionConfig
	// Reset clears baseline background / motion history for a stream.
	Reset(streamID string)
}
