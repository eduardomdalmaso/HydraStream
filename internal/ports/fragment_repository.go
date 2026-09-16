package ports

import (
	"context"
	"time"

	"hydrastream/internal/domain"
)

// FragmentRepository defines the secondary port for storing and querying recording fragments.
type FragmentRepository interface {
	Save(ctx context.Context, frag *domain.RecordingFragment, data []byte) error
	FindByID(ctx context.Context, streamID, fragmentID string) (*domain.RecordingFragment, []byte, error)
	ListByStream(ctx context.Context, streamID string, start, end time.Time, limit int) ([]*domain.RecordingFragment, error)
	Delete(ctx context.Context, streamID, fragmentID string) error
}
