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
	ffmpegBin := getFFmpegPath()

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

	targetURL := sourceURL
	if targetURL == "" || strings.HasPrefix(targetURL, "synthetic://") {
		targetURL = fmt.Sprintf("rtsp://localhost:8554/%s", streamID)
	}

	for {
		select {
		case <-r.Context().Done():
			return
		default:
		}

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

		if err := cmd.Run(); err != nil {
			if r.Context().Err() != nil {
				return
			}
			samplePath := filepath.Clean(filepath.Join("samples", fmt.Sprintf("%s.jpg", streamID)))
			data, errRead := os.ReadFile(samplePath)
			if errRead == nil && len(data) > 0 {
				fmt.Fprintf(w, "--ffmpeg\r\nContent-Type: image/jpeg\r\nContent-Length: %d\r\n\r\n", len(data))
				w.Write(data)
				w.Write([]byte("\r\n"))
				if hasFlusher {
					flusher.Flush()
				}
			}
			time.Sleep(300 * time.Millisecond)
		}
	}
}
