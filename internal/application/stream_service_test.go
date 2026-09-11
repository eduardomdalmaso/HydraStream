package application_test

import (
	"context"
	"testing"

	"hydrastream/internal/adapters/secondary/logger"
	"hydrastream/internal/adapters/secondary/memory"
	"hydrastream/internal/application"
	"hydrastream/internal/domain"
)

func TestStreamServiceRegisterAndGet(t *testing.T) {
	repo := memory.NewStreamRepository()
	service := application.NewStreamService(repo, nil, nil)
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
	service := application.NewStreamService(repo, nil, nil)
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

func TestStreamServiceTelemetry(t *testing.T) {
	repo := memory.NewStreamRepository()
	ringLog := logger.NewRingLogger(100)
	service := application.NewStreamService(repo, nil, nil, ringLog)
	ctx := context.Background()

	// 1. Health
	health, err := service.GetHealth(ctx)
	if err != nil {
		t.Fatalf("expected health without error, got: %v", err)
	}
	if health.Service != "hydrastream-dataplane" {
		t.Errorf("expected hydrastream-dataplane service, got %s", health.Service)
	}
	if health.Status != "healthy" {
		t.Errorf("expected healthy status, got %s", health.Status)
	}

	// 2. Hardware
	hw, err := service.GetHardwareTelemetry(ctx)
	if err != nil {
		t.Fatalf("expected hardware telemetry without error, got: %v", err)
	}
	if hw.Host.CPUCores <= 0 {
		t.Errorf("expected positive CPU cores, got %d", hw.Host.CPUCores)
	}

	// 3. Unified Telemetry
	unified, err := service.GetUnifiedTelemetry(ctx)
	if err != nil {
		t.Fatalf("expected unified telemetry without error, got: %v", err)
	}
	if unified.Health.Service == "" {
		t.Errorf("expected populated health in unified telemetry")
	}

	// 4. Logs & Errors
	service.RecordLog(domain.LogLevelError, "test", "Simulated error event", nil)
	logs, err := service.GetLogs(ctx, domain.LogFilter{Level: domain.LogLevelError})
	if err != nil {
		t.Fatalf("expected logs query without error, got: %v", err)
	}
	if len(logs.Logs) == 0 {
		t.Fatalf("expected at least 1 error log")
	}
	if logs.Logs[0].Message != "Simulated error event" {
		t.Errorf("expected matching message, got %s", logs.Logs[0].Message)
	}

	errSummary, err := service.GetErrorSummary(ctx)
	if err != nil {
		t.Fatalf("expected error summary without error, got: %v", err)
	}
	if errSummary.TotalErrors == 0 {
		t.Errorf("expected positive total errors in summary")
	}
}
