package ports

import (
	"context"

	"hydrastream/internal/domain"
)

// StreamUseCase defines primary port interface for application services.
type StreamUseCase interface {
	RegisterStream(ctx context.Context, stream *domain.Stream) error
	GetStream(ctx context.Context, streamID string) (*domain.Stream, error)
	ListStreams(ctx context.Context, searchQuery, tenantFilter, sortBy string, page, limit int) ([]*domain.Stream, int, error)
	DeleteStream(ctx context.Context, streamID string) error
	UpdateConsumer(ctx context.Context, streamID, analyticType string, targetFPS float64, format string) error
	GetClusterTopology(ctx context.Context, streamID string) (*domain.ClusterTopology, error)
	GetSystemInfo(ctx context.Context) (*domain.SystemInfo, error)
	GetControlPanelTelemetry(ctx context.Context) (*domain.ControlPanelTelemetry, error)
	GetIngestStats(ctx context.Context, streamID string) (*domain.IngestStats, error)
}
