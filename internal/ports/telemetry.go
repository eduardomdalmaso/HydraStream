package ports

import (
	"context"

	"hydrastream/internal/domain"
)

// LogCollector defines the secondary driven port for in-memory ring-buffer logging.
type LogCollector interface {
	RecordLog(level domain.LogLevel, component, message string, details map[string]interface{})
	QueryLogs(ctx context.Context, filter domain.LogFilter) (*domain.LogQueryResult, error)
	GetErrorSummary(ctx context.Context) (*domain.ErrorSummary, error)
}
