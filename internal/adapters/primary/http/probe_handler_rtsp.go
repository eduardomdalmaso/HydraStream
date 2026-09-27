package http

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
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

	if req.Username != "" && !strings.Contains(targetURL, "@") {
		auth := fmt.Sprintf("%s:%s@", url.QueryEscape(req.Username), url.QueryEscape(req.Password))
		targetURL = strings.Replace(targetURL, "rtsp://", "rtsp://"+auth, 1)
	}

	start := time.Now()
	ffmpegBin := getFFmpegPath()
	tmpFile := filepath.Join("samples", fmt.Sprintf("probe_%d.jpg", time.Now().UnixNano()))
	snapCmd := exec.Command(ffmpegBin, "-rtsp_transport", "tcp", "-timeout", "5000000",
		"-i", targetURL, "-update", "1", "-frames:v", "1", "-q:v", "2", "-y", tmpFile)
	proc.SetHideWindow(snapCmd)
	out, errCmd := snapCmd.CombinedOutput()
	outStr := string(out)

	if errCmd == nil {
		if data, errRead := os.ReadFile(tmpFile); errRead == nil && len(data) > 0 {
			_ = os.Remove(tmpFile)
			b64 := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)
			codec, res, fps := extractMediaInfoFromFFmpeg(outStr)

			_ = json.NewEncoder(w).Encode(StreamProbeResponse{
				Online:      true,
				LatencyMs:   time.Since(start).Milliseconds(),
				Codec:       codec,
				Resolution:  res,
				FPS:         fps,
				SnapshotURL: b64,
				RTSPURL:     targetURL,
			})
			return
		}
	}

	latency := time.Since(start).Milliseconds()
	if strings.Contains(outStr, "401") || strings.Contains(strings.ToLower(outStr), "unauthorized") || strings.Contains(outStr, "Authentication") {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online:       true,
			AuthRequired: true,
			LatencyMs:    latency,
			Codec:        "H.264",
			Resolution:   "1920x1080",
			FPS:          30,
			Error:        "Autenticação RTSP necessária (401 Unauthorized). Verifique usuário e senha.",
		})
		return
	}

	if strings.Contains(outStr, "404") || strings.Contains(strings.ToLower(outStr), "not found") {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online:    false,
			LatencyMs: latency,
			Error:     "Caminho do fluxo RTSP não encontrado (404 Not Found)",
		})
		return
	}

	_ = json.NewEncoder(w).Encode(StreamProbeResponse{
		Online:    false,
		LatencyMs: latency,
		Error:     fmt.Sprintf("Falha ao conectar via RTSP com %s (timeout ou stream offline)", host),
	})
}

func (h *Handler) probeRTMP(w http.ResponseWriter, req StreamProbeRequest) {
	host := req.IPAddress
	port := req.Port
	if host == "" && req.URL != "" {
		if u, err := url.Parse(req.URL); err == nil {
			host = u.Hostname()
			if p := u.Port(); p != "" {
				fmt.Sscanf(p, "%d", &port)
			}
		}
	}
	if host == "" {
		host = "127.0.0.1"
	}
	if port <= 0 {
		port = 1935
	}

	if err := isBlockedIPTarget(host); err != nil {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online: false,
			Error:  fmt.Sprintf("Bloqueio de Segurança: %v", err),
		})
		return
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online:    false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     fmt.Sprintf("Falha ao conectar na porta RTMP %s: servidor offline", addr),
		})
		return
	}
	defer conn.Close()

	_ = json.NewEncoder(w).Encode(StreamProbeResponse{
		Online:     true,
		LatencyMs:  time.Since(start).Milliseconds(),
		Codec:      "H.264",
		Resolution: "1920x1080",
		FPS:        30,
	})
}
