package http

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"hydrastream/internal/adapters/secondary/proc"
	"hydrastream/internal/domain"
)

var (
	mjpegMu      sync.Mutex
	mjpegCancels = make(map[string]context.CancelFunc)
)

func stopMJPEGInternal(streamID string) {
	mjpegMu.Lock()
	defer mjpegMu.Unlock()
	cleanID := filepath.Base(streamID)
	baseID := strings.TrimSuffix(cleanID, "_sub")

	for _, id := range []string{streamID, cleanID, baseID} {
		if cancel, exists := mjpegCancels[id]; exists && cancel != nil {
			cancel()
			delete(mjpegCancels, id)
		}
	}
}

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

	cleanID := filepath.Base(st.StreamID)
	baseID := strings.TrimSuffix(cleanID, "_sub")
	ffmpegBin := proc.GetFFmpegPath()

	// Terminate any running H264 bridge for this camera so RTSP socket is freed
	stopH264BridgeInternal(cleanID)
	stopH264BridgeInternal(baseID)

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	mjpegMu.Lock()
	if oldCancel, exists := mjpegCancels[cleanID]; exists && oldCancel != nil {
		oldCancel()
	}
	mjpegCancels[cleanID] = cancel
	mjpegMu.Unlock()

	defer func() {
		mjpegMu.Lock()
		delete(mjpegCancels, cleanID)
		mjpegMu.Unlock()
	}()

	sourceURL := st.SourceURL
	if sourceURL == "" {
		dbPaths := []string{
			"../hydravms/hydravms.db",
			"hydravms.db",
		}
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

	var urlsToTry []string
	if sourceURL != "" && !strings.Contains(sourceURL, "localhost:8554") {
		urlsToTry = append(urlsToTry, sourceURL)
	}
	urlsToTry = append(urlsToTry,
		fmt.Sprintf("rtsp://localhost:8554/%s", cleanID),
	)

	// Extract dynamic quality and fps from query params (?q=4&fps=20)
	qParam := r.URL.Query().Get("q")
	if qParam == "" || len(qParam) > 2 {
		qParam = "4"
	}
	fpsParam := r.URL.Query().Get("fps")
	if fpsParam == "" || len(fpsParam) > 2 {
		fpsParam = "20"
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		for _, targetURL := range urlsToTry {
			args := []string{
				"-loglevel", "warning",
				"-fflags", "nobuffer+discardcorrupt",
				"-flags", "low_delay",
				"-rtsp_transport", "tcp",
				"-timeout", "5000000",
				"-i", targetURL,
				"-an",
				"-threads", "2",
				"-c:v", "mjpeg",
				"-q:v", qParam,
				"-r", fpsParam,
				"-f", "mpjpeg",
				"-boundary_tag", "ffmpeg",
				"-",
			}

			cmd := exec.CommandContext(ctx, ffmpegBin, args...)
			proc.SetHideWindow(cmd)
			cmd.Stdout = w
			cmd.Stderr = os.Stderr

			if err := cmd.Run(); err == nil || ctx.Err() != nil {
				return
			}
		}

		if ctx.Err() != nil {
			return
		}

		time.Sleep(1 * time.Second)
	}
}

func isMJPEGActive(streamID string) bool {
	mjpegMu.Lock()
	defer mjpegMu.Unlock()
	cleanID := filepath.Base(streamID)
	baseID := strings.TrimSuffix(cleanID, "_sub")
	for _, id := range []string{streamID, cleanID, baseID} {
		if _, ok := mjpegCancels[id]; ok {
			return true
		}
	}
	return false
}

