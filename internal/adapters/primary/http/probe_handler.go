package http

import (
	"context"
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

	"hydrastream/internal/domain"
)

// StreamProbeRequest defines payload for checking stream connectivity.
type StreamProbeRequest struct {
	Protocol  string `json:"protocol"`
	URL       string `json:"url"`
	IPAddress string `json:"ip_address"`
	Port      int    `json:"port"`
	Username  string `json:"username"`
	Password  string `json:"password"`
}

// StreamProbeResponse holds diagnostic and metadata results from probing a stream.
type StreamProbeResponse struct {
	Online       bool   `json:"online"`
	Error        string `json:"error,omitempty"`
	AuthRequired bool   `json:"auth_required,omitempty"`
	Codec        string `json:"codec,omitempty"`
	Resolution   string `json:"resolution,omitempty"`
	FPS          int    `json:"fps,omitempty"`
	LatencyMs    int64  `json:"latency_ms,omitempty"`
	SnapshotURL  string `json:"snapshot_url,omitempty"`
	Manufacturer string `json:"manufacturer,omitempty"`
	Model        string `json:"model,omitempty"`
	Firmware     string `json:"firmware,omitempty"`
	SerialNumber string `json:"serial_number,omitempty"`
	RTSPURL      string `json:"rtsp_url,omitempty"`
}

// isBlockedIPTarget validates that the target IP is not an internal loopback or cloud metadata IP (SSRF guard).
func isBlockedIPTarget(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("endereço de destino vazio")
	}

	// Block standard loopback and metadata hostnames
	if strings.EqualFold(host, "localhost") || strings.EqualFold(host, "127.0.0.1") || strings.EqualFold(host, "::1") {
		// Allowed in development only if explicit
		if os.Getenv("ALLOW_LOCAL_PROBE") != "true" {
			return fmt.Errorf("requisições para localhost/loopback bloqueadas por política de segurança SSRF")
		}
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		// If cannot resolve hostname, return error
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("falha ao resolver endereço IP de '%s'", host)
		}
		ips = []net.IP{ip}
	}

	for _, ip := range ips {
		// Block Cloud Metadata Service (AWS, GCP, Azure: 169.254.169.254)
		if ip.String() == "169.254.169.254" {
			return fmt.Errorf("acesso ao serviço de metadados cloud (169.254.169.254) estritamente bloqueado")
		}

		if ip.IsLoopback() && os.Getenv("ALLOW_LOCAL_PROBE") != "true" {
			return fmt.Errorf("endereço IP loopback (%s) bloqueado por segurança", ip.String())
		}

		if ip.IsMulticast() || ip.IsUnspecified() {
			return fmt.Errorf("endereço IP inválido ou não roteável (%s)", ip.String())
		}
	}

	return nil
}

// HandleProbeStream tests real-time reachability of RTSP, ONVIF, or RTMP streams.
func (h *Handler) HandleProbeStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var req StreamProbeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online: false,
			Error:  "Corpo da requisição JSON inválido",
		})
		return
	}

	proto := strings.ToUpper(strings.TrimSpace(req.Protocol))
	if proto == "" {
		if strings.HasPrefix(req.URL, "onvif://") {
			proto = "ONVIF"
		} else if strings.HasPrefix(req.URL, "rtmp://") {
			proto = "RTMP"
		} else {
			proto = "RTSP"
		}
	}

	switch proto {
	case "ONVIF":
		h.probeONVIF(w, r, req)
	case "RTMP":
		h.probeRTMP(w, req)
	case "LOOP", "FILE", "FILE_LOOP":
		h.probeLoopFile(w, req)
	default: // RTSP
		h.probeRTSP(w, req)
	}
}

// HandleListSamples lists available sample video files for loop streaming.
func (h *Handler) HandleListSamples(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	var samples []map[string]string

	sampleDirs := []string{
		"samples",
		"/home/hades/Documents/HydraStream/samples",
	}

	for _, sDir := range sampleDirs {
		files, err := os.ReadDir(sDir)
		if err == nil {
			for _, f := range files {
				if !f.IsDir() && (strings.HasSuffix(f.Name(), ".mp4") || strings.HasSuffix(f.Name(), ".mkv") || strings.HasSuffix(f.Name(), ".avi")) {
					samples = append(samples, map[string]string{
						"name": f.Name(),
						"path": filepath.Join("samples", f.Name()),
					})
				}
			}
			if len(samples) > 0 {
				break
			}
		}
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{"samples": samples})
}

func (h *Handler) probeLoopFile(w http.ResponseWriter, req StreamProbeRequest) {
	rawPath := strings.TrimPrefix(req.URL, "file://")
	if rawPath == "" {
		rawPath = req.IPAddress
	}
	if rawPath == "" {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online: false,
			Error:  "Caminho do arquivo de vídeo é obrigatório",
		})
		return
	}

	// Strictly restrict file inspection to samples/ directory (LFI Protection)
	cleanBase := filepath.Base(rawPath)
	if !strings.HasSuffix(cleanBase, ".mp4") && !strings.HasSuffix(cleanBase, ".mkv") && !strings.HasSuffix(cleanBase, ".avi") && !strings.HasSuffix(cleanBase, ".jpg") {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online: false,
			Error:  "Extensão de arquivo de mídia inválida",
		})
		return
	}

	candidates := []string{
		filepath.Clean(filepath.Join("samples", cleanBase)),
		filepath.Clean(filepath.Join("/home/hades/Documents/HydraStream/samples", cleanBase)),
	}

	var foundPath string
	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			foundPath = c
			break
		}
	}

	if foundPath == "" {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online: false,
			Error:  fmt.Sprintf("Arquivo não encontrado na pasta de amostras: %s", cleanBase),
		})
		return
	}

	start := time.Now()
	codec := "H.264"
	res := "1920x1080 Full HD"
	fps := 30

	probeCmd := exec.Command("ffprobe", "-v", "error", "-select_streams", "v:0",
		"-show_entries", "stream=codec_name,width,height,r_frame_rate",
		"-of", "csv=p=0", foundPath)
	if out, err := probeCmd.Output(); err == nil {
		parts := strings.Split(strings.TrimSpace(string(out)), ",")
		if len(parts) >= 1 && parts[0] != "" {
			if strings.Contains(strings.ToLower(parts[0]), "hevc") || strings.Contains(strings.ToLower(parts[0]), "265") {
				codec = "H.265 (HEVC)"
			} else {
				codec = strings.ToUpper(parts[0])
			}
		}
		if len(parts) >= 3 && parts[1] != "" && parts[2] != "" {
			res = fmt.Sprintf("%sx%s", parts[1], parts[2])
		}
	}

	var snapshotURI string
	snapCmd := exec.Command("ffmpeg", "-ss", "00:00:01", "-i", foundPath, "-vframes", "1", "-q:v", "3", "-f", "image2pipe", "-c:v", "mjpeg", "pipe:1")
	if snapBytes, err := snapCmd.Output(); err == nil && len(snapBytes) > 0 {
		snapshotURI = fmt.Sprintf("data:image/jpeg;base64,%s", base64.StdEncoding.EncodeToString(snapBytes))
	}

	_ = json.NewEncoder(w).Encode(StreamProbeResponse{
		Online:       true,
		LatencyMs:    time.Since(start).Milliseconds(),
		Codec:        codec,
		Resolution:   res,
		FPS:          fps,
		SnapshotURL:  snapshotURI,
		Manufacturer: "Hydra Video Loop",
		Model:        filepath.Base(foundPath),
		RTSPURL:      fmt.Sprintf("file://%s", foundPath),
	})
}

func (h *Handler) probeONVIF(w http.ResponseWriter, r *http.Request, req StreamProbeRequest) {
	ip := req.IPAddress
	port := req.Port
	if ip == "" && req.URL != "" {
		if u, err := url.Parse(req.URL); err == nil {
			ip = u.Hostname()
			if p := u.Port(); p != "" {
				fmt.Sscanf(p, "%d", &port)
			}
		}
	}
	if ip == "" {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online: false,
			Error:  "Endereço IP ou URL é obrigatório para probe ONVIF",
		})
		return
	}

	if err := isBlockedIPTarget(ip); err != nil {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online: false,
			Error:  fmt.Sprintf("Bloqueio de Segurança: %v", err),
		})
		return
	}

	if port <= 0 {
		port = 80
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	dev, err := h.useCase.ProbeONVIFDevice(ctx, domain.ONVIFProbeRequest{
		IPAddress: ip,
		Port:      port,
		Username:  req.Username,
		Password:  req.Password,
	})

	if err != nil {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online:    false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     fmt.Sprintf("Falha na comunicação ONVIF com %s:%d: %v", ip, port, err),
		})
		return
	}

	latency := time.Since(start).Milliseconds()

	codec := "H.264"
	res := "1920x1080 Full HD"
	fps := 30

	if len(dev.Profiles) > 0 {
		p := dev.Profiles[0]
		if p.Encoding != "" {
			switch strings.ToUpper(p.Encoding) {
			case "H264", "H.264":
				codec = "H.264"
			case "H265", "H.265", "HEVC":
				codec = "H.265 (HEVC)"
			default:
				codec = p.Encoding
			}
		}
		if p.Width > 0 && p.Height > 0 {
			switch {
			case p.Width == 1280 && p.Height == 720:
				res = "1280x720 HD"
			case p.Width == 1920 && p.Height == 1080:
				res = "1920x1080 Full HD"
			case p.Width == 2560 && p.Height == 1440:
				res = "2560x1440 2K"
			case p.Width == 3840 && p.Height == 2160:
				res = "3840x2160 4K UHD"
			default:
				res = fmt.Sprintf("%dx%d", p.Width, p.Height)
			}
		}
		if p.FPS > 0 {
			fps = p.FPS
		}
	}

	_ = json.NewEncoder(w).Encode(StreamProbeResponse{
		Online:       true,
		LatencyMs:    latency,
		Codec:        codec,
		Resolution:   res,
		FPS:          fps,
		SnapshotURL:  dev.SnapshotURL,
		Manufacturer: dev.Manufacturer,
		Model:        dev.Model,
		Firmware:     dev.FirmwareVersion,
		SerialNumber: dev.SerialNumber,
		RTSPURL:      dev.RTSPURL,
	})
}

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
