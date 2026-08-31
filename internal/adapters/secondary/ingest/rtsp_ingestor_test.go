package ingest

import (
	"context"
	"testing"
	"time"

	"hydrastream/internal/domain"
)

func TestRTSPIngestorSyntheticLifecycle(t *testing.T) {
	ingestor := NewRTSPIngestor()
	ctx := context.Background()

	st := &domain.Stream{
		StreamID:  "test_synthetic_stream",
		SourceURL: "synthetic://benchmark_loop",
		IngestFPS: 30.0,
	}

	err := ingestor.StartIngest(ctx, st)
	if err != nil {
		t.Fatalf("failed to start ingest: %v", err)
	}

	// Wait for a few synthetic frames to pump
	time.Sleep(100 * time.Millisecond)

	stats, err := ingestor.GetIngestStats(ctx, st.StreamID)
	if err != nil {
		t.Fatalf("failed to get stats: %v", err)
	}

	if stats.StreamID != st.StreamID {
		t.Errorf("expected streamID %s, got %s", st.StreamID, stats.StreamID)
	}
	if stats.FramesTotal == 0 {
		t.Error("expected frames total > 0")
	}

	list, err := ingestor.ListActiveIngests(ctx)
	if err != nil {
		t.Fatalf("failed to list active ingests: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 active ingest, got %d", len(list))
	}

	err = ingestor.StopIngest(ctx, st.StreamID)
	if err != nil {
		t.Fatalf("failed to stop ingest: %v", err)
	}
}
