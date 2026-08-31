package ports

import (
	"context"

	"hydrastream/internal/domain"
)

// StreamIngestor defines the secondary port for managing live video stream ingestion sessions.
type StreamIngestor interface {
	StartIngest(ctx context.Context, stream *domain.Stream) error
	StopIngest(ctx context.Context, streamID string) error
	GetIngestStats(ctx context.Context, streamID string) (*domain.IngestStats, error)
	ListActiveIngests(ctx context.Context) ([]*domain.IngestStats, error)
}
