package application

import (
	"context"
	"fmt"
	"math"
	"time"

	"hydrastream/internal/adapters/secondary/memory"
	"hydrastream/internal/domain"
	"hydrastream/internal/ports"
)

func (s *StreamService) SetFragmentRepository(fragRepo ports.FragmentRepository) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fragRepo = fragRepo
}

func (s *StreamService) SetRecordingPublisher(pub ports.RecordingEventPublisher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.recPublisher = pub
}

func (s *StreamService) SaveRecordingFragment(ctx context.Context, frag *domain.RecordingFragment, data []byte) error {
	if s.fragRepo == nil {
		s.fragRepo = memory.NewFragmentRepository("recordings")
	}

	if err := frag.Validate(); err != nil {
		s.RecordLog(domain.LogLevelWarn, "recordings", fmt.Sprintf("Recording fragment validation failed: %v", err), nil)
		return err
	}

	if err := s.fragRepo.Save(ctx, frag, data); err != nil {
		s.RecordLog(domain.LogLevelError, "recordings", fmt.Sprintf("Failed to save fragment for stream '%s': %v", frag.StreamID, err), nil)
		return err
	}

	s.RecordLog(domain.LogLevelInfo, "recordings", fmt.Sprintf("Saved recording fragment '%s' for stream '%s' (%.1fs, %d bytes)", frag.ID, frag.StreamID, frag.DurationSeconds, frag.FileSizeBytes), nil)

	s.mu.Lock()
	pub := s.recPublisher
	s.mu.Unlock()

	if pub != nil {
		durationSec := int(math.Max(1.0, math.Round(frag.DurationSeconds)))
		_ = pub.PublishRecordingSegment(ctx, frag.TenantID, frag.StreamID, frag.RecordingMode, frag.StoragePath, frag.StartTime, frag.EndTime, durationSec, frag.FileSizeBytes)
	}

	return nil
}

func (s *StreamService) GetRecordingFragment(ctx context.Context, streamID, fragmentID string) (*domain.RecordingFragment, []byte, error) {
	if s.fragRepo == nil {
		return nil, nil, domain.ErrFragmentNotFound
	}
	return s.fragRepo.FindByID(ctx, streamID, fragmentID)
}

func (s *StreamService) ListRecordingFragments(ctx context.Context, streamID string, start, end time.Time, limit int) ([]*domain.RecordingFragment, error) {
	if s.fragRepo == nil {
		return []*domain.RecordingFragment{}, nil
	}
	return s.fragRepo.ListByStream(ctx, streamID, start, end, limit)
}
