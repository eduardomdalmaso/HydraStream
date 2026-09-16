//go:build !windows

package gpu

import (
	"math"
	"syscall"

	"hydrastream/internal/domain"
)

// GetSHMTelemetry returns POSIX SHM filesystem statistics on Unix systems.
func GetSHMTelemetry() domain.StorageSHMTelemetry {
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

	return shmTel
}
