package memory

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"hydrastream/internal/domain"
	"hydrastream/internal/ports"
)

// StreamRepository implements ports.StreamRepository using a thread-safe map.
type StreamRepository struct {
	mu        sync.RWMutex
	streams   map[string]*domain.Stream
	startTime time.Time
}

// NewStreamRepository creates a new in-memory StreamRepository pre-seeded with sample data.
func NewStreamRepository() *StreamRepository {
	repo := &StreamRepository{
		streams:   make(map[string]*domain.Stream),
		startTime: time.Now(),
	}

	// Seed sample stream
	st := &domain.Stream{
		TenantID:       "tenant_company_alpha",
		StreamID:       "cam_entrance_01",
		SourceURL:      "synthetic://benchmark_4k_gpu",
		DecodingEngine: "nvidia_nvdec",
		Status:         "online",
		Resolution:     "1920x1080",
		Codec:          "h264",
		IngestFPS:      30.0,
		CreatedAt:      time.Now(),
		Consumers: []domain.Consumer{
			{
				AnalyticType: "lpr_ocr",
				TargetFPS:    2.0,
				ActualFPS:    2.0,
				OutputFormat: "shm_numpy",
				SHMKey:       "/hs_shm_tenant_company_alpha_cam_entrance_01_lpr",
				CreatedAt:    time.Now(),
			},
			{
				AnalyticType: "object_tracker",
				TargetFPS:    15.0,
				ActualFPS:    14.98,
				OutputFormat: "cuda_ipc_tensor",
				CreatedAt:    time.Now(),
			},
		},
	}
	st.SetDefaults()
	repo.streams[st.StreamID] = st

	return repo
}

func (r *StreamRepository) Save(ctx context.Context, st *domain.Stream) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	st.SetDefaults()
	r.streams[st.StreamID] = st
	return nil
}

func (r *StreamRepository) FindByID(ctx context.Context, streamID string) (*domain.Stream, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	st, ok := r.streams[streamID]
	if !ok {
		return nil, domain.ErrStreamNotFound
	}
	return st, nil
}

func (r *StreamRepository) ListFiltered(ctx context.Context, searchQuery, tenantFilter, sortBy string, page, limit int) ([]*domain.Stream, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	searchQuery = strings.ToLower(strings.TrimSpace(searchQuery))
	tenantFilter = strings.ToLower(strings.TrimSpace(tenantFilter))

	var filtered []*domain.Stream
	for _, st := range r.streams {
		if st.ResourceScore == 0 {
			st.ResourceScore = st.CalculateResourceScore()
		}

		// Tenant filter
		if tenantFilter != "" && strings.ToLower(st.TenantID) != tenantFilter {
			continue
		}
		// Search query filter
		if searchQuery != "" {
			match := strings.Contains(strings.ToLower(st.StreamID), searchQuery) ||
				strings.Contains(strings.ToLower(st.SourceURL), searchQuery) ||
				strings.Contains(strings.ToLower(st.Codec), searchQuery)
			if !match {
				continue
			}
		}
		filtered = append(filtered, st)
	}

	// Sort
	sort.Slice(filtered, func(i, j int) bool {
		switch sortBy {
		case "latency_desc":
			return filtered[i].DecodeLatency > filtered[j].DecodeLatency
		case "fps_desc":
			return filtered[i].IngestFPS > filtered[j].IngestFPS
		case "stream_id_asc":
			return filtered[i].StreamID < filtered[j].StreamID
		default: // "resource_desc"
			return filtered[i].ResourceScore > filtered[j].ResourceScore
		}
	})

	total := len(filtered)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	startIndex := (page - 1) * limit
	if startIndex >= total {
		return []*domain.Stream{}, total, nil
	}

	endIndex := startIndex + limit
	if endIndex > total {
		endIndex = total
	}

	return filtered[startIndex:endIndex], total, nil
}

func (r *StreamRepository) ListAll(ctx context.Context) ([]*domain.Stream, error) {
	streams, _, err := r.ListFiltered(ctx, "", "", "resource_desc", 1, 1000)
	return streams, err
}

func (r *StreamRepository) Delete(ctx context.Context, streamID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.streams[streamID]; !ok {
		return domain.ErrStreamNotFound
	}
	delete(r.streams, streamID)
	return nil
}

func (r *StreamRepository) UpdateConsumerFPS(ctx context.Context, streamID, analyticType string, targetFPS float64, format string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	st, ok := r.streams[streamID]
	if !ok {
		return domain.ErrStreamNotFound
	}

	st.UpdateConsumer(analyticType, targetFPS, format)
	return nil
}

func (r *StreamRepository) UptimeSeconds(ctx context.Context) uint64 {
	return uint64(time.Since(r.startTime).Seconds())
}

// Ensure interface compliance
var _ ports.StreamRepository = (*StreamRepository)(nil)
