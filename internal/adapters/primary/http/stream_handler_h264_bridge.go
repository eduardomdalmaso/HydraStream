package http

import (
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
)

var (
	h264BridgeMu sync.Mutex
	h264Bridges   = make(map[string]*exec.Cmd)
)

func stopH264BridgeInternal(streamID string) {
	h264BridgeMu.Lock()
	defer h264BridgeMu.Unlock()
	cleanID := filepath.Base(streamID)
	baseID := strings.TrimSuffix(cleanID, "_sub")

	for _, id := range []string{streamID, cleanID, baseID} {
		if cmd, exists := h264Bridges[id]; exists && cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
			delete(h264Bridges, id)
		}
	}
}

func (h *Handler) handleH264BridgeStart(w http.ResponseWriter, r *http.Request, streamID string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	cleanID := filepath.Base(streamID)
	baseID := strings.TrimSuffix(cleanID, "_sub")

	// Terminate any active MJPEG readers for this camera to free the RTSP socket
	stopMJPEGInternal(cleanID)
	stopMJPEGInternal(baseID)
	time.Sleep(150 * time.Millisecond)

	h264BridgeMu.Lock()
	if cmd, running := h264Bridges[streamID]; running && cmd != nil && cmd.Process != nil {
		h264BridgeMu.Unlock()
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "already_running", "stream_id": streamID})
		return
	}
	h264BridgeMu.Unlock()

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
		rtmpPort = "1936"
	}
	targetRTMP := fmt.Sprintf("rtmp://127.0.0.1:%s/live/%s", rtmpPort, cleanID)
	ffmpegBin := proc.GetFFmpegPath()

	_ = os.MkdirAll("samples", 0755)
	samplePath := filepath.Clean(filepath.Join("samples", fmt.Sprintf("%s.jpg", cleanID)))

	args := []string{
		"-loglevel", "warning",
		"-fflags", "+genpts",
		"-avoid_negative_ts", "make_zero",
		"-rtsp_transport", "tcp",
		"-i", sourceURL,
		"-c:v", "copy",
		"-an",
		"-f", "flv",
		targetRTMP,
		"-map", "0:v",
		"-vf", "fps=1",
		"-update", "1",
		"-y", samplePath,
	}

	cmd := exec.Command(ffmpegBin, args...)
	proc.SetHideWindow(cmd)

	if err := cmd.Start(); err != nil {
		http.Error(w, fmt.Sprintf(`{"error":"failed to start h264 bridge: %s"}`, err.Error()), http.StatusInternalServerError)
		return
	}

	h264BridgeMu.Lock()
	h264Bridges[streamID] = cmd
	h264BridgeMu.Unlock()

	go func() {
		_ = cmd.Wait()
		h264BridgeMu.Lock()
		if cur, ok := h264Bridges[streamID]; ok && cur == cmd {
			delete(h264Bridges, streamID)
		}
		h264BridgeMu.Unlock()
	}()

	time.Sleep(300 * time.Millisecond)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "started",
		"stream_id": streamID,
		"wsp_url":   fmt.Sprintf("ws://%s:%s/live/%s", getHostForPublish(r), "8086", cleanID),
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
	baseID := strings.TrimSuffix(cleanID, "_sub")
	for _, id := range []string{streamID, cleanID, baseID} {
		if cmd, ok := h264Bridges[id]; ok && cmd != nil && cmd.Process != nil {
			return true
		}
	}
	return false
}
