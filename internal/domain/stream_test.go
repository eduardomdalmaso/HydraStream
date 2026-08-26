package domain_test

import (
	"testing"

	"hydrastream/internal/domain"
)

func TestStreamValidate(t *testing.T) {
	invalidStream := &domain.Stream{}
	if err := invalidStream.Validate(); err == nil {
		t.Errorf("expected error for empty stream_id, got nil")
	}

	validStream := &domain.Stream{StreamID: "cam_01"}
	if err := validStream.Validate(); err != nil {
		t.Errorf("expected no error for valid stream_id, got: %v", err)
	}
}

func TestStreamSetDefaults(t *testing.T) {
	st := &domain.Stream{
		StreamID:       "cam_test",
		CPULoadPercent: 10.0,
		GPUMemoryMB:    100.0,
		NetworkKbps:    5000.0,
	}
	st.SetDefaults()

	if st.Status != "online" {
		t.Errorf("expected status 'online', got '%s'", st.Status)
	}
	if st.Resolution != "1920x1080" {
		t.Errorf("expected resolution '1920x1080', got '%s'", st.Resolution)
	}
	if st.Codec != "h264" {
		t.Errorf("expected codec 'h264', got '%s'", st.Codec)
	}
	if st.IngestFPS != 30.0 {
		t.Errorf("expected ingest FPS 30.0, got %f", st.IngestFPS)
	}

	expectedScore := (10.0 * 2.0) + (100.0 / 10.0) + (5000.0 / 1000.0) // 20 + 10 + 5 = 35
	if st.ResourceScore != expectedScore {
		t.Errorf("expected resource score %f, got %f", expectedScore, st.ResourceScore)
	}
}

func TestStreamUpdateConsumer(t *testing.T) {
	st := &domain.Stream{
		StreamID: "cam_01",
		Consumers: []domain.Consumer{
			{AnalyticType: "lpr", TargetFPS: 2.0, OutputFormat: "json"},
		},
	}

	// Update existing consumer
	st.UpdateConsumer("lpr", 5.0, "shm")
	if len(st.Consumers) != 1 {
		t.Fatalf("expected 1 consumer, got %d", len(st.Consumers))
	}
	if st.Consumers[0].TargetFPS != 5.0 || st.Consumers[0].OutputFormat != "shm" {
		t.Errorf("consumer not updated correctly: %+v", st.Consumers[0])
	}

	// Add new consumer
	st.UpdateConsumer("face_rec", 15.0, "tensor")
	if len(st.Consumers) != 2 {
		t.Fatalf("expected 2 consumers, got %d", len(st.Consumers))
	}
	if st.Consumers[1].AnalyticType != "face_rec" || st.Consumers[1].TargetFPS != 15.0 {
		t.Errorf("new consumer not added correctly: %+v", st.Consumers[1])
	}
}
