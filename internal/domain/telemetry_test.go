package domain_test

import (
	"testing"
	"time"

	"hydrastream/internal/domain"
)

func TestTelemetryDomainModels(t *testing.T) {
	now := time.Now()

	logEntry := domain.LogEntry{
		ID:        1,
		Timestamp: now,
		Level:     domain.LogLevelInfo,
		Component: "ingest",
		Message:   "RTSP stream session connected",
	}

	if logEntry.Level != domain.LogLevelInfo {
		t.Errorf("expected level INFO, got %s", logEntry.Level)
	}

	health := domain.SystemHealth{
		Status:        "healthy",
		Service:       "hydrastream-dataplane",
		Version:       "1.0.0",
		UptimeSeconds: 120,
		StartedAt:     now.Add(-120 * time.Second),
		Services: domain.ServiceStatus{
			RTSPIngestor:    "ONLINE",
			MediaMTXRelay:   "ONLINE",
			POSIXSHMBuffers: "ONLINE",
			NATSEventMesh:   "ONLINE",
			GPUAcceleration: "ONLINE",
		},
		Streams: domain.StreamsSummary{
			TotalStreams:   2,
			OnlineStreams:  2,
			TotalConsumers: 4,
			TotalIngestFPS: 60.0,
		},
		Timestamp: now,
	}

	if health.Status != "healthy" {
		t.Errorf("expected healthy status, got %s", health.Status)
	}

	hw := domain.HardwareTelemetry{
		Host: domain.HostHardware{
			Hostname: "hydra-node",
			OS:       "linux",
			CPUCores: 16,
		},
		GPU: domain.GPUHardwareTelemetry{
			Detected:    true,
			Model:       "NVIDIA GeForce RTX 5090",
			TotalVRAMMB: 32607,
		},
		Timestamp: now,
	}

	if !hw.GPU.Detected {
		t.Errorf("expected GPU detected true")
	}
}
