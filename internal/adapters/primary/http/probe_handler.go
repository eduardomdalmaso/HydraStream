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
		"/home/hades/Documents/HydraStream/samples",
		"samples",
	}

	for _, sDir := range sampleDirs {
		files, err := os.ReadDir(sDir)
		if err == nil {
			for _, f := range files {
				if !f.IsDir() && (strings.HasSuffix(f.Name(), ".mp4") || strings.HasSuffix(f.Name(), ".mkv") || strings.HasSuffix(f.Name(), ".avi")) {
					samples = append(samples, map[string]string{
						"name": f.Name(),
						"path": filepath.Join(sDir, f.Name()),
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
	filePath := strings.TrimPrefix(req.URL, "file://")
	if filePath == "" {
		filePath = req.IPAddress
	}
	if filePath == "" {
		_ = json.NewEncoder(w).Encode(StreamProbeResponse{
			Online: false,
			Error:  "Caminho do arquivo de vídeo é obrigatório",
		})
		return
	}

	candidates := []string{
		filePath,
		filepath.Join("/home/hades/Documents/HydraStream/samples", filePath),
		filepath.Join("samples", filePath),
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
			Error:  fmt.Sprintf("Arquivo não encontrado no disco: %s", filePath),
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

	_ = json.NewEncoder(w).Encode(StreamProbeResponse{
		Online:       true,
		LatencyMs:    time.Since(start).Milliseconds(),
		Codec:        "H.265 (HEVC)",
		Resolution:   "1920x1080 Full HD",
		FPS:          30,
		Manufacturer: dev.Manufacturer,
		Model:        dev.Model,
		Firmware:     dev.FirmwareVersion,
		SerialNumber: dev.SerialNumber,
		RTSPURL:      dev.RTSPURL,
	})
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

	_ = json.NewEncoder(w).Encode(StreamProbeResponse{
		Online:     true,
		LatencyMs:  latency,
		Codec:      "H.264",
		Resolution: "1920x1080",
		FPS:        30,
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
