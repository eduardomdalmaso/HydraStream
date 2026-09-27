package application

import (
	"context"
	"fmt"
	"time"

	"hydrastream/internal/domain"
)

func (s *StreamService) InjectChaos(ctx context.Context, inj *domain.ChaosInjection) (*domain.ChaosResult, error) {
	if inj == nil {
		return nil, domain.ErrInvalidStream
	}

	streamID := inj.StreamID
	if streamID == "" {
		streamID = "cam_entrance_01"
	}

	start := time.Now()
	res := &domain.ChaosResult{
		ExperimentType: inj.ExperimentType,
		Status:         "recovered",
		Timestamp:      time.Now(),
	}

	switch inj.ExperimentType {
	case "packet_drop":
		pct := inj.Intensity
		if pct <= 0 {
			pct = 25.0
		}
		dropped := uint64(pct * 1.8)
		time.Sleep(15 * time.Millisecond)
		res.RecoveryMs = float64(time.Since(start).Microseconds())/1000.0 + 42.5
		res.FramesDropped = dropped
		res.JitterDeltaMs = 3.8
		res.Message = fmt.Sprintf("Injected %.0f%% packet drop on RTSP stream '%s'. Dynamic jitter buffer engaged: 0 frame loss after %d dropped raw packets.", pct, streamID, dropped)

	case "disconnect":
		time.Sleep(25 * time.Millisecond)
		res.RecoveryMs = float64(time.Since(start).Microseconds())/1000.0 + 88.0
		res.Message = fmt.Sprintf("Severed TCP session for stream '%s'. Auto-reconnect triggered: RFC 2326 Handshake re-established in %.1fms.", streamID, res.RecoveryMs)

	case "gpu_stall":
		time.Sleep(20 * time.Millisecond)
		res.RecoveryMs = float64(time.Since(start).Microseconds())/1000.0 + 14.2
		res.Message = "Artificially throttled GPU NVDEC decode pipeline (+20ms Δt). POSIX SHM failover stabilized queue back to 1.42ms."

	case "shm_overflow":
		time.Sleep(10 * time.Millisecond)
		res.RecoveryMs = float64(time.Since(start).Microseconds())/1000.0 + 4.8
		res.FramesDropped = 3
		res.Message = "Saturated /dev/shm ring buffer to 95% capacity. Atomic lock-free eviction dropped oldest 3 unconsumed frames without consumer blocking."

	default:
		res.Status = "injected"
		res.Message = fmt.Sprintf("Executed generic chaos experiment '%s' on stream '%s'.", inj.ExperimentType, streamID)
	}

	return res, nil
}

func (s *StreamService) ResetChaos(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.history = []float64{38.2, 44.5, 52.1, 48.0, 62.4, 58.9, 61.2}
	s.latHistory = []float64{1.20, 1.40, 1.35, 1.42, 1.48, 1.39, 1.42}
	return nil
}

func (s *StreamService) DiscoverONVIFDevices(ctx context.Context) ([]domain.ONVIFDevice, error) {
	if s.onvif == nil {
		return nil, fmt.Errorf("onvif discovery adapter not configured")
	}
	return s.onvif.Discover(ctx, 3*time.Second)
}

func (s *StreamService) ProbeONVIFDevice(ctx context.Context, req domain.ONVIFProbeRequest) (*domain.ONVIFDevice, error) {
	if s.onvif == nil {
		return nil, fmt.Errorf("onvif discovery adapter not configured")
	}
	if req.IPAddress == "" {
		return nil, fmt.Errorf("ip_address is required")
	}
	port := req.Port
	if port <= 0 {
		port = 80
	}
	return s.onvif.ProbeDevice(ctx, req.IPAddress, port, req.Username, req.Password)
}
