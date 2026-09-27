package http

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
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
	proc.SetHideWindow(probeCmd)
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
	proc.SetHideWindow(snapCmd)
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

	if dev.SnapshotURL != "" && strings.HasPrefix(dev.SnapshotURL, "data:image/jpeg;base64,") {
		cleanIP := strings.ReplaceAll(ip, ".", "_")
		b64 := strings.TrimPrefix(dev.SnapshotURL, "data:image/jpeg;base64,")
		if rawBytes, errDec := base64.StdEncoding.DecodeString(b64); errDec == nil && len(rawBytes) > 0 {
			_ = os.MkdirAll("samples", 0755)
			_ = os.WriteFile(fmt.Sprintf("samples/%s.jpg", dev.DeviceID), rawBytes, 0644)
			_ = os.WriteFile(fmt.Sprintf("samples/cam_%s.jpg", cleanIP), rawBytes, 0644)
			_ = os.WriteFile(fmt.Sprintf("samples/%s.jpg", cleanIP), rawBytes, 0644)
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
