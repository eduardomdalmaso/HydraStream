package shm

import (
	"testing"
)

func TestSHMAdapterMockRoundtrip(t *testing.T) {
	mgr := NewManager()
	streamID := "test_go_stream_01"

	path, err := mgr.WriteMockHeader(streamID, 1920, 1080, 16)
	if err != nil {
		t.Fatalf("failed to write mock header: %v", err)
	}
	defer mgr.Cleanup(streamID)

	if path == "" {
		t.Fatal("expected non-empty path")
	}

	hdr, err := mgr.InspectStreamSHM(streamID)
	if err != nil {
		t.Fatalf("failed to inspect SHM: %v", err)
	}

	if hdr.Magic != HydraMagic {
		t.Errorf("expected magic %x, got %x", HydraMagic, hdr.Magic)
	}
	if hdr.Width != 1920 || hdr.Height != 1080 {
		t.Errorf("expected 1920x1080, got %dx%d", hdr.Width, hdr.Height)
	}
	if hdr.WriteSequence != 42 {
		t.Errorf("expected write sequence 42, got %d", hdr.WriteSequence)
	}

	seq, err := mgr.ReadLatestSequence(streamID)
	if err != nil {
		t.Fatalf("failed to read sequence: %v", err)
	}
	if seq != 42 {
		t.Errorf("expected sequence 42, got %d", seq)
	}
}
