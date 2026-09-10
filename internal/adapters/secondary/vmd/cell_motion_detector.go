package vmd

import (
	"errors"
	"math"
	"sync"
	"time"

	"hydrastream/internal/domain"
	"hydrastream/internal/ports"
)

var (
	// ErrInvalidFrameDimensions indicates frame width or height is invalid.
	ErrInvalidFrameDimensions = errors.New("frame width and height must be positive")
	// ErrInvalidFrameBuffer indicates buffer size does not match dimensions.
	ErrInvalidFrameBuffer = errors.New("frame buffer size does not match specified dimensions and channels")
)

type streamMotionState struct {
	config         domain.CellMotionConfig
	prevCellValues []float64
	hasBaseline    bool
	isMotionActive bool
	lastMotionTime time.Time
	motionStart    time.Time
}

// CellMotionDetector implements the ONVIF-standard Cell Motion Detection engine.
type CellMotionDetector struct {
	mu      sync.RWMutex
	streams map[string]*streamMotionState
}

// NewCellMotionDetector creates a thread-safe motion detection adapter.
func NewCellMotionDetector() ports.MotionDetector {
	return &CellMotionDetector{
		streams: make(map[string]*streamMotionState),
	}
}

func (d *CellMotionDetector) getOrCreateState(streamID string) *streamMotionState {
	d.mu.Lock()
	defer d.mu.Unlock()

	s, ok := d.streams[streamID]
	if !ok {
		s = &streamMotionState{
			config: domain.DefaultCellMotionConfig(),
		}
		d.streams[streamID] = s
	}
	return s
}

// SetConfig updates motion detection parameters for a stream.
func (d *CellMotionDetector) SetConfig(streamID string, config domain.CellMotionConfig) error {
	if err := config.Validate(); err != nil {
		return err
	}
	s := d.getOrCreateState(streamID)
	d.mu.Lock()
	s.config = config
	s.hasBaseline = false // Invalidate baseline on config change
	d.mu.Unlock()
	return nil
}

// GetConfig returns the current motion detection config.
func (d *CellMotionDetector) GetConfig(streamID string) domain.CellMotionConfig {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if s, ok := d.streams[streamID]; ok {
		return s.config
	}
	return domain.DefaultCellMotionConfig()
}

// Reset clears the baseline and motion state.
func (d *CellMotionDetector) Reset(streamID string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	delete(d.streams, streamID)
}

// AnalyzeFrame processes a raw frame buffer and evaluates motion against cell grid baselines.
func (d *CellMotionDetector) AnalyzeFrame(streamID string, frameData []byte, width, height int, channels int) (*domain.MotionEvent, error) {
	if width <= 0 || height <= 0 {
		return nil, ErrInvalidFrameDimensions
	}
	if channels <= 0 {
		channels = 1
	}
	expectedLen := width * height * channels
	if len(frameData) < expectedLen {
		return nil, ErrInvalidFrameBuffer
	}

	s := d.getOrCreateState(streamID)
	d.mu.Lock()
	defer d.mu.Unlock()

	cfg := s.config
	totalCells := cfg.Columns * cfg.Rows
	currentCells := make([]float64, totalCells)

	cellW := width / cfg.Columns
	cellH := height / cfg.Rows
	if cellW <= 0 || cellH <= 0 {
		return nil, ErrInvalidFrameDimensions
	}

	// Calculate average intensity per cell with stride sampling for ultra-fast CPU performance (< 0.05ms)
	step := 2 // Subsample every 2nd pixel for speed
	for cy := 0; cy < cfg.Rows; cy++ {
		yStart := cy * cellH
		yEnd := yStart + cellH
		for cx := 0; cx < cfg.Columns; cx++ {
			xStart := cx * cellW
			xEnd := xStart + cellW
			cellIdx := cy*cfg.Columns + cx

			var sum float64
			var count int

			for y := yStart; y < yEnd; y += step {
				rowOffset := y * width * channels
				for x := xStart; x < xEnd; x += step {
					pxOffset := rowOffset + x*channels
					if channels == 1 {
						sum += float64(frameData[pxOffset])
					} else {
						// ITU-R BT.601 Luminance calculation: Y = 0.299R + 0.587G + 0.114B
						r := float64(frameData[pxOffset])
						g := float64(frameData[pxOffset+1])
						b := float64(frameData[pxOffset+2])
						sum += 0.299*r + 0.587*g + 0.114*b
					}
					count++
				}
			}

			if count > 0 {
				currentCells[cellIdx] = sum / float64(count)
			}
		}
	}

	now := time.Now()
	event := &domain.MotionEvent{
		StreamID:   streamID,
		TotalCells: totalCells,
		Timestamp:  now,
	}

	if !s.hasBaseline || len(s.prevCellValues) != totalCells {
		s.prevCellValues = currentCells
		s.hasBaseline = true
		s.isMotionActive = false
		return event, nil
	}

	// Sensitivity mapping: sensitivity 100 -> threshold multiplier 0.2, sensitivity 0 -> 2.0
	sensFactor := math.Max(0.2, (100.0-cfg.Sensitivity)/50.0)
	effectiveThreshold := math.Max(2.0, cfg.CellThreshold*sensFactor)

	activeCount := 0
	for i := 0; i < totalCells; i++ {
		delta := math.Abs(currentCells[i] - s.prevCellValues[i])
		if delta >= effectiveThreshold {
			activeCount++
		}
	}

	activeRatio := float64(activeCount) / float64(totalCells)
	event.ActiveCells = activeCount
	event.ActiveRatio = activeRatio

	// Adaptive EMA update for background baseline
	alpha := 0.25
	for i := 0; i < totalCells; i++ {
		s.prevCellValues[i] = (1-alpha)*s.prevCellValues[i] + alpha*currentCells[i]
	}

	hasInstantMotion := activeRatio >= cfg.ActiveRatioMin
	if hasInstantMotion {
		s.lastMotionTime = now
		if !s.isMotionActive {
			s.isMotionActive = true
			s.motionStart = now
		}
	} else if s.isMotionActive {
		// Check Cooldown timer
		cooldown := time.Duration(cfg.CooldownSeconds) * time.Second
		if now.Sub(s.lastMotionTime) >= cooldown {
			s.isMotionActive = false
		}
	}

	event.IsMotionActive = s.isMotionActive
	if s.isMotionActive {
		event.DurationSeconds = now.Sub(s.motionStart).Seconds()
	}

	return event, nil
}
