package observability

import (
	"context"
	"time"
)

type TraceContext struct {
	TraceID       string
	TraceKind     string
	Component     string
	Operation     string
	RequestID     string
	ToolName      string
	TaskID        int64
	TaskType      string
	MemoryID      int64
	Sampled       bool
	StartedAtUTC  time.Time
	ParentTraceID string
}

type SpanContext struct {
	TraceID      string
	SpanID       string
	ParentSpanID string
	Component    string
	Operation    string
	StageName    string
	StartedAtUTC time.Time
}

type StartTraceArgs struct {
	TraceKind     string
	Component     string
	Operation     string
	RequestID     string
	ToolName      string
	TaskID        int64
	TaskType      string
	MemoryID      int64
	ParentTraceID string
	Meta          map[string]any
	StartedAt     time.Time
	Sampled       *bool
}

type TraceResult struct {
	Status       string
	ErrorCode    string
	ErrorMessage string
	Meta         map[string]any
	EndedAt      time.Time
}

type StartSpanArgs struct {
	TraceID      string
	ParentSpanID string
	Component    string
	Operation    string
	StageName    string
	Meta         map[string]any
	StartedAt    time.Time
}

type SpanResult struct {
	Status       string
	QueueDelay   time.Duration
	RowsRead     int64
	RowsWritten  int64
	BytesIn      int64
	BytesOut     int64
	ErrorCode    string
	ErrorMessage string
	Meta         map[string]any
	EndedAt      time.Time
}

type ErrorEvent struct {
	TraceID   string
	SpanID    string
	Component string
	Operation string
	ToolName  string
	TaskID    int64
	TaskType  string
	MemoryID  int64
	Severity  string
	Message   string
	Details   map[string]any
}

type SlowOperationEvent struct {
	TraceID   string
	SpanID    string
	Component string
	Operation string
	ToolName  string
	TaskID    int64
	TaskType  string
	MemoryID  int64
	Threshold time.Duration
	Observed  time.Duration
	Message   string
	Details   map[string]any
}

type DiagnosticEvent struct {
	TraceID   string
	SpanID    string
	Component string
	Operation string
	Message   string
	Details   map[string]any
}

type SlowOperationSummary struct {
	CreatedAtUTC string
	Component    string
	Operation    string
	ToolName     string
	TaskType     string
	ObservedUS   int64
	ThresholdUS  int64
	Message      string
	TraceID      string
}

type ErrorEventSummary struct {
	CreatedAtUTC string
	Component    string
	Operation    string
	ToolName     string
	TaskType     string
	Severity     string
	Message      string
	TraceID      string
}

type RollupHealthSummary struct {
	BucketLevel    string
	BucketStartUTC string
	Component      string
	Operation      string
	ToolName       string
	TaskType       string
	TraceKind      string
	TotalCount     int64
	ErrorCount     int64
	SlowCount      int64
	P95US          int64
}

type Recorder interface {
	StartTrace(ctx context.Context, args StartTraceArgs) (TraceContext, context.Context)
	FinishTrace(ctx context.Context, trace TraceContext, result TraceResult) error
	StartSpan(ctx context.Context, args StartSpanArgs) (SpanContext, context.Context)
	FinishSpan(ctx context.Context, span SpanContext, result SpanResult) error
	RecordError(ctx context.Context, event ErrorEvent) error
	RecordSlowOperation(ctx context.Context, event SlowOperationEvent) error
	RecordDiagnostic(ctx context.Context, event DiagnosticEvent) error
	FlushRollups(ctx context.Context, now time.Time) error
	RunRetention(ctx context.Context, now time.Time) error
	RecentSlowOperations(ctx context.Context, limit int) ([]SlowOperationSummary, error)
	RecentErrorEvents(ctx context.Context, limit int) ([]ErrorEventSummary, error)
	RollupHealth(ctx context.Context, limit int) ([]RollupHealthSummary, error)
	Enabled() bool
	Config() Config
}
