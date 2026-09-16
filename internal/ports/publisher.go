package ports

import (
	"context"
	"time"
)

// RecordingEventPublisher defines port for publishing recording segment events to NATS/Event Mesh.
type RecordingEventPublisher interface {
	PublishRecordingSegment(ctx context.Context, tenantID, cameraID, recordingMode, s3Key string, startTime, endTime time.Time, durationSec int, fileSizeBytes int64) error
}
