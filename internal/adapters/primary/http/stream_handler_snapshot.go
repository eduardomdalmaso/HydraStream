package http

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"hydrastream/internal/adapters/secondary/proc"
	"hydrastream/internal/adapters/secondary/rust"
	"hydrastream/internal/domain"
)

func (h *Handler) handleSnapshot(w http.ResponseWriter, r *http.Request, streamID string) {
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")

	cleanBase := filepath.Base(streamID)
	_ = os.MkdirAll("samples", 0755)
	samplePath := filepath.Clean(filepath.Join("samples", fmt.Sprintf("%s.jpg", cleanBase)))
	if !strings.HasPrefix(samplePath, "samples") && !strings.HasPrefix(samplePath, "samples/") && !strings.HasPrefix(samplePath, "samples\\") {
		http.Error(w, `{"error":"invalid path"}`, http.StatusBadRequest)
		return
	}

	var sourceURL string
	if r != nil && r.URL.Query().Get("url") != "" {
		sourceURL = r.URL.Query().Get("url")
	}
	if sourceURL == "" {
		if st, err := h.useCase.GetStream(r.Context(), streamID); err == nil && st != nil {
			sourceURL = st.SourceURL
		}
	}
	if sourceURL == "" {
		dbPaths := []string{
			"../hydravms/hydravms.db",
			"hydravms.db",
		}
		for _, dbp := range dbPaths {
			if _, errStat := os.Stat(dbp); errStat == nil {
				cmd := exec.Command("sqlite3", dbp, fmt.Sprintf("SELECT rtsp_url FROM cameras WHERE id = '%s' AND (is_active = 1 OR is_active IS NULL) LIMIT 1;", cleanBase))
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

	// If the stream does not exist or was deleted, purge stale cache and return standby frame
	if sourceURL == "" {
		_ = os.Remove(samplePath)
		altMatches, _ := filepath.Glob(fmt.Sprintf("samples/*%s*.jpg", cleanBase))
		for _, alt := range altMatches {
			_ = os.Remove(alt)
		}
		standbyData := generateStandbySnapshot(cleanBase)
		_, _ = w.Write(standbyData)
		return
	}

	isRefresh := r != nil && (r.URL.Query().Get("refresh") == "true" || r.URL.Query().Get("force") == "true")

	// 1. If H264 forwarder is active, it continuously writes samplePath at 1 FPS
	if isH264BridgeActive(cleanBase) {
		for i := 0; i < 4; i++ {
			if fi, errStat := os.Stat(samplePath); errStat == nil && time.Since(fi.ModTime()) < 3*time.Second {
				if data, errRead := os.ReadFile(samplePath); errRead == nil && len(data) > 0 {
					_, _ = w.Write(data)
					return
				}
			}
			time.Sleep(250 * time.Millisecond)
		}
	}

	// 2. If MJPEG reader is active, grab 1 frame directly from local HTTP stream to avoid RTSP socket collision
	if isMJPEGActive(cleanBase) {
		if frame, err := extractFrameFromLocalMJPEG(cleanBase); err == nil && len(frame) > 0 {
			_ = os.WriteFile(samplePath, frame, 0644)
			_, _ = w.Write(frame)
			return
		}
	}

	// 3. If not refresh, deliver cached file immediately (only if real snapshot > 20KB)
	if !isRefresh {
		if fi, errStat := os.Stat(samplePath); errStat == nil && fi.Size() > 20000 {
			if data, err := os.ReadFile(samplePath); err == nil && len(data) > 0 {
				_, _ = w.Write(data)
				return
			}
		}
	}

	if fi, errStat := os.Stat(samplePath); os.IsNotExist(errStat) || (errStat == nil && fi.Size() <= 20000) {
		if probeData, errP := os.ReadFile(filepath.Join("samples", "latest_probe.jpg")); errP == nil && len(probeData) > 20000 {
			if fiP, errStatP := os.Stat(filepath.Join("samples", "latest_probe.jpg")); errStatP == nil && time.Since(fiP.ModTime()) < 120*time.Second {
				_ = os.WriteFile(samplePath, probeData, 0644)
				_, _ = w.Write(probeData)
				return
			}
		}
	}

	// 3.5 Try HydraEngine Rust (retina client) first to resolve camera Digest/OPTIONS quirks immediately (<500ms)
	if sourceURL != "" && strings.HasPrefix(strings.ToLower(sourceURL), "rtsp://") && !strings.Contains(sourceURL, "localhost:") && !strings.Contains(sourceURL, "127.0.0.1:") {
		if data, errSnap := rust.CaptureSnapshotNativeRust(sourceURL, samplePath, 6*time.Second); errSnap == nil && len(data) > 20000 {
			_, _ = w.Write(data)
			return
		}
	}

	var urlsToTry []string
	if sourceURL != "" && !strings.Contains(sourceURL, "localhost:8554") {
		urlsToTry = append(urlsToTry, sourceURL)
	}
	urlsToTry = append(urlsToTry,
		fmt.Sprintf("rtsp://localhost:8554/%s_sub", cleanBase),
		fmt.Sprintf("rtsp://localhost:8554/%s", cleanBase),
	)

	ffmpegBin := proc.GetFFmpegPath()
	for _, u := range urlsToTry {
		snapCmd := exec.Command(ffmpegBin, "-rtsp_transport", "tcp", "-timeout", "3000000",
			"-i", u, "-update", "1", "-frames:v", "1", "-q:v", "2", "-y", samplePath)
		proc.SetHideWindow(snapCmd)
		if err := snapCmd.Run(); err == nil {
			if data, errRead := os.ReadFile(samplePath); errRead == nil && len(data) > 20000 {
				_, _ = w.Write(data)
				return
			}
		}
	}

	if data, err := os.ReadFile(samplePath); err == nil && len(data) > 20000 {
		_, _ = w.Write(data)
		return
	}

	if sourceURL != "" {
		if u, errParse := url.Parse(sourceURL); errParse == nil {
			host := u.Hostname()
			user := ""
			pass := ""
			if u.User != nil {
				user = u.User.Username()
				pass, _ = u.User.Password()
			}
			if host != "" && host != "127.0.0.1" && host != "localhost" {
				ctxTimeout, cancel := context.WithTimeout(r.Context(), 3*time.Second)
				dev, errProbe := h.useCase.ProbeONVIFDevice(ctxTimeout, domain.ONVIFProbeRequest{
					IPAddress: host,
					Port:      80,
					Username:  user,
					Password:  pass,
				})
				cancel()
				if errProbe == nil && dev != nil && dev.SnapshotURL != "" && strings.HasPrefix(dev.SnapshotURL, "data:image/jpeg;base64,") {
					b64 := strings.TrimPrefix(dev.SnapshotURL, "data:image/jpeg;base64,")
					if rawBytes, errDec := base64.StdEncoding.DecodeString(b64); errDec == nil && len(rawBytes) > 20000 {
						_ = os.WriteFile(samplePath, rawBytes, 0644)
						_, _ = w.Write(rawBytes)
						return
					}
				}
			}
		}
	}

	standbyData := generateStandbySnapshot(cleanBase)
	_ = os.WriteFile(samplePath, standbyData, 0644)
	_, _ = w.Write(standbyData)
}

func generateStandbySnapshot(_ string) []byte {
	width, height := 640, 360
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	bg := color.RGBA{R: 11, G: 14, B: 20, A: 255}
	grid := color.RGBA{R: 20, G: 28, B: 42, A: 255}
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			if x%40 == 0 || y%40 == 0 {
				img.Set(x, y, grid)
			} else {
				img.Set(x, y, bg)
			}
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85})
	return buf.Bytes()
}

func extractFrameFromLocalMJPEG(streamID string) ([]byte, error) {
	url := fmt.Sprintf("http://127.0.0.1:8080/api/v1/streams/%s/mjpeg", streamID)
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	buf := make([]byte, 512*1024)
	nTotal := 0
	startIdx := -1

	for nTotal < len(buf) {
		n, err := resp.Body.Read(buf[nTotal:])
		if n > 0 {
			nTotal += n
			if startIdx == -1 {
				startIdx = bytes.Index(buf[:nTotal], []byte{0xFF, 0xD8})
			}
			if startIdx != -1 {
				endIdx := bytes.Index(buf[startIdx+2:nTotal], []byte{0xFF, 0xD9})
				if endIdx != -1 {
					fullEnd := startIdx + 2 + endIdx + 2
					frame := make([]byte, fullEnd-startIdx)
					copy(frame, buf[startIdx:fullEnd])
					return frame, nil
				}
			}
		}
		if err != nil {
			break
		}
	}
	return nil, fmt.Errorf("frame not found in mjpeg stream")
}
