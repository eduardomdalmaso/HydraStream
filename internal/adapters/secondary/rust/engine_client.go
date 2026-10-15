package rust

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"hydrastream/internal/adapters/secondary/proc"
)

type RustProbeResult struct {
	Online     bool   `json:"online"`
	Codec      string `json:"codec"`
	Resolution string `json:"resolution"`
	FPS        int    `json:"fps"`
	LatencyMs  int64  `json:"latency_ms"`
	SDP        string `json:"sdp"`
}

func getHydraEnginePath() string {
	candidates := []string{
		filepath.Join("bin", "hydra-engine.exe"),
		filepath.Join("target", "release", "hydra-engine.exe"),
		filepath.Join("target", "debug", "hydra-engine.exe"),
		"hydra-engine.exe",
		filepath.Join("..", "bin", "hydra-engine.exe"),
	}
	for _, p := range candidates {
		if fi, err := os.Stat(p); err == nil && !fi.IsDir() {
			return p
		}
	}
	return "hydra-engine.exe"
}

// ProbeRTSPNativeRust executes hydra-engine probe using pure Rust retina RTSP client
func ProbeRTSPNativeRust(rawURL string, timeout time.Duration) (*RustProbeResult, error) {
	engineBin := getHydraEnginePath()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, engineBin, "probe", "--url", rawURL)
	proc.SetHideWindow(cmd)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("hydra-engine probe failed: %w", err)
	}

	var res RustProbeResult
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, fmt.Errorf("invalid json from hydra-engine probe: %w", err)
	}

	return &res, nil
}

// CaptureSnapshotNativeRust executes hydra-engine snapshot using pure Rust retina RTSP client
func CaptureSnapshotNativeRust(rawURL string, outFile string, timeout time.Duration) ([]byte, error) {
	engineBin := getHydraEnginePath()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, engineBin, "snapshot", "--url", rawURL, "--out", outFile)
	proc.SetHideWindow(cmd)
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("hydra-engine snapshot failed: %w", err)
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read captured snapshot file: %w", err)
	}

	return data, nil
}

// SpawnStreamPipe starts hydra-engine stream --url <url> and returns its stdout reader and cleanup func
func SpawnStreamPipe(ctx context.Context, rawURL string) (io.ReadCloser, func(), error) {
	engineBin := getHydraEnginePath()
	cmd := exec.CommandContext(ctx, engineBin, "stream", "--url", rawURL)
	proc.SetHideWindow(cmd)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	cleanup := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}
	return stdout, cleanup, nil
}
