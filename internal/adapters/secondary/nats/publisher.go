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

// PublishCameraOffline publishes an instant critical event following CloudEvents v1.0 standard envelope.
func (p *StreamNATSPublisher) PublishCameraOffline(ctx context.Context, tenantID, streamID, errorMsg string) error {
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}
	subject := fmt.Sprintf("hydra.v1.%s.cameras.%s.events", tenantID, streamID)

	payload, err := json.Marshal(map[string]interface{}{
		"specversion": "1.0",
		"id":          fmt.Sprintf("evt_off_%s_%d", streamID, time.Now().UnixNano()),
		"source":      "hydrastream/ingest-engine",
		"type":        "system.camera.offline",
		"subject":     streamID,
		"time":        time.Now().UTC().Format(time.RFC3339Nano),
		"tenant_id":   tenantID,
		"category":    "SYSTEM_EVENT",
		"severity":    "critical",
		"data": map[string]interface{}{
			"camera_id":   streamID,
			"camera_name": streamID,
			"status":      "offline",
			"reason":      errorMsg,
			"timestamp":   time.Now().UTC().Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		return err
	}

	_ = p.nc.Publish(fmt.Sprintf("hydra.v1.%s.events.system.camera.offline", tenantID), payload)
	return p.nc.Publish(subject, payload)
}

// PublishCameraOnline publishes an instant recovery event following CloudEvents v1.0 standard envelope.
func (p *StreamNATSPublisher) PublishCameraOnline(ctx context.Context, tenantID, streamID string) error {
	if tenantID == "" {
		tenantID = "00000000-0000-0000-0000-000000000001"
	}
	subject := fmt.Sprintf("hydra.v1.%s.cameras.%s.events", tenantID, streamID)

	payload, err := json.Marshal(map[string]interface{}{
		"specversion": "1.0",
		"id":          fmt.Sprintf("evt_on_%s_%d", streamID, time.Now().UnixNano()),
		"source":      "hydrastream/ingest-engine",
		"type":        "system.camera.online",
		"subject":     streamID,
		"time":        time.Now().UTC().Format(time.RFC3339Nano),
		"tenant_id":   tenantID,
		"category":    "SYSTEM_EVENT",
		"severity":    "low",
		"data": map[string]interface{}{
			"camera_id":   streamID,
			"camera_name": streamID,
			"status":      "online",
			"reason":      "RTSP stream connected",
			"timestamp":   time.Now().UTC().Format(time.RFC3339Nano),
		},
	})
	if err != nil {
		return err
	}

	_ = p.nc.Publish(fmt.Sprintf("hydra.v1.%s.events.system.camera.online", tenantID), payload)
	return p.nc.Publish(subject, payload)
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

// Close gracefully closes the NATS connection.
func (p *StreamNATSPublisher) Close() {
	if p.nc != nil {
		p.nc.Close()
	}
}
