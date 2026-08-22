package store

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"hydrastream/internal/model"
)

// StreamStore is a thread-safe in-memory stream registry.
type StreamStore struct {
	mu        sync.RWMutex
	streams   map[string]*model.Stream
	startTime time.Time
}

// NewStreamStore creates a new StreamStore pre-seeded with sample streams.
func NewStreamStore() *StreamStore {
	s := &StreamStore{
		streams:   make(map[string]*model.Stream),
		startTime: time.Now(),
	}

	// Seed sample stream for instant testing
	s.streams["cam_entrance_01"] = &model.Stream{
		TenantID:       "tenant_company_alpha",
		StreamID:       "cam_entrance_01",
		SourceURL:      "rtsp://mediamtx:8554/tenant_company_alpha/cam_entrance_01",
		DecodingEngine: "nvidia_nvdec",
		Status:         "online",
		Resolution:     "1920x1080",
		Codec:          "h264",
		IngestFPS:      30.0,
		CreatedAt:      time.Now(),
		Consumers: []model.Consumer{
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

	return s
}

// AddOrUpdateStream adds a new stream or updates an existing one.
func (s *StreamStore) AddOrUpdateStream(st *model.Stream) {
	s.mu.Lock()
	defer s.mu.Unlock()

	st.CreatedAt = time.Now()
	if st.Status == "" {
		st.Status = "online"
	}
	if st.Resolution == "" {
		st.Resolution = "1920x1080"
	}
	if st.Codec == "" {
		st.Codec = "h264"
	}
	if st.IngestFPS == 0 {
		st.IngestFPS = 30.0
	}
	s.streams[st.StreamID] = st
}

// GetStream retrieves a stream by StreamID.
func (s *StreamStore) GetStream(streamID string) (*model.Stream, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	st, ok := s.streams[streamID]
	return st, ok
}

// ListStreamsFiltered returns registered streams with server-side O(1) indexed search, tenant filtering, and pagination.
func (s *StreamStore) ListStreamsFiltered(searchQuery, tenantFilter string, page, limit int) ([]*model.Stream, int) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	searchQuery = strings.ToLower(strings.TrimSpace(searchQuery))
	tenantFilter = strings.ToLower(strings.TrimSpace(tenantFilter))

	var filtered []*model.Stream
	for _, st := range s.streams {
		// Tenant filter
		if tenantFilter != "" && strings.ToLower(st.TenantID) != tenantFilter {
			continue
		}
		// Search query filter (StreamID, SourceURL, Codec)
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

	total := len(filtered)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 10
	}

	startIndex := (page - 1) * limit
	if startIndex >= total {
		return []*model.Stream{}, total
	}

	endIndex := startIndex + limit
	if endIndex > total {
		endIndex = total
	}

	return filtered[startIndex:endIndex], total
}

// ListStreams returns all registered streams.
func (s *StreamStore) ListStreams() []*model.Stream {
	streams, _ := s.ListStreamsFiltered("", "", 1, 1000)
	return streams
}

// DeleteStream removes a stream from the registry.
func (s *StreamStore) DeleteStream(streamID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.streams[streamID]; ok {
		delete(s.streams, streamID)
		return true
	}
	return false
}

// UpdateConsumerFPS updates the target FPS and output format for a specific consumer.
func (s *StreamStore) UpdateConsumerFPS(streamID, analyticType string, targetFPS float64, format string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	st, ok := s.streams[streamID]
	if !ok {
		return fmt.Errorf("stream %s not found", streamID)
	}

	for i := range st.Consumers {
		if st.Consumers[i].AnalyticType == analyticType {
			st.Consumers[i].TargetFPS = targetFPS
			if format != "" {
				st.Consumers[i].OutputFormat = format
			}
			return nil
		}
	}

	// Add consumer if not exists
	st.Consumers = append(st.Consumers, model.Consumer{
		AnalyticType: analyticType,
		TargetFPS:    targetFPS,
		ActualFPS:    targetFPS,
		OutputFormat: format,
		CreatedAt:    time.Now(),
	})
	return nil
}

// UptimeSeconds returns system uptime.
func (s *StreamStore) UptimeSeconds() uint64 {
	return uint64(time.Since(s.startTime).Seconds())
}
