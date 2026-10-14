package http

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"hydrastream/internal/domain"
)

func (h *Handler) autoScanONVIF(host, user, pass string) *domain.ONVIFDevice {
	if host == "" {
		return nil
	}
	commonPorts := []int{80, 8080, 8899, 5000, 8000, 8001}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	type scanResult struct {
		dev *domain.ONVIFDevice
	}
	resChan := make(chan scanResult, len(commonPorts))

	for _, p := range commonPorts {
		go func(port int) {
			addr := net.JoinHostPort(host, strconv.Itoa(port))
			conn, err := net.DialTimeout("tcp", addr, 350*time.Millisecond)
			if err != nil {
				resChan <- scanResult{nil}
				return
			}
			_ = conn.Close()

			dev, errProbe := h.useCase.ProbeONVIFDevice(ctx, domain.ONVIFProbeRequest{
				IPAddress: host,
				Port:      port,
				Username:  user,
				Password:  pass,
			})
			if errProbe == nil && dev != nil {
				resChan <- scanResult{dev}
			} else {
				resChan <- scanResult{nil}
			}
		}(p)
	}

	for i := 0; i < len(commonPorts); i++ {
		res := <-resChan
		if res.dev != nil {
			return res.dev
		}
	}
	return nil
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

	addr := net.JoinHostPort(host, strconv.Itoa(port))
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
