package ports

import (
	"context"

	"hydrastream/internal/domain"
)

// StreamRepository defines secondary port interface for stream persistence.
type StreamRepository interface {
	Save(ctx context.Context, stream *domain.Stream) error
	FindByID(ctx context.Context, streamID string) (*domain.Stream, error)
	ListFiltered(ctx context.Context, searchQuery, tenantFilter, sortBy string, page, limit int) ([]*domain.Stream, int, error)
	ListAll(ctx context.Context) ([]*domain.Stream, error)
	Delete(ctx context.Context, streamID string) error
	UpdateConsumerFPS(ctx context.Context, streamID, analyticType string, targetFPS float64, format string) error
	UptimeSeconds(ctx context.Context) uint64
}
