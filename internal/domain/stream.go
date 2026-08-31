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

// IngestStats represents real-time stream ingestion telemetry.
type IngestStats struct {
	StreamID      string    `json:"stream_id"`
	Status        string    `json:"status"` // "connecting", "streaming", "reconnecting", "stopped"
	IngestFPS     float64   `json:"ingest_fps"`
	BitrateKbps   float64   `json:"bitrate_kbps"`
	FramesTotal   uint64    `json:"frames_total"`
	BytesTotal    uint64    `json:"bytes_total"`
	LastFrameTime time.Time `json:"last_frame_time"`
	ErrorMsg      string    `json:"error_msg,omitempty"`
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

// ClusterNode represents a compute worker node in the topology cluster.
type ClusterNode struct {
	NodeName      string  `json:"node_name"`
	NodeIP        string  `json:"node_ip"`
	CPUArch       string  `json:"cpu_architecture"`
	GPUHardware   string  `json:"gpu_hardware"`
	DecoderEngine string  `json:"decoder_engine"`
	Status        string  `json:"status"`
	LoadPercent   float64 `json:"load_percent"`
	MemoryPercent float64 `json:"memory_percent"`
	ActiveStreams string  `json:"active_streams"`
	NodeType      string  `json:"node_type"`
}

// ClusterTopology represents cluster hardware topology.
type ClusterTopology struct {
	StreamID      string            `json:"stream_id"`
	IngestionNode TopologyNode      `json:"ingestion_node"`
	ConsumerRoute []ConsumerRouting `json:"consumer_routing"`
	Nodes         []ClusterNode     `json:"nodes"`
}

// ControlPanelTelemetry aggregates real runtime metrics for the live dashboard.
type ControlPanelTelemetry struct {
	HealthScore        float64   `json:"health_score"`
	SLAStatus          string    `json:"sla_status"`
	ActiveClusterNodes string    `json:"active_cluster_nodes"`
	NodesSummary       string    `json:"nodes_summary"`
	AvgDecodeLatencyMs float64   `json:"avg_decode_latency_ms"`
	DecoderEngineName  string    `json:"decoder_engine_name"`
	POSIXShmOccupancy  float64   `json:"posix_shm_occupancy"`
	ShmLockFreeStatus  string    `json:"shm_lock_free_status"`
	PeakBandwidthMbps  float64   `json:"peak_bandwidth_mbps"`
	BandwidthHistory   []float64 `json:"bandwidth_history"`
	LatencyHistory     []float64 `json:"latency_history"`
	ActiveStreamsCount int       `json:"active_streams_count"`
	TotalIngestFPS     float64   `json:"total_ingest_fps"`
}

// ChaosInjection represents a real chaos experiment request.
type ChaosInjection struct {
	ExperimentType string  `json:"experiment_type"` // "packet_drop", "disconnect", "gpu_stall", "shm_overflow"
	Intensity      float64 `json:"intensity"`       // e.g. 25 (%) for packet drop
	StreamID       string  `json:"stream_id"`
}

// ChaosResult represents the real-time measured outcome of the chaos injection.
type ChaosResult struct {
	ExperimentType string    `json:"experiment_type"`
	Status         string    `json:"status"` // "injected", "recovered"
	Message        string    `json:"message"`
	RecoveryMs     float64   `json:"recovery_ms"`
	FramesDropped  uint64    `json:"frames_dropped"`
	JitterDeltaMs  float64   `json:"jitter_delta_ms"`
	Timestamp      time.Time `json:"timestamp"`
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
