package logger

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"hydrastream/internal/domain"
	"hydrastream/internal/ports"
)

const defaultRingCapacity = 1000

// RingLogger is an in-memory thread-safe ring-buffer log and error collector.
type RingLogger struct {
	mu         sync.RWMutex
	capacity   int
	entries    []domain.LogEntry
	nextID     uint64
	totalLogs  uint64
	totalWarns uint64
	totalErrs  uint64
	lastError  *domain.LogEntry
}

// NewRingLogger creates a new RingLogger instance with default capacity.
func NewRingLogger(capacity ...int) *RingLogger {
	capSize := defaultRingCapacity
	if len(capacity) > 0 && capacity[0] > 0 {
		capSize = capacity[0]
	}

	return &RingLogger{
		capacity: capSize,
		entries:  make([]domain.LogEntry, 0, capSize),
	}
}

// Write implements io.Writer to intercept standard Go log output.
func (r *RingLogger) Write(p []byte) (n int, err error) {
	raw := strings.TrimSpace(string(p))
	if raw == "" {
		return len(p), nil
	}

	// Determine log level and component from message content
	level := domain.LogLevelInfo
	component := "system"

	upper := strings.ToUpper(raw)
	if strings.Contains(upper, "ERROR") || strings.Contains(raw, "❌") || strings.Contains(upper, "FAILED") || strings.Contains(upper, "FATAL") {
		level = domain.LogLevelError
	} else if strings.Contains(upper, "WARN") || strings.Contains(raw, "⚠️") {
		level = domain.LogLevelWarn
	} else if strings.Contains(upper, "DEBUG") {
		level = domain.LogLevelDebug
	}

	// Extract [Component] if present, e.g. [HydraStream], [RTSP], [ONVIF], [NATS], [MediaMTX]
	if idxStart := strings.Index(raw, "["); idxStart != -1 {
		if idxEnd := strings.Index(raw[idxStart:], "]"); idxEnd != -1 {
			tag := strings.ToLower(strings.TrimSpace(raw[idxStart+1 : idxStart+idxEnd]))
			if tag != "" {
				component = tag
			}
		}
	}

	r.RecordLog(level, component, raw, nil)
	return len(p), nil
}

// RecordLog appends a structured log entry into the ring buffer.
func (r *RingLogger) RecordLog(level domain.LogLevel, component, message string, details map[string]interface{}) {
	id := atomic.AddUint64(&r.nextID, 1)
	atomic.AddUint64(&r.totalLogs, 1)

	switch level {
	case domain.LogLevelWarn:
		atomic.AddUint64(&r.totalWarns, 1)
	case domain.LogLevelError, domain.LogLevelCritical:
		atomic.AddUint64(&r.totalErrs, 1)
	}

	entry := domain.LogEntry{
		ID:        id,
		Timestamp: time.Now().UTC(),
		Level:     level,
		Component: component,
		Message:   message,
		Details:   details,
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if level == domain.LogLevelError || level == domain.LogLevelCritical {
		entryCopy := entry
		r.lastError = &entryCopy
	}

	if len(r.entries) >= r.capacity {
		// Evict oldest item in ring
		r.entries = append(r.entries[1:], entry)
	} else {
		r.entries = append(r.entries, entry)
	}
}

// QueryLogs filters and returns matching log records.
func (r *RingLogger) QueryLogs(ctx context.Context, filter domain.LogFilter) (*domain.LogQueryResult, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	limit := filter.Limit
	if limit <= 0 || limit > 500 {
		limit = 50
	}

	filterLevel := strings.ToUpper(string(filter.Level))
	filterComp := strings.ToLower(filter.Component)

	var filtered []domain.LogEntry

	// Traverse in reverse (newest first)
	for i := len(r.entries) - 1; i >= 0; i-- {
		entry := r.entries[i]

		if !filter.Since.IsZero() && entry.Timestamp.Before(filter.Since) {
			continue
		}

		if filterLevel != "" && string(entry.Level) != filterLevel {
			continue
		}

		if filterComp != "" && !strings.Contains(strings.ToLower(entry.Component), filterComp) {
			continue
		}

		filtered = append(filtered, entry)
		if len(filtered) >= limit {
			break
		}
	}

	return &domain.LogQueryResult{
		TotalLogs:   int(atomic.LoadUint64(&r.totalLogs)),
		TotalErrors: int(atomic.LoadUint64(&r.totalErrs)),
		TotalWarns:  int(atomic.LoadUint64(&r.totalWarns)),
		FilterLevel: filterLevel,
		Logs:        filtered,
	}, nil
}

// GetErrorSummary returns aggregate error statistics.
func (r *RingLogger) GetErrorSummary(ctx context.Context) (*domain.ErrorSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	errByComp := make(map[string]int)
	var recentErrs []domain.LogEntry

	for i := len(r.entries) - 1; i >= 0; i-- {
		entry := r.entries[i]
		if entry.Level == domain.LogLevelError || entry.Level == domain.LogLevelCritical {
			errByComp[entry.Component]++
			if len(recentErrs) < 20 {
				recentErrs = append(recentErrs, entry)
			}
		}
	}

	var lastErrTime *time.Time
	var lastErrMsg string
	if r.lastError != nil {
		t := r.lastError.Timestamp
		lastErrTime = &t
		lastErrMsg = r.lastError.Message
	}

	return &domain.ErrorSummary{
		TotalErrors:       int(atomic.LoadUint64(&r.totalErrs)),
		TotalWarnings:     int(atomic.LoadUint64(&r.totalWarns)),
		LastErrorTime:     lastErrTime,
		LastErrorMessage:  lastErrMsg,
		ErrorsByComponent: errByComp,
		RecentErrors:      recentErrs,
	}, nil
}

// FormatStats returns a human-readable telemetry string.
func (r *RingLogger) FormatStats() string {
	return fmt.Sprintf("Logs: %d | Errors: %d | Warnings: %d",
		atomic.LoadUint64(&r.totalLogs),
		atomic.LoadUint64(&r.totalErrs),
		atomic.LoadUint64(&r.totalWarns),
	)
}

// Ensure interface compliance
var _ ports.LogCollector = (*RingLogger)(nil)
