//go:build !windows

package proc

import (
	"os"
	"os/exec"
	"path/filepath"
)

// SetHideWindow is a no-op on non-Windows platforms.
func SetHideWindow(cmd *exec.Cmd) *exec.Cmd {
	return cmd
}

// GetFFmpegPath returns ffmpeg binary on unix, prioritizing high-performance local builds.
func GetFFmpegPath() string {
	candidates := []string{
		"./bin/ffmpeg",
		"bin/ffmpeg",
		"../HydraStream/bin/ffmpeg",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			if abs, err := filepath.Abs(c); err == nil {
				return abs
			}
			return c
		}
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	return "ffmpeg"
}

// GetFFprobePath returns ffprobe binary on unix, prioritizing local builds.
func GetFFprobePath() string {
	candidates := []string{
		"./bin/ffprobe",
		"bin/ffprobe",
		"../HydraStream/bin/ffprobe",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			if abs, err := filepath.Abs(c); err == nil {
				return abs
			}
			return c
		}
	}
	if p, err := exec.LookPath("ffprobe"); err == nil {
		return p
	}
	return "ffprobe"
}
