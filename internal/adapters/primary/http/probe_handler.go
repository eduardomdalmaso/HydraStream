package http

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
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

func isBlockedIPTarget(host string) error {
	host = strings.TrimSpace(host)
	if host == "" {
		return fmt.Errorf("endereço de destino vazio")
	}

	if strings.EqualFold(host, "localhost") || strings.EqualFold(host, "127.0.0.1") || strings.EqualFold(host, "::1") {
		if os.Getenv("ALLOW_LOCAL_PROBE") != "true" {
			return fmt.Errorf("requisições para localhost/loopback bloqueadas por política de segurança SSRF")
		}
	}

	ips, err := net.LookupIP(host)
	if err != nil {
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("falha ao resolver endereço IP de '%s'", host)
		}
		ips = []net.IP{ip}
	}

	for _, ip := range ips {
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
		http.Error(w, `{"error":"invalid JSON body"}`, http.StatusBadRequest)
		return
	}

	protocol := strings.ToLower(req.Protocol)
	switch protocol {
	case "loop", "file":
		h.probeLoopFile(w, req)
	case "onvif":
		h.probeONVIF(w, r, req)
	case "rtmp":
		h.probeRTMP(w, req)
	default:
		h.probeRTSP(w, req)
	}
}
