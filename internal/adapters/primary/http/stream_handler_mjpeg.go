package http

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"hydrastream/internal/adapters/secondary/proc"
	"hydrastream/internal/domain"
)

func (h *Handler) handleMJPEG(w http.ResponseWriter, r *http.Request, st *domain.Stream) {
	w.Header().Set("Content-Type", "multipart/x-mixed-replace; boundary=ffmpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, hasFlusher := w.(http.Flusher)
	if hasFlusher {
		flusher.Flush()
	}

	streamID := filepath.Base(st.StreamID)
	ffmpegBin := proc.GetFFmpegPath()

	sourceURL := st.SourceURL
	if sourceURL == "" {
		dbPaths := []string{
			"../hydravms/hydravms.db",
			"hydravms.db",
		}
		for _, dbp := range dbPaths {
			if _, errStat := os.Stat(dbp); errStat == nil {
				cmd := exec.Command("sqlite3", dbp, fmt.Sprintf("SELECT rtsp_url FROM cameras WHERE id = '%s' LIMIT 1;", streamID))
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

	urlsToTry := []string{
		fmt.Sprintf("rtsp://localhost:8554/%s_sub", streamID),
		fmt.Sprintf("rtsp://localhost:8554/%s", streamID),
	}
	if sourceURL != "" && !strings.Contains(sourceURL, "localhost:8554") {
		urlsToTry = append(urlsToTry, sourceURL)
	}

	for {
		select {
		case <-r.Context().Done():
			return
		default:
		}

		streamed := false
		for _, targetURL := range urlsToTry {
			args := []string{
				"-loglevel", "error",
				"-fflags", "nobuffer",
				"-flags", "low_delay",
				"-fflags", "+discardcorrupt",
				"-rtsp_transport", "tcp",
				"-timeout", "5000000",
				"-i", targetURL,
				"-an",
				"-threads", "2",
				"-c:v", "mjpeg",
				"-q:v", "5",
				"-r", "15",
				"-f", "mpjpeg",
				"-boundary_tag", "ffmpeg",
				"-",
			}

			cmd := exec.CommandContext(r.Context(), ffmpegBin, args...)
			proc.SetHideWindow(cmd)
			cmd.Stdout = w
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err == nil || r.Context().Err() != nil {
				return
			}
			streamed = true
		}

		if !streamed || r.Context().Err() != nil {
			return
		}
		time.Sleep(300 * time.Millisecond)
	}
}
