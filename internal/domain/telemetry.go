package domain

import (
	"time"
)

// LogLevel defines standardized severity levels.
type LogLevel string

const (
	LogLevelDebug    LogLevel = "DEBUG"
	LogLevelInfo     LogLevel = "INFO"
	LogLevelWarn     LogLevel = "WARN"
	LogLevelError    LogLevel = "ERROR"
	LogLevelCritical LogLevel = "CRITICAL"
)

// LogEntry represents a structured log event in the telemetry ring buffer.
type LogEntry struct {
	ID        uint64                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Level     LogLevel               `json:"level"`
	Component string                 `json:"component"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// LogFilter defines parameters for querying in-memory logs.
type LogFilter struct {
	Level     LogLevel  `json:"level,omitempty"`
	Component string    `json:"component,omitempty"`
	Since     time.Time `json:"since,omitempty"`
	Limit     int       `json:"limit,omitempty"`
}

// LogQueryResult represents the response when querying telemetry logs.
type LogQueryResult struct {
	TotalLogs   int        `json:"total_logs"`
	TotalErrors int        `json:"total_errors"`
	TotalWarns  int        `json:"total_warns"`
	FilterLevel string     `json:"filter_level,omitempty"`
	Logs        []LogEntry `json:"logs"`
}

// ErrorSummary aggregates recent engine anomalies and error frequencies.
type ErrorSummary struct {
	TotalErrors        int            `json:"total_errors"`
	TotalWarnings      int            `json:"total_warnings"`
	LastErrorTime      *time.Time     `json:"last_error_time,omitempty"`
	LastErrorMessage   string         `json:"last_error_message,omitempty"`
	ErrorsByComponent  map[string]int `json:"errors_by_component"`
	RecentErrors       []LogEntry     `json:"recent_errors"`
}

// HostHardware holds CPU and operating system telemetry.
type HostHardware struct {
	Hostname       string  `json:"hostname"`
	OS             string  `json:"os"`
	Architecture   string  `json:"architecture"`
	CPUCores       int     `json:"cpu_cores"`
	Goroutines     int     `json:"goroutines"`
	HostTotalRAMMB float64 `json:"host_total_ram_mb"`
	HostUsedRAMMB  float64 `json:"host_used_ram_mb"`
	HostRAMUsagePct float64 `json:"host_ram_usage_pct"`
}

// MemoryTelemetry holds Go runtime memory allocation details.
type MemoryTelemetry struct {
	AllocMB      float64 `json:"alloc_mb"`
	TotalAllocMB float64 `json:"total_alloc_mb"`
	SysMB        float64 `json:"sys_mb"`
	HeapAllocMB  float64 `json:"heap_alloc_mb"`
	HeapSysMB    float64 `json:"heap_sys_mb"`
	NumGC        uint32  `json:"num_gc"`
	GCPauseMs    float64 `json:"gc_pause_ms"`
}

// GPUHardwareTelemetry holds real NVIDIA NVML / GPU metrics.
type GPUHardwareTelemetry struct {
	Detected          bool    `json:"detected"`
	Model             string  `json:"model"`
	TotalVRAMMB       float64 `json:"total_vram_mb"`
	UsedVRAMMB        float64 `json:"used_vram_mb"`
	FreeVRAMMB        float64 `json:"free_vram_mb"`
	VRAMUsagePct      float64 `json:"vram_usage_pct"`
	GPUUtilPct        float64 `json:"gpu_util_pct"`
	MemoryUtilPct     float64 `json:"memory_util_pct"`
	TempCelsius       float64 `json:"temp_celsius"`
	PowerWatts        float64 `json:"power_watts"`
	DriverVersion     string  `json:"driver_version"`
	EngineName        string  `json:"engine_name"`
	CUDAIPCSupported  bool    `json:"cuda_ipc_supported"`
}

// StorageSHMTelemetry holds POSIX /dev/shm shared memory metrics.
type StorageSHMTelemetry struct {
	MountPoint        string  `json:"mount_point"`
	TotalBytes        uint64  `json:"total_bytes"`
	UsedBytes         uint64  `json:"used_bytes"`
	FreeBytes         uint64  `json:"free_bytes"`
	OccupancyPct      float64 `json:"occupancy_pct"`
	RingBufferStatus  string  `json:"ring_buffer_status"`
	LockFreeMode      bool    `json:"lock_free_mode"`
}

// HardwareTelemetry aggregates all system hardware metrics.
type HardwareTelemetry struct {
	Host      HostHardware         `json:"host"`
	Memory    MemoryTelemetry      `json:"memory"`
	GPU       GPUHardwareTelemetry `json:"gpu"`
	SHM       StorageSHMTelemetry  `json:"posix_shm"`
	Timestamp time.Time            `json:"timestamp"`
}

// ServiceStatus holds individual engine subsystem readiness.
type ServiceStatus struct {
	RTSPIngestor    string `json:"rtsp_ingestor"`
	MediaMTXRelay   string `json:"mediamtx_relay"`
	POSIXSHMBuffers string `json:"posix_shm_buffers"`
	NATSEventMesh   string `json:"nats_event_mesh"`
	GPUAcceleration string `json:"gpu_acceleration"`
}

// StreamsSummary provides quick stream and consumer counters.
type StreamsSummary struct {
	TotalStreams    int     `json:"total_streams"`
	OnlineStreams   int     `json:"online_streams"`
	OfflineStreams  int     `json:"offline_streams"`
	TotalConsumers  int     `json:"total_consumers"`
	TotalIngestFPS  float64 `json:"total_ingest_fps"`
	PeakBandwidthMb float64 `json:"peak_bandwidth_mbps"`
}

// SystemHealth aggregates overall health check telemetry.
type SystemHealth struct {
	Status        string          `json:"status"` // "healthy", "degraded", "unhealthy"
	Service       string          `json:"service"`
	Version       string          `json:"version"`
	UptimeSeconds uint64          `json:"uptime_seconds"`
	StartedAt     time.Time       `json:"started_at"`
	Services      ServiceStatus   `json:"services"`
	Streams       StreamsSummary  `json:"streams"`
	ErrorCount    int             `json:"error_count"`
	WarningCount  int             `json:"warning_count"`
	Timestamp     time.Time       `json:"timestamp"`
}

// UnifiedTelemetry combines health, hardware, and summary telemetry in one payload.
type UnifiedTelemetry struct {
	Health    SystemHealth      `json:"health"`
	Hardware  HardwareTelemetry `json:"hardware"`
	Errors    ErrorSummary      `json:"error_summary"`
	Timestamp time.Time         `json:"timestamp"`
}
