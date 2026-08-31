package gpu

import (
	"testing"
)

func TestDetectHardware(t *testing.T) {
	info := DetectHardware()
	if info.Detected {
		if info.Model == "" {
			t.Error("expected non-empty GPU model when detected")
		}
		if info.TotalVRAMMB <= 0 {
			t.Error("expected positive VRAM total when detected")
		}
		t.Logf("Detected Real GPU: %s | VRAM: %.1f MB (%.1f%%) | Temp: %.1f°C",
			info.Model, info.TotalVRAMMB, info.VRAMUsagePct, info.TempCelsius)
	} else {
		t.Log("No NVIDIA GPU detected, using CPU fallback")
	}
}
