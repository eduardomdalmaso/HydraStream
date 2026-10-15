package http

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"hydrastream/internal/adapters/secondary/proc"
	"hydrastream/internal/adapters/secondary/rust"
	"hydrastream/internal/domain"
)

func extractMediaInfoFromFFmpeg(outStr string) (codec string, resolution string, fps int) {
	codec = "H.264"
	resolution = "1920x1080 Full HD"
	fps = 30

	codecRe := regexp.MustCompile(`(?i)Video:\s*([a-zA-Z0-9_-]+)`)
	if match := codecRe.FindStringSubmatch(outStr); len(match) > 1 {
		raw := strings.ToLower(match[1])
		switch {
		case strings.Contains(raw, "h264") || strings.Contains(raw, "avc"):
			codec = "H.264"
		case strings.Contains(raw, "h265") || strings.Contains(raw, "hevc"):
			codec = "H.265 (HEVC)"
		case strings.Contains(raw, "mjpeg"):
			codec = "MJPEG"
		case strings.Contains(raw, "mpeg4"):
			codec = "MPEG-4"
		default:
			codec = strings.ToUpper(raw)
		}
	}

	resRe := regexp.MustCompile(`(?i)(\d{3,4})x(\d{3,4})`)
	if match := resRe.FindStringSubmatch(outStr); len(match) > 2 {
		w, _ := strconv.Atoi(match[1])
		h, _ := strconv.Atoi(match[2])
		if w > 0 && h > 0 {
			switch {
			case w == 1280 && h == 720:
				resolution = "1280x720 HD"
			case w == 1920 && h == 1080:
				resolution = "1920x1080 Full HD"
			case w == 2560 && h == 1440:
				resolution = "2560x1440 2K"
			case w == 3840 && h == 2160:
				resolution = "3840x2160 4K UHD"
			default:
				resolution = fmt.Sprintf("%dx%d", w, h)
			}
		}
	}

	fpsRe := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(?:fps|tbr)`)
	if match := fpsRe.FindStringSubmatch(outStr); len(match) > 1 {
		if f, err := strconv.ParseFloat(match[1], 64); err == nil && f > 0 {
			fps = int(f + 0.5)
		}
	}

	return codec, resolution, fps
}

func (h *Handler) probeRTSP(w http.ResponseWriter, req StreamProbeRequest) {
	targetURL := req.URL
	ip := req.IPAddress
	port := req.Port

	if targetURL == "" && ip != "" {
		if port <= 0 {
			port = 554
		}
		targetURL = fmt.Sprintf("rtsp://%s:%d/live", ip, port)
	}

	host := ip
	if targetURL != "" {
		if u, err := url.Parse(targetURL); err == nil {
			if h := u.Hostname(); h != "" {
				host = h
			}
			if p := u.Port(); p != "" {
				fmt.Sscanf(p, "%d", &port)
			}
		}
	}
	if port <= 0 {
		port = 554
	}
	if host == "" {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online: false,
			Error:  "Endereço IP ou URL RTSP inválida",
		})
		return
	}

	if err := isBlockedIPTarget(host); err != nil {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online: false,
			Error:  fmt.Sprintf("Bloqueio de Segurança: %v", err),
		})
		return
	}

	authUser := req.Username
	authPass := req.Password
	if authUser == "" && targetURL != "" {
		if u, err := url.Parse(targetURL); err == nil && u.User != nil {
			authUser = u.User.Username()
			authPass, _ = u.User.Password()
		}
	}

	onvifChan := make(chan *domain.ONVIFDevice, 1)
	go func() {
		onvifChan <- h.autoScanONVIF(host, authUser, authPass)
	}()

	if req.Username != "" && !strings.Contains(targetURL, "@") {
		auth := fmt.Sprintf("%s:%s@", url.QueryEscape(req.Username), url.QueryEscape(req.Password))
		targetURL = strings.Replace(targetURL, "rtsp://", "rtsp://"+auth, 1)
	}

	start := time.Now()
	ffmpegBin := proc.GetFFmpegPath()
	tmpFile := filepath.Join("samples", fmt.Sprintf("probe_%d.jpg", time.Now().UnixNano()))
	snapCmd := exec.Command(ffmpegBin, "-rtsp_transport", "tcp", "-timeout", "5000000",
		"-i", targetURL, "-update", "1", "-frames:v", "1", "-q:v", "2", "-y", tmpFile)
	proc.SetHideWindow(snapCmd)
	out, errCmd := snapCmd.CombinedOutput()
	outStr := string(out)

	var onvifDev *domain.ONVIFDevice
	select {
	case dev := <-onvifChan:
		onvifDev = dev
	case <-time.After(1 * time.Second):
	}

	if errCmd == nil {
		if data, errRead := os.ReadFile(tmpFile); errRead == nil && len(data) > 0 {
			targetStreamID := req.StreamID
			if targetStreamID == "" {
				targetStreamID = req.CameraID
			}
			_ = os.MkdirAll("samples", 0755)
			if targetStreamID != "" {
				cleanID := filepath.Base(targetStreamID)
				_ = os.WriteFile(filepath.Join("samples", fmt.Sprintf("%s.jpg", cleanID)), data, 0644)
			}
			_ = os.WriteFile(filepath.Join("samples", "latest_probe.jpg"), data, 0644)
			_ = os.Remove(tmpFile)
			b64 := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)
			codec, res, fps := extractMediaInfoFromFFmpeg(outStr)

			resp := StreamProbeResponse{
				Online:      true,
				LatencyMs:   time.Since(start).Milliseconds(),
				Codec:       codec,
				Resolution:  res,
				FPS:         fps,
				SnapshotURL: b64,
				RTSPURL:     targetURL,
			}

			if onvifDev != nil {
				resp.ONVIFAvailable = true
				resp.ONVIFPort = onvifDev.Port
				if resp.Manufacturer == "" {
					resp.Manufacturer = onvifDev.Manufacturer
				}
				if resp.Model == "" {
					resp.Model = onvifDev.Model
				}
				resp.Firmware = onvifDev.FirmwareVersion
				resp.SerialNumber = onvifDev.SerialNumber

				for _, prof := range onvifDev.Profiles {
					resp.Profiles = append(resp.Profiles, fmt.Sprintf("%s (%dx%d @ %d FPS)", prof.Name, prof.Width, prof.Height, prof.FPS))
					if strings.Contains(strings.ToLower(prof.Name), "sub") || (prof.Width > 0 && prof.Width <= 1280 && prof.Width < 1920) {
						if resp.SubstreamURL == "" && prof.RTSPStream != "" {
							resp.SubstreamURL = prof.RTSPStream
						}
					}
				}
			}

			_ = json.NewEncoder(w).Encode(resp)
			return
		}
	}

	// Fallback: Delegate RTSP probing and snapshot to HydraEngine Rust (retina client)
	if rustRes, errRust := rust.ProbeRTSPNativeRust(targetURL, 5*time.Second); errRust == nil && rustRes != nil && rustRes.Online {
		var b64Snap string
		if data, errSnap := rust.CaptureSnapshotNativeRust(targetURL, tmpFile, 6*time.Second); errSnap == nil && len(data) > 0 {
			targetStreamID := req.StreamID
			if targetStreamID == "" {
				targetStreamID = req.CameraID
			}
			_ = os.MkdirAll("samples", 0755)
			if targetStreamID != "" {
				cleanID := filepath.Base(targetStreamID)
				_ = os.WriteFile(filepath.Join("samples", fmt.Sprintf("%s.jpg", cleanID)), data, 0644)
			}
			_ = os.WriteFile(filepath.Join("samples", "latest_probe.jpg"), data, 0644)
			_ = os.Remove(tmpFile)
			b64Snap = "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)
		}

		resp := StreamProbeResponse{
			Online:      true,
			LatencyMs:   rustRes.LatencyMs,
			Codec:       rustRes.Codec,
			Resolution:  rustRes.Resolution,
			FPS:         rustRes.FPS,
			SnapshotURL: b64Snap,
			RTSPURL:     targetURL,
		}
		if onvifDev != nil {
			resp.ONVIFAvailable = true
			resp.ONVIFPort = onvifDev.Port
			if resp.Manufacturer == "" {
				resp.Manufacturer = onvifDev.Manufacturer
			}
			if resp.Model == "" {
				resp.Model = onvifDev.Model
			}
			resp.Firmware = onvifDev.FirmwareVersion
			resp.SerialNumber = onvifDev.SerialNumber
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	latency := time.Since(start).Milliseconds()
	if strings.Contains(outStr, "401") || strings.Contains(strings.ToLower(outStr), "unauthorized") || strings.Contains(outStr, "Authentication") {
		resp := StreamProbeResponse{
			Online:       true,
			AuthRequired: true,
			LatencyMs:    latency,
			Codec:        "H.264",
			Resolution:   "1920x1080",
			FPS:          30,
			Error:        "Autenticação RTSP necessária (401 Unauthorized). Verifique usuário e senha.",
		}

		if onvifDev != nil {
			resp.ONVIFAvailable = true
			resp.ONVIFPort = onvifDev.Port
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	if strings.Contains(outStr, "404") || strings.Contains(strings.ToLower(outStr), "not found") {
		resp := StreamProbeResponse{
			Online:    false,
			LatencyMs: latency,
			Error:     "Caminho do fluxo RTSP não encontrado (404 Not Found)",
		}
		if onvifDev != nil {
			resp.ONVIFAvailable = true
			resp.ONVIFPort = onvifDev.Port
			if onvifDev.RTSPURL != "" {
				resp.RTSPURL = onvifDev.RTSPURL
			}
		}
		_ = json.NewEncoder(w).Encode(resp)
		return
	}

	failResp := StreamProbeResponse{
		Online:    false,
		LatencyMs: latency,
		Error:     fmt.Sprintf("Falha ao conectar via RTSP com %s (timeout ou stream offline)", host),
	}
	if onvifDev != nil {
		failResp.ONVIFAvailable = true
		failResp.ONVIFPort = onvifDev.Port
		if onvifDev.RTSPURL != "" {
			failResp.RTSPURL = onvifDev.RTSPURL
		}
	}
	_ = json.NewEncoder(w).Encode(failResp)
}

