package domain

import (
	"errors"
	"time"
)

// Consumer represents a downstream analytics consumer subscribed to a stream.
type Consumer struct {
	AnalyticType string    `json:"analytic_type"`
	TargetFPS    float64   `json:"target_fps"`
	ActualFPS    float64   `json:"actual_fps,omitempty"`
	OutputFormat string    `json:"output_format"`
	SHMKey       string    `json:"shm_key,omitempty"`
	DroppedCount uint64    `json:"dropped_frames,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

// Stream represents an ingested RTSP camera or video file stream.
type Stream struct {
	TenantID       string     `json:"tenant_id"`
	StreamID       string     `json:"stream_id"`
	SourceURL      string     `json:"source_url"`
	DecodingEngine string     `json:"decoding_engine"`
	Status         string     `json:"status"` // "online", "offline", "reconnecting"
	Resolution     string     `json:"resolution"`
	Codec          string     `json:"codec"`
	IngestFPS      float64    `json:"ingest_fps"`
	NetworkKbps    float64    `json:"network_kbps"`
	CPULoadPercent float64    `json:"cpu_load_percent"`
	GPUMemoryMB    float64    `json:"gpu_memory_mb"`
	DecodeLatency  float64    `json:"decode_latency_ms"`
	ResourceScore  float64    `json:"resource_score"`
	Consumers      []Consumer `json:"consumers"`
	CreatedAt      time.Time  `json:"created_at"`
}

// SystemInfo represents basic non-sensitive engine status.
type SystemInfo struct {
	AppName       string          `json:"app_name"`
	Version       string          `json:"version"`
	UptimeSeconds uint64          `json:"uptime_seconds"`
	EngineMode    string          `json:"engine_mode"`
	GPUDetected   bool            `json:"gpu_detected"`
	GPUModel      string          `json:"gpu_model"`
	Features      map[string]bool `json:"features"`
}

// TopologyNode represents a node in cluster topology readout.
type TopologyNode struct {
	NodeName      string `json:"node_name"`
	NodeIP        string `json:"node_ip"`
	CPUArch       string `json:"cpu_architecture"`
	GPUHardware   string `json:"gpu_hardware"`
	DecoderEngine string `json:"decoder_engine"`
}

// ConsumerRouting represents cross-node or node-local transport routing.
type ConsumerRouting struct {
	Analytic       string `json:"analytic"`
	TargetNode     string `json:"target_node"`
	SameNode       bool   `json:"same_node"`
	TransportUsed  string `json:"transport_used"`
	TargetHardware string `json:"target_hardware"`
}

// ClusterTopology represents cluster hardware topology.
type ClusterTopology struct {
	StreamID      string            `json:"stream_id"`
	IngestionNode TopologyNode      `json:"ingestion_node"`
	ConsumerRoute []ConsumerRouting `json:"consumer_routing"`
}

// Validate checks business rules for stream creation/update.
func (s *Stream) Validate() error {
	if s.StreamID == "" {
		return errors.New("stream_id is required")
	}
	return nil
}

// CalculateResourceScore computes synthetic resource consumption score.
func (s *Stream) CalculateResourceScore() float64 {
	return (s.CPULoadPercent * 2.0) + (s.GPUMemoryMB / 10.0) + (s.NetworkKbps / 1000.0)
}

// SetDefaults sets sensible domain defaults for empty fields.
func (s *Stream) SetDefaults() {
	if s.CreatedAt.IsZero() {
		s.CreatedAt = time.Now()
	}
	if s.Status == "" {
		s.Status = "online"
	}
	if s.Resolution == "" {
		s.Resolution = "1920x1080"
	}
	if s.Codec == "" {
		s.Codec = "h264"
	}
	if s.IngestFPS == 0 {
		s.IngestFPS = 30.0
	}
	if s.ResourceScore == 0 {
		s.ResourceScore = s.CalculateResourceScore()
	}
}

// UpdateConsumer updates an existing consumer or appends a new consumer.
func (s *Stream) UpdateConsumer(analyticType string, targetFPS float64, format string) {
	for i := range s.Consumers {
		if s.Consumers[i].AnalyticType == analyticType {
			s.Consumers[i].TargetFPS = targetFPS
			if format != "" {
				s.Consumers[i].OutputFormat = format
			}
			return
		}
	}
	// Append if consumer doesn't exist
	s.Consumers = append(s.Consumers, Consumer{
		AnalyticType: analyticType,
		TargetFPS:    targetFPS,
		ActualFPS:    targetFPS,
		OutputFormat: format,
		CreatedAt:    time.Now(),
	})
}
