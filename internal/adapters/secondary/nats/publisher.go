package nats

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go"
)

// StreamNATSPublisher publishes stream lifecycle and real-time telemetry from HydraStream to NATS.
type StreamNATSPublisher struct {
	nc *nats.Conn
}

// NewStreamNATSPublisher connects to NATS at the provided URL.
func NewStreamNATSPublisher(url string) (*StreamNATSPublisher, error) {
	opts := []nats.Option{
		nats.Name("HydraStream-IngestEngine"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2 * time.Second),
		nats.PingInterval(20 * time.Second),
		nats.MaxPingsOutstanding(3),
	}

	nc, err := nats.Connect(url, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS from HydraStream: %w", err)
	}

	log.Printf("📡 [HydraStream] Connected to NATS Event Mesh at %s\n", url)
	return &StreamNATSPublisher{nc: nc}, nil
}

// PublishTelemetry publishes FPS, bitrate and codec metrics for a camera.
func (p *StreamNATSPublisher) PublishTelemetry(ctx context.Context, tenantID, streamID string, fps float64, bitrateKbps int, codec string) error {
	subject := fmt.Sprintf("hydra.v1.%s.cameras.%s.telemetry", tenantID, streamID)

	payload, err := json.Marshal(map[string]interface{}{
		"stream_id":    streamID,
		"tenant_id":    tenantID,
		"fps":          fps,
		"bitrate_kbps": bitrateKbps,
		"codec":        codec,
		"timestamp":    time.Now().UnixMilli(),
		"status":       "online",
	})
	if err != nil {
		return err
	}

	return p.nc.Publish(subject, payload)
}

// PublishStreamStatus publishes connection/disconnection lifecycle events.
func (p *StreamNATSPublisher) PublishStreamStatus(ctx context.Context, tenantID, streamID, status, errorMsg string) error {
	subject := fmt.Sprintf("hydra.v1.%s.cameras.%s.status", tenantID, streamID)

	payload, err := json.Marshal(map[string]interface{}{
		"stream_id":  streamID,
		"tenant_id":  tenantID,
		"status":     status,
		"error":      errorMsg,
		"timestamp":  time.Now().UnixMilli(),
	})
	if err != nil {
		return err
	}

	return p.nc.Publish(subject, payload)
}

// Close gracefully closes the NATS connection.
func (p *StreamNATSPublisher) Close() {
	if p.nc != nil {
		p.nc.Close()
	}
}
