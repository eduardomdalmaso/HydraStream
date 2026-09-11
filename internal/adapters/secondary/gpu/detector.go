package gpu

import (
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"hydrastream/internal/domain"
)

// HardwareInfo represents real detected GPU and hardware acceleration metrics (backward compatible).
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

// GetCompleteHardwareTelemetry collects host CPU, RAM, NVIDIA GPU, and POSIX /dev/shm stats.
func GetCompleteHardwareTelemetry() domain.HardwareTelemetry {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "localhost"
	}

	// 1. Host Memory from /proc/meminfo if on Linux
	var hostTotalRAM, hostUsedRAM, hostRAMPct float64
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		lines := strings.Split(string(data), "\n")
		var memTotalKB, memAvailKB float64
		for _, line := range lines {
			if strings.HasPrefix(line, "MemTotal:") {
				fmt.Sscanf(line, "MemTotal: %f kB", &memTotalKB)
			} else if strings.HasPrefix(line, "MemAvailable:") {
				fmt.Sscanf(line, "MemAvailable: %f kB", &memAvailKB)
			}
		}
		if memTotalKB > 0 {
			hostTotalRAM = math.Round(memTotalKB / 1024.0)
			hostUsedRAM = math.Round((memTotalKB - memAvailKB) / 1024.0)
			hostRAMPct = math.Round((hostUsedRAM / hostTotalRAM) * 1000.0) / 10.0
		}
	}

	// 2. Go runtime memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	memTel := domain.MemoryTelemetry{
		AllocMB:      math.Round(float64(m.Alloc)/(1024*1024)*10) / 10,
		TotalAllocMB: math.Round(float64(m.TotalAlloc)/(1024*1024)*10) / 10,
		SysMB:        math.Round(float64(m.Sys)/(1024*1024)*10) / 10,
		HeapAllocMB:  math.Round(float64(m.HeapAlloc)/(1024*1024)*10) / 10,
		HeapSysMB:    math.Round(float64(m.HeapSys)/(1024*1024)*10) / 10,
		NumGC:        m.NumGC,
		GCPauseMs:    math.Round(float64(m.PauseNs[(m.NumGC+255)%256])/1e6*100) / 100,
	}

	// 3. GPU detailed query via nvidia-smi
	gpuTel := domain.GPUHardwareTelemetry{
		Detected:         false,
		Model:            "CPU Host (No GPU acceleration)",
		EngineName:       "POSIX SHM (/dev/shm)",
		CUDAIPCSupported: false,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "nvidia-smi",
		"--query-gpu=name,memory.total,memory.used,memory.free,utilization.gpu,utilization.memory,temperature.gpu,power.draw,driver_version",
		"--format=csv,noheader,nounits")
	if out, err := cmd.Output(); err == nil {
		fields := strings.Split(strings.TrimSpace(string(out)), ",")
		if len(fields) >= 9 {
			model := strings.TrimSpace(fields[0])
			totalMB, _ := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)
			usedMB, _ := strconv.ParseFloat(strings.TrimSpace(fields[2]), 64)
			freeMB, _ := strconv.ParseFloat(strings.TrimSpace(fields[3]), 64)
			gpuUtil, _ := strconv.ParseFloat(strings.TrimSpace(fields[4]), 64)
			memUtil, _ := strconv.ParseFloat(strings.TrimSpace(fields[5]), 64)
			tempC, _ := strconv.ParseFloat(strings.TrimSpace(fields[6]), 64)
			powerW, _ := strconv.ParseFloat(strings.TrimSpace(fields[7]), 64)
			driverVer := strings.TrimSpace(fields[8])

			vramPct := 0.0
			if totalMB > 0 {
				vramPct = math.Round((usedMB/totalMB)*1000.0) / 10.0
			}

			gpuTel = domain.GPUHardwareTelemetry{
				Detected:          true,
				Model:             model,
				TotalVRAMMB:       totalMB,
				UsedVRAMMB:        usedMB,
				FreeVRAMMB:        freeMB,
				VRAMUsagePct:      vramPct,
				GPUUtilPct:        gpuUtil,
				MemoryUtilPct:     memUtil,
				TempCelsius:       tempC,
				PowerWatts:        powerW,
				DriverVersion:     driverVer,
				EngineName:        "NVDEC CUDA IPC (Zero-Copy VRAM)",
				CUDAIPCSupported:  true,
			}
		}
	}

	// 4. POSIX SHM metrics
	shmTel := domain.StorageSHMTelemetry{
		MountPoint:       "/dev/shm",
		RingBufferStatus: "ONLINE (Lock-Free)",
		LockFreeMode:     true,
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs("/dev/shm", &stat); err == nil && stat.Blocks > 0 {
		totalBytes := stat.Blocks * uint64(stat.Bsize)
		freeBytes := stat.Bfree * uint64(stat.Bsize)
		usedBytes := totalBytes - freeBytes
		shmTel.TotalBytes = totalBytes
		shmTel.UsedBytes = usedBytes
		shmTel.FreeBytes = freeBytes
		if totalBytes > 0 {
			shmTel.OccupancyPct = math.Round((float64(usedBytes)/float64(totalBytes))*1000.0) / 10.0
		}
	}

	return domain.HardwareTelemetry{
		Host: domain.HostHardware{
			Hostname:        hostname,
			OS:              runtime.GOOS,
			Architecture:    runtime.GOARCH,
			CPUCores:        runtime.NumCPU(),
			Goroutines:      runtime.NumGoroutine(),
			HostTotalRAMMB:  hostTotalRAM,
			HostUsedRAMMB:   hostUsedRAM,
			HostRAMUsagePct: hostRAMPct,
		},
		Memory:    memTel,
		GPU:       gpuTel,
		SHM:       shmTel,
		Timestamp: time.Now().UTC(),
	}
}
