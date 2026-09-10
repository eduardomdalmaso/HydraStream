package vmd_test

import (
	"testing"
	"time"

	"hydrastream/internal/adapters/secondary/vmd"
	"hydrastream/internal/domain"
)

func TestCellMotionDetector_StaticScene(t *testing.T) {
	detector := vmd.NewCellMotionDetector()
	w, h := 160, 120
	staticFrame := make([]byte, w*h)
	for i := range staticFrame {
		staticFrame[i] = 128
	}

	// 1. First frame establishes baseline
	ev1, err := detector.AnalyzeFrame("cam_test", staticFrame, w, h, 1)
	if err != nil {
		t.Fatalf("unexpected error on first frame: %v", err)
	}
	if ev1.IsMotionActive {
		t.Errorf("expected no motion on initial baseline frame")
	}

	// 2. Second static frame should have 0 active cells
	ev2, err := detector.AnalyzeFrame("cam_test", staticFrame, w, h, 1)
	if err != nil {
		t.Fatalf("unexpected error on second frame: %v", err)
	}
	if ev2.IsMotionActive || ev2.ActiveCells > 0 {
		t.Errorf("expected no motion on identical frame, got active cells: %d", ev2.ActiveCells)
	}
}

func TestCellMotionDetector_MotionTriggerAndCooldown(t *testing.T) {
	detector := vmd.NewCellMotionDetector()
	cfg := domain.DefaultCellMotionConfig()
	cfg.CooldownSeconds = 1
	_ = detector.SetConfig("cam_test", cfg)

	w, h := 160, 120
	frame1 := make([]byte, w*h)
	for i := range frame1 {
		frame1[i] = 50
	}
	_, _ = detector.AnalyzeFrame("cam_test", frame1, w, h, 1)

	// Invert frame brightness to simulate sudden human movement in front of camera
	frame2 := make([]byte, w*h)
	for i := range frame2 {
		frame2[i] = 220
	}

	ev, err := detector.AnalyzeFrame("cam_test", frame2, w, h, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ev.IsMotionActive || ev.ActiveCells == 0 {
		t.Errorf("expected motion trigger, got active: %v, cells: %d", ev.IsMotionActive, ev.ActiveCells)
	}

	// Feed static frames until baseline adapts and cooldown expires
	for i := 0; i < 15; i++ {
		_, _ = detector.AnalyzeFrame("cam_test", frame2, w, h, 1)
	}

	// Wait for cooldown (1 second)
	time.Sleep(1100 * time.Millisecond)
	evAfterCooldown, _ := detector.AnalyzeFrame("cam_test", frame2, w, h, 1)
	if evAfterCooldown.IsMotionActive {
		t.Errorf("expected motion to deactivate after cooldown timeout")
	}
}
