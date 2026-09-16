package memory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"hydrastream/internal/domain"
	"hydrastream/internal/ports"
)

// FragmentRepository implements an in-memory & disk-backed repository for recording fragments.
type FragmentRepository struct {
	mu        sync.RWMutex
	fragments map[string]map[string]*domain.RecordingFragment // streamID -> fragmentID -> Fragment
	baseDir   string
}

// NewFragmentRepository creates a new FragmentRepository instance.
func NewFragmentRepository(baseDir string) *FragmentRepository {
	if baseDir == "" {
		baseDir = "recordings"
	}
	_ = os.MkdirAll(baseDir, 0755)

	return &FragmentRepository{
		fragments: make(map[string]map[string]*domain.RecordingFragment),
		baseDir:   baseDir,
	}
}

// Save stores the metadata and writes data to disk if provided.
func (r *FragmentRepository) Save(ctx context.Context, frag *domain.RecordingFragment, data []byte) error {
	if err := frag.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	streamMap, exists := r.fragments[frag.StreamID]
	if !exists {
		streamMap = make(map[string]*domain.RecordingFragment)
		r.fragments[frag.StreamID] = streamMap
	}

	cleanStreamID := filepath.Base(filepath.Clean(frag.StreamID))
	if cleanStreamID == "." || cleanStreamID == "/" || cleanStreamID == "" {
		return domain.ErrInvalidFragmentStreamID
	}
	frag.StreamID = cleanStreamID

	if frag.ID == "" {
		frag.ID = fmt.Sprintf("frag_%s_%d", frag.StreamID, time.Now().UnixNano())
	}
	frag.ID = filepath.Base(filepath.Clean(frag.ID))

	streamDir := filepath.Join(r.baseDir, cleanStreamID)
	_ = os.MkdirAll(streamDir, 0755)

	ext := ".mp4"
	filePath := filepath.Join(streamDir, fmt.Sprintf("%s%s", frag.ID, ext))


	if len(data) > 0 {
		if err := os.WriteFile(filePath, data, 0644); err != nil {
			return fmt.Errorf("failed to write fragment binary to disk: %w", err)
		}
		frag.FileSizeBytes = int64(len(data))
		frag.StoragePath = filePath
	} else if frag.StoragePath == "" {
		frag.StoragePath = filePath
	}

	streamMap[frag.ID] = frag
	return nil
}

// FindByID retrieves a fragment and its binary payload.
func (r *FragmentRepository) FindByID(ctx context.Context, streamID, fragmentID string) (*domain.RecordingFragment, []byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	streamMap, exists := r.fragments[streamID]
	if !exists {
		return nil, nil, domain.ErrFragmentNotFound
	}

	frag, found := streamMap[fragmentID]
	if !found {
		return nil, nil, domain.ErrFragmentNotFound
	}

	var data []byte
	if frag.StoragePath != "" {
		if b, err := os.ReadFile(frag.StoragePath); err == nil {
			data = b
		}
	}

	return frag, data, nil
}

// ListByStream returns recording fragments for a stream ordered by StartTime descending.
func (r *FragmentRepository) ListByStream(ctx context.Context, streamID string, start, end time.Time, limit int) ([]*domain.RecordingFragment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	streamMap, exists := r.fragments[streamID]
	if !exists {
		return []*domain.RecordingFragment{}, nil
	}

	results := make([]*domain.RecordingFragment, 0, len(streamMap))
	for _, f := range streamMap {
		if !start.IsZero() && f.EndTime.Before(start) {
			continue
		}
		if !end.IsZero() && f.StartTime.After(end) {
			continue
		}
		results = append(results, f)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].StartTime.After(results[j].StartTime)
	})

	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}

// Delete removes a fragment from memory and disk.
func (r *FragmentRepository) Delete(ctx context.Context, streamID, fragmentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	streamMap, exists := r.fragments[streamID]
	if !exists {
		return domain.ErrFragmentNotFound
	}

	frag, found := streamMap[fragmentID]
	if !found {
		return domain.ErrFragmentNotFound
	}

	if frag.StoragePath != "" {
		_ = os.Remove(frag.StoragePath)
	}

	delete(streamMap, fragmentID)
	return nil
}

var _ ports.FragmentRepository = (*FragmentRepository)(nil)
