package application_test

import (
	"context"
	"testing"

	"hydrastream/internal/adapters/secondary/memory"
	"hydrastream/internal/application"
	"hydrastream/internal/domain"
)

func TestStreamServiceRegisterAndGet(t *testing.T) {
	repo := memory.NewStreamRepository()
	service := application.NewStreamService(repo)
	ctx := context.Background()

	newStream := &domain.Stream{
		StreamID:  "cam_backyard_01",
		TenantID:  "tenant_beta",
		SourceURL: "rtsp://mediamtx:8554/cam_backyard_01",
	}

	err := service.RegisterStream(ctx, newStream)
	if err != nil {
		t.Fatalf("expected no error on register, got: %v", err)
	}

	st, err := service.GetStream(ctx, "cam_backyard_01")
	if err != nil {
		t.Fatalf("expected to find stream cam_backyard_01, got: %v", err)
	}
	if st.TenantID != "tenant_beta" {
		t.Errorf("expected tenant_beta, got %s", st.TenantID)
	}
}

func TestStreamServiceDelete(t *testing.T) {
	repo := memory.NewStreamRepository()
	service := application.NewStreamService(repo)
	ctx := context.Background()

	err := service.DeleteStream(ctx, "cam_entrance_01")
	if err != nil {
		t.Fatalf("expected clean delete, got %v", err)
	}

	_, err = service.GetStream(ctx, "cam_entrance_01")
	if err != domain.ErrStreamNotFound {
		t.Errorf("expected ErrStreamNotFound after delete, got %v", err)
	}
}
