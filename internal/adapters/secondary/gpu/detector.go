package gpu

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// HardwareInfo represents real detected GPU and hardware acceleration metrics.
type HardwareInfo struct {
	Detected     bool    `json:"detected"`
	Model        string  `json:"model"`
	TotalVRAMMB  float64 `json:"total_vram_mb"`
	UsedVRAMMB   float64 `json:"used_vram_mb"`
	VRAMUsagePct float64 `json:"vram_usage_pct"`
	GPUUtilPct   float64 `json:"gpu_util_pct"`
	TempCelsius  float64 `json:"temp_celsius"`
	EngineName   string  `json:"engine_name"`
}

// DetectHardware queries the underlying GPU hardware.
func DetectHardware() HardwareInfo {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "nvidia-smi", "--query-gpu=name,memory.total,memory.used,utilization.gpu,temperature.gpu", "--format=csv,noheader,nounits")
	out, err := cmd.Output()
	if err == nil {
		fields := strings.Split(strings.TrimSpace(string(out)), ",")
		if len(fields) >= 5 {
			model := strings.TrimSpace(fields[0])
			totalMB, _ := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)
			usedMB, _ := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
			utilPct, _ := strconv.ParseFloat(strings.TrimSpace(fields[3]), 64)
			tempC, _ := strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)

			vramPct := 0.0
			if totalMB > 0 {
				vramPct = (usedMB / totalMB) * 100.0
			}

			return HardwareInfo{
				Detected:     true,
				Model:        fmt.Sprintf("%s (%.0fGB VRAM)", model, totalMB/1024.0),
				TotalVRAMMB:  totalMB,
				UsedVRAMMB:   usedMB,
				VRAMUsagePct: vramPct,
				GPUUtilPct:   utilPct,
				TempCelsius:  tempC,
				EngineName:   "NVDEC CUDA IPC (Zero-Copy VRAM)",
			}
		}
	}

	// Fallback to CPU decoding if no NVIDIA GPU detected
	return HardwareInfo{
		Detected:     false,
		Model:        "CPU Host (Hardware Acceleration Disabled)",
		TotalVRAMMB:  0,
		UsedVRAMMB:   0,
		VRAMUsagePct: 0,
		GPUUtilPct:   0,
		TempCelsius:  0,
		EngineName:   "FFmpeg POSIX SHM (/dev/shm)",
	}
}
