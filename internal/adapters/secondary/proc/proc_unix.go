//go:build !windows

package proc

import "os/exec"

// SetHideWindow is a no-op on non-Windows platforms.
func SetHideWindow(cmd *exec.Cmd) *exec.Cmd {
	return cmd
}

// GetFFmpegPath returns ffmpeg binary on unix.
func GetFFmpegPath() string {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	return "ffmpeg"
}

// GetFFprobePath returns ffprobe binary on unix.
func GetFFprobePath() string {
	if p, err := exec.LookPath("ffprobe"); err == nil {
		return p
	}
	return "ffprobe"
}
