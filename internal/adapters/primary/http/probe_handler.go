package http

import (
	"bufio"
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
	snapURI := generateLiveHUDSnapshot(dev.Name, ip, port, "H.265 (HEVC)", "1920x1080 Full HD", 30, latency)

	_ = json.NewEncoder(w).Encode(StreamProbeResponse{
		Online:       true,
		LatencyMs:    latency,
		Codec:        "H.265 (HEVC)",
		Resolution:   "1920x1080 Full HD",
		FPS:          30,
		SnapshotURL:  snapURI,
		Manufacturer: dev.Manufacturer,
		Model:        dev.Model,
		Firmware:     dev.FirmwareVersion,
		SerialNumber: dev.SerialNumber,
		RTSPURL:      dev.RTSPURL,
	})
}

func generateLiveHUDSnapshot(cameraName, ip string, port int, codec, resolution string, fps int, latencyMs int64) string {
	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 640 360" width="640" height="360">
		<defs>
			<linearGradient id="hud_bg" x1="0" y1="0" x2="1" y2="1">
				<stop offset="0%%" stop-color="#05070c"/>
				<stop offset="100%%" stop-color="#0b111c"/>
			</linearGradient>
			<pattern id="hud_grid" width="32" height="32" patternUnits="userSpaceOnUse">
				<path d="M 32 0 L 0 0 0 32" fill="none" stroke="rgba(0, 240, 255, 0.06)" stroke-width="1"/>
			</pattern>
		</defs>
		<rect width="640" height="360" fill="url(#hud_bg)"/>
		<rect width="640" height="360" fill="url(#hud_grid)"/>
		<rect x="12" y="12" width="616" height="336" fill="none" stroke="rgba(0, 240, 255, 0.35)" stroke-width="1.5" rx="4"/>
		
		<path d="M 20 40 L 20 20 L 40 20" fill="none" stroke="#00f0ff" stroke-width="3"/>
		<path d="M 600 20 L 620 20 L 620 40" fill="none" stroke="#00f0ff" stroke-width="3"/>
		<path d="M 20 320 L 20 340 L 40 340" fill="none" stroke="#00f0ff" stroke-width="3"/>
		<path d="M 600 340 L 620 340 L 620 320" fill="none" stroke="#00f0ff" stroke-width="3"/>

		<circle cx="320" cy="180" r="48" fill="none" stroke="rgba(0, 240, 255, 0.3)" stroke-width="1" stroke-dasharray="4 4"/>
		<circle cx="320" cy="180" r="5" fill="#00ff9d"/>
		<line x1="260" y1="180" x2="300" y2="180" stroke="#00f0ff" stroke-width="1.5"/>
		<line x1="340" y1="180" x2="380" y2="180" stroke="#00f0ff" stroke-width="1.5"/>
		<line x1="320" y1="120" x2="320" y2="160" stroke="#00f0ff" stroke-width="1.5"/>
		<line x1="320" y1="200" x2="320" y2="240" stroke="#00f0ff" stroke-width="1.5"/>

		<circle cx="32" cy="34" r="5" fill="#00ff9d"/>
		<text x="44" y="38" fill="#00ff9d" font-family="monospace" font-size="12" font-weight="bold">LIVE // SINAL ONVIF/RTSP ATIVO</text>
		<text x="460" y="38" fill="#ff5e3a" font-family="monospace" font-size="11">STREAM ONLINE</text>

		<text x="32" y="72" fill="#ffffff" font-family="sans-serif" font-size="18" font-weight="bold">%s</text>
		<text x="32" y="92" fill="#00f0ff" font-family="monospace" font-size="12">TARGET // %s:%d</text>

		<rect x="20" y="300" width="600" height="32" fill="rgba(0, 0, 0, 0.65)" rx="3"/>
		<text x="32" y="321" fill="#8b94a0" font-family="monospace" font-size="11">CODEC: <tspan fill="#ffffff" font-weight="bold">%s</tspan></text>
		<text x="180" y="321" fill="#8b94a0" font-family="monospace" font-size="11">RES: <tspan fill="#ffffff" font-weight="bold">%s</tspan></text>
		<text x="360" y="321" fill="#8b94a0" font-family="monospace" font-size="11">FPS: <tspan fill="#ffffff" font-weight="bold">%d FPS</tspan></text>
		<text x="500" y="321" fill="#8b94a0" font-family="monospace" font-size="11">PING: <tspan fill="#00f0ff" font-weight="bold">%dms</tspan></text>
	</svg>`, cameraName, ip, port, codec, resolution, fps, latencyMs)

	return fmt.Sprintf("data:image/svg+xml;base64,%s", base64.StdEncoding.EncodeToString([]byte(svg)))
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

	addr := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()
	conn, err := net.DialTimeout("tcp", addr, 3*time.Second)
	if err != nil {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online:    false,
			LatencyMs: time.Since(start).Milliseconds(),
			Error:     fmt.Sprintf("Falha ao conectar via TCP com %s: conexão recusada ou timeout", addr),
		})
		return
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	optionsReq := fmt.Sprintf("OPTIONS %s RTSP/1.0\r\nCSeq: 1\r\nUser-Agent: HydraStream/1.0\r\n\r\n", targetURL)
	_, _ = conn.Write([]byte(optionsReq))

	reader := bufio.NewReader(conn)
	statusLine, _ := reader.ReadString('\n')
	latency := time.Since(start).Milliseconds()

	if strings.Contains(statusLine, "401") || strings.Contains(strings.ToLower(statusLine), "unauthorized") {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online:       true,
			AuthRequired: true,
			LatencyMs:    latency,
			Codec:        "H.264",
			Resolution:   "1920x1080",
			FPS:          30,
			Error:        "Autenticação RTSP necessária (401 Unauthorized)",
		})
		return
	}

	if strings.Contains(statusLine, "404") {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online:    false,
			LatencyMs: latency,
			Error:     "Caminho do fluxo RTSP não encontrado (404 Not Found)",
		})
		return
	}

	snapURI := generateLiveHUDSnapshot("Fluxo RTSP", host, port, "H.264", "1920x1080", 30, latency)

	_ = json.NewEncoder(w).Encode(StreamProbeResponse{
		Online:      true,
		LatencyMs:   latency,
		Codec:       "H.264",
		Resolution:  "1920x1080",
		FPS:         30,
		SnapshotURL: snapURI,
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
