package model

import "time"

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
	Consumers      []Consumer `json:"consumers"`
	CreatedAt      time.Time  `json:"created_at"`
}

// SystemInfo represents basic non-sensitive engine status.
type SystemInfo struct {
	AppName       string            `json:"app_name"`
	Version       string            `json:"version"`
	UptimeSeconds uint64            `json:"uptime_seconds"`
	EngineMode    string            `json:"engine_mode"`
	GPUDetected   bool              `json:"gpu_detected"`
	GPUModel      string            `json:"gpu_model"`
	Features      map[string]bool   `json:"features"`
}

// TopologyNode represents a node in cluster topology readout.
type TopologyNode struct {
	NodeName       string `json:"node_name"`
	NodeIP         string `json:"node_ip"`
	CPUArch        string `json:"cpu_architecture"`
	GPUHardware    string `json:"gpu_hardware"`
	DecoderEngine  string `json:"decoder_engine"`
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
	StreamID       string            `json:"stream_id"`
	IngestionNode  TopologyNode      `json:"ingestion_node"`
	ConsumerRoute  []ConsumerRouting `json:"consumer_routing"`
}
