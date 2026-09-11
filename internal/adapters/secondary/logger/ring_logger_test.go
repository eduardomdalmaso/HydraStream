package logger_test

import (
	"context"
	"testing"

	"hydrastream/internal/adapters/secondary/logger"
	"hydrastream/internal/domain"
)

func TestRingLoggerOperations(t *testing.T) {
	rl := logger.NewRingLogger(5) // small capacity for test

	// 1. Record logs
	rl.RecordLog(domain.LogLevelInfo, "ingest", "Stream connected", nil)
	rl.RecordLog(domain.LogLevelWarn, "gpu", "VRAM high occupancy", nil)
	rl.RecordLog(domain.LogLevelError, "nats", "Failed to connect to NATS", nil)

	ctx := context.Background()

	// 2. Query all logs
	res, err := rl.QueryLogs(ctx, domain.LogFilter{})
	if err != nil {
		t.Fatalf("unexpected error querying logs: %v", err)
	}
	if len(res.Logs) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(res.Logs))
	}
	if res.TotalLogs != 3 {
		t.Errorf("expected total logs 3, got %d", res.TotalLogs)
	}
	if res.TotalErrors != 1 {
		t.Errorf("expected total errors 1, got %d", res.TotalErrors)
	}
	if res.TotalWarns != 1 {
		t.Errorf("expected total warns 1, got %d", res.TotalWarns)
	}

	// 3. Filter by Level
	errRes, _ := rl.QueryLogs(ctx, domain.LogFilter{Level: domain.LogLevelError})
	if len(errRes.Logs) != 1 {
		t.Errorf("expected 1 error log, got %d", len(errRes.Logs))
	}
	if errRes.Logs[0].Component != "nats" {
		t.Errorf("expected nats component, got %s", errRes.Logs[0].Component)
	}

	// 4. Error Summary
	summary, err := rl.GetErrorSummary(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.TotalErrors != 1 {
		t.Errorf("expected 1 error, got %d", summary.TotalErrors)
	}
	if summary.ErrorsByComponent["nats"] != 1 {
		t.Errorf("expected 1 error for nats, got %d", summary.ErrorsByComponent["nats"])
	}

	// 5. Test Ring Buffer Overflow Eviction
	for i := 0; i < 10; i++ {
		rl.RecordLog(domain.LogLevelInfo, "shm", "Frame written", nil)
	}
	resOverflow, _ := rl.QueryLogs(ctx, domain.LogFilter{})
	if len(resOverflow.Logs) > 5 {
		t.Errorf("expected max 5 entries in ring buffer, got %d", len(resOverflow.Logs))
	}
}

func TestRingLoggerWriter(t *testing.T) {
	rl := logger.NewRingLogger(10)
	msg := []byte("❌ [RTSP] Stream handshake timeout")
	_, err := rl.Write(msg)
	if err != nil {
		t.Fatalf("write failed: %v", err)
	}

	res, _ := rl.QueryLogs(context.Background(), domain.LogFilter{})
	if len(res.Logs) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(res.Logs))
	}
	if res.Logs[0].Level != domain.LogLevelError {
		t.Errorf("expected ERROR level, got %s", res.Logs[0].Level)
	}
	if res.Logs[0].Component != "rtsp" {
		t.Errorf("expected rtsp component, got %s", res.Logs[0].Component)
	}
}
