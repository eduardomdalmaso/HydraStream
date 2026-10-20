package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hydrastream/internal/adapters/secondary/proc"
	"hydrastream/internal/adapters/secondary/rust"
)

var (
	h264BridgeMu sync.Mutex
	h264Bridges  = make(map[string]*exec.Cmd)
	h264Pipes    = make(map[string]func())
	h264Starting = make(map[string]chan struct{})
	nvencOnce    sync.Once
	nvencAvail   bool
)

func killOrphanFFmpegForStream(cleanID string) {
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
		fmt.Sprintf("Get-CimInstance Win32_Process -Filter \"Name = 'ffmpeg.exe'\" | Where-Object { $_.CommandLine -like '*1935/live/%s*' } | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }", cleanID))
	proc.SetHideWindow(cmd)
	_ = cmd.Run()
}

// StopAllH264Bridges stops all running FFmpeg bridge processes
func StopAllH264Bridges() {
	h264BridgeMu.Lock()
	defer h264BridgeMu.Unlock()
	for id, clean := range h264Pipes {
		if clean != nil {
			clean()
		}
		delete(h264Pipes, id)
	}
	for id, cmd := range h264Bridges {
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		delete(h264Bridges, id)
	}
}

func isNVENCAvailable() bool {
	nvencOnce.Do(func() {
		cmd := exec.Command(proc.GetFFmpegPath(), "-f", "lavfi", "-i", "nullsrc=s=256x256:d=0.04", "-c:v", "h264_nvenc", "-f", "null", "-")
		proc.SetHideWindow(cmd)
		if err := cmd.Run(); err == nil {
			nvencAvail = true
		}
	})
	return nvencAvail
}

func stopH264BridgeInternal(streamID string) {
	h264BridgeMu.Lock()
	defer h264BridgeMu.Unlock()
	cleanID := filepath.Base(streamID)

	for _, id := range []string{streamID, cleanID} {
		if clean, has := h264Pipes[id]; has && clean != nil {
			clean()
			delete(h264Pipes, id)
		}
		if cmd, exists := h264Bridges[id]; exists && cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
			delete(h264Bridges, id)
		}
	}
	killOrphanFFmpegForStream(cleanID)
}

func (h *Handler) handleH264BridgeStart(w http.ResponseWriter, r *http.Request, streamID string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	cleanID := filepath.Base(streamID)
	baseID := strings.TrimSuffix(cleanID, "_sub")

	h264BridgeMu.Lock()
	if cmd, running := h264Bridges[cleanID]; running && cmd != nil && cmd.Process != nil {
		h264BridgeMu.Unlock()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "already_running", "stream_id": cleanID})
		return
	}
	if ch, starting := h264Starting[cleanID]; starting {
		h264BridgeMu.Unlock()
		<-ch
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "already_running", "stream_id": cleanID})
		return
	}
	startCh := make(chan struct{})
	h264Starting[cleanID] = startCh
	h264BridgeMu.Unlock()

	defer func() {
		h264BridgeMu.Lock()
		delete(h264Starting, cleanID)
		close(startCh)
		h264BridgeMu.Unlock()
	}()

	// Terminate any active MJPEG readers for this stream to free the RTSP socket
	stopMJPEGInternal(cleanID)

	sourceURL := ""

	st, _ := h.useCase.GetStream(r.Context(), streamID)
	if st != nil && st.SourceURL != "" && !strings.Contains(st.SourceURL, "localhost") {
		sourceURL = st.SourceURL
	}

	if sourceURL == "" {
		dbPaths := []string{"../hydravms/hydravms.db", "hydravms.db"}
		for _, dbp := range dbPaths {
			if _, errStat := os.Stat(dbp); errStat == nil {
				field := "rtsp_url"
				if strings.HasSuffix(cleanID, "_sub") {
					field = "COALESCE(NULLIF(sub_stream_url, ''), rtsp_url)"
				}
				cmd := exec.Command("sqlite3", dbp, fmt.Sprintf("SELECT %s FROM cameras WHERE id = '%s' LIMIT 1;", field, baseID))
				proc.SetHideWindow(cmd)
				if out, errCmd := cmd.Output(); errCmd == nil {
					u := strings.TrimSpace(string(out))
					if u != "" {
						sourceURL = u
						break
					}
				}
			}
		}
	}

	if sourceURL == "" {
		http.Error(w, `{"error":"camera source url not found"}`, http.StatusNotFound)
		return
	}

	rtmpPort := os.Getenv("RTMP_PORT")
	if rtmpPort == "" {
		rtmpPort = "1935"
	}
	targetRTMP := fmt.Sprintf("rtmp://127.0.0.1:%s/live/%s", rtmpPort, cleanID)
	ffmpegBin := proc.GetFFmpegPath()



	var cmd *exec.Cmd
	var pipeCleanup func()

	codecFormat := "h264"
	videoArgs := []string{"-c:v", "copy"}
	if strings.HasSuffix(cleanID, "_sub") || (st != nil && (strings.EqualFold(st.Codec, "h265") || strings.EqualFold(st.Codec, "hevc"))) {
		codecFormat = "hevc"
		if isNVENCAvailable() {
			videoArgs = []string{
				"-c:v", "h264_nvenc",
				"-preset", "p1",
				"-tune", "ull",
				"-zerolatency", "1",
				"-profile:v", "baseline",
				"-level", "3.1",
				"-bf", "0",
				"-forced-idr", "1",
				"-g", "25",
				"-b:v", "1000k",
				"-maxrate", "1500k",
				"-bufsize", "500k",
			}
		} else {
			videoArgs = []string{"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency", "-profile:v", "baseline", "-level", "3.1", "-bf", "0", "-g", "25", "-threads", "2"}
		}
	}

	baseFlags := []string{
		"-loglevel", "warning",
		"-probesize", "32768",
		"-analyzeduration", "0",
		"-fflags", "nobuffer+fastseek+genpts",
		"-flags", "low_delay",
		"-avoid_negative_ts", "make_zero",
	}

	killOrphanFFmpegForStream(cleanID)

	if strings.HasPrefix(strings.ToLower(sourceURL), "rtsp://") && !strings.Contains(sourceURL, "localhost:") && !strings.Contains(sourceURL, "127.0.0.1:") {
		pipeStdout, cleanup, errPipe := rust.SpawnStreamPipe(context.Background(), sourceURL)
		if errPipe == nil {
			pipeCleanup = cleanup
			args := append(append(baseFlags, "-f", codecFormat, "-i", "-"), videoArgs...)
			args = append(args, "-an", "-f", "flv", targetRTMP)
			cmd = exec.Command(ffmpegBin, args...)
			proc.SetHideWindow(cmd)
			cmd.Stdin = pipeStdout
		}
	}

	if cmd == nil {
		args := append(append(baseFlags, "-rtsp_transport", "tcp", "-i", sourceURL), videoArgs...)
		args = append(args, "-an", "-f", "flv", targetRTMP)
		cmd = exec.Command(ffmpegBin, args...)
		proc.SetHideWindow(cmd)
	}

	if err := cmd.Start(); err != nil {
		if pipeCleanup != nil {
			pipeCleanup()
		}
		http.Error(w, fmt.Sprintf(`{"error":"failed to start h264 bridge: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	h264BridgeMu.Lock()
	h264Bridges[cleanID] = cmd
	if pipeCleanup != nil {
		h264Pipes[cleanID] = pipeCleanup
	}
	h264BridgeMu.Unlock()

	go func() {
		_ = cmd.Wait()
		if pipeCleanup != nil {
			pipeCleanup()
		}
		h264BridgeMu.Lock()
		if cur, ok := h264Bridges[cleanID]; ok && cur == cmd {
			delete(h264Bridges, cleanID)
			delete(h264Pipes, cleanID)
		}
		h264BridgeMu.Unlock()
	}()

	time.Sleep(150 * time.Millisecond)

	wspPort := os.Getenv("WSP_PORT")
	if wspPort == "" {
		wspPort = "8085"
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "started",
		"stream_id": streamID,
		"wsp_url":   fmt.Sprintf("ws://%s:%s/live/%s", getHostForPublish(r), wspPort, cleanID),
	})
}

func (h *Handler) handleH264BridgeStop(w http.ResponseWriter, _ *http.Request, streamID string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	stopH264BridgeInternal(streamID)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "stopped", "stream_id": streamID})
}

func isH264BridgeActive(streamID string) bool {
	h264BridgeMu.Lock()
	defer h264BridgeMu.Unlock()
	cleanID := filepath.Base(streamID)
	for _, id := range []string{streamID, cleanID} {
		if cmd, ok := h264Bridges[id]; ok && cmd != nil && cmd.Process != nil {
			return true
		}
	}
	return false
}
