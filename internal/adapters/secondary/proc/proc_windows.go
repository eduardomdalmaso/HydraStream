//go:build windows

package proc

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

// SetHideWindow suppresses the console window creation on Windows when launching subprocesses.
func SetHideWindow(cmd *exec.Cmd) *exec.Cmd {
	if cmd == nil {
		return nil
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags = 0x08000000 // CREATE_NO_WINDOW
	return cmd
}

// GetFFmpegPath searches for ffmpeg executable in PATH, bin, and standard WinGet locations.
func GetFFmpegPath() string {
	if p, err := exec.LookPath("ffmpeg.exe"); err == nil {
		return p
	}
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	candidates := []string{
		"./bin/ffmpeg.exe",
		"../HydraStream/bin/ffmpeg.exe",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		matches, _ := filepath.Glob(filepath.Join(localAppData, "Microsoft", "WinGet", "Packages", "*", "*", "bin", "ffmpeg.exe"))
		if len(matches) > 0 {
			return matches[0]
		}
		matches, _ = filepath.Glob(filepath.Join(localAppData, "Microsoft", "WinGet", "Links", "ffmpeg.exe"))
		if len(matches) > 0 {
			return matches[0]
		}
	}
	userProfile := os.Getenv("USERPROFILE")
	if userProfile != "" {
		matches, _ := filepath.Glob(filepath.Join(userProfile, "AppData", "Local", "Microsoft", "WinGet", "Packages", "*", "*", "bin", "ffmpeg.exe"))
		if len(matches) > 0 {
			return matches[0]
		}
	}
	return "ffmpeg.exe"
}

// GetFFprobePath searches for ffprobe executable in PATH, bin, and standard WinGet locations.
func GetFFprobePath() string {
	if p, err := exec.LookPath("ffprobe.exe"); err == nil {
		return p
	}
	if p, err := exec.LookPath("ffprobe"); err == nil {
		return p
	}
	candidates := []string{
		"./bin/ffprobe.exe",
		"../HydraStream/bin/ffprobe.exe",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		matches, _ := filepath.Glob(filepath.Join(localAppData, "Microsoft", "WinGet", "Packages", "*", "*", "bin", "ffprobe.exe"))
		if len(matches) > 0 {
			return matches[0]
		}
		matches, _ = filepath.Glob(filepath.Join(localAppData, "Microsoft", "WinGet", "Links", "ffprobe.exe"))
		if len(matches) > 0 {
			return matches[0]
		}
	}
	return "ffprobe.exe"
}
