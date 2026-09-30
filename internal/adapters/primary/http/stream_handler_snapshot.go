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
	"hydrastream/internal/domain"
)

func getFFmpegPath() string {
	if p, err := exec.LookPath("ffmpeg"); err == nil {
		return p
	}
	if p, err := exec.LookPath("ffmpeg.exe"); err == nil {
		return p
	}
	userProfile := os.Getenv("USERPROFILE")
	if userProfile != "" {
		matches, _ := filepath.Glob(filepath.Join(userProfile, "AppData", "Local", "Microsoft", "WinGet", "Packages", "*", "*", "bin", "ffmpeg.exe"))
		if len(matches) > 0 {
			return matches[0]
		}
		matches, _ = filepath.Glob(filepath.Join(userProfile, "AppData", "Local", "Microsoft", "WinGet", "Packages", "*", "bin", "ffmpeg.exe"))
		if len(matches) > 0 {
			return matches[0]
		}
	}
	return "ffmpeg"
}

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

	isRefresh := r != nil && (r.URL.Query().Get("refresh") == "true" || r.URL.Query().Get("force") == "true")

	if !isRefresh {
		if data, err := os.ReadFile(samplePath); err == nil && len(data) > 0 {
			_, _ = w.Write(data)
			return
		}
		altMatches, _ := filepath.Glob(fmt.Sprintf("samples/*%s*.jpg", cleanBase))
		for _, alt := range altMatches {
			if data, err := os.ReadFile(alt); err == nil && len(data) > 0 {
				_, _ = w.Write(data)
				return
			}
		}
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
				cmd := exec.Command("sqlite3", dbp, fmt.Sprintf("SELECT rtsp_url FROM cameras WHERE id = '%s' OR id LIKE '%%%s%%' LIMIT 1;", cleanBase, cleanBase))
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
		fmt.Sprintf("rtsp://localhost:8554/%s_sub", cleanBase),
		fmt.Sprintf("rtsp://localhost:8554/%s", cleanBase),
	)

	ffmpegBin := getFFmpegPath()
	for _, u := range urlsToTry {
		snapCmd := exec.Command(ffmpegBin, "-rtsp_transport", "tcp", "-stimeout", "3000000",
			"-i", u, "-update", "1", "-frames:v", "1", "-q:v", "2", "-y", samplePath)
		proc.SetHideWindow(snapCmd)
		if err := snapCmd.Run(); err == nil {
			if data, errRead := os.ReadFile(samplePath); errRead == nil && len(data) > 0 {
				_, _ = w.Write(data)
				return
			}
		}
	}

	if data, err := os.ReadFile(samplePath); err == nil && len(data) > 0 {
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
					if rawBytes, errDec := base64.StdEncoding.DecodeString(b64); errDec == nil && len(rawBytes) > 0 {
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

func generateStandbySnapshot(streamID string) []byte {
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
