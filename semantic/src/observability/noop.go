package observability

import (
	"context"
	"time"
)

type noopRecorder struct{ cfg Config }

func NewNoopRecorder(cfg Config) Recorder { return noopRecorder{cfg: cfg} }

func (n noopRecorder) StartTrace(ctx context.Context, args StartTraceArgs) (TraceContext, context.Context) {
	started := args.StartedAt
	if started.IsZero() {
		started = time.Now().UTC()
	}
	return TraceContext{TraceKind: args.TraceKind, Component: args.Component, Operation: args.Operation, RequestID: args.RequestID, ToolName: args.ToolName, TaskID: args.TaskID, TaskType: args.TaskType, MemoryID: args.MemoryID, Sampled: false, StartedAtUTC: started, ParentTraceID: args.ParentTraceID}, ctx
}
func (n noopRecorder) FinishTrace(context.Context, TraceContext, TraceResult) error { return nil }
func (n noopRecorder) StartSpan(ctx context.Context, args StartSpanArgs) (SpanContext, context.Context) {
	started := args.StartedAt
	if started.IsZero() {
		started = time.Now().UTC()
	}
	return SpanContext{TraceID: args.TraceID, Component: args.Component, Operation: args.Operation, StageName: args.StageName, StartedAtUTC: started}, ctx
}
func (n noopRecorder) FinishSpan(context.Context, SpanContext, SpanResult) error     { return nil }
func (n noopRecorder) RecordError(context.Context, ErrorEvent) error                 { return nil }
func (n noopRecorder) RecordSlowOperation(context.Context, SlowOperationEvent) error { return nil }
func (n noopRecorder) RecordDiagnostic(context.Context, DiagnosticEvent) error       { return nil }
func (n noopRecorder) FlushRollups(context.Context, time.Time) error                 { return nil }
func (n noopRecorder) RunRetention(context.Context, time.Time) error                 { return nil }
func (n noopRecorder) RecentSlowOperations(context.Context, int) ([]SlowOperationSummary, error) {
	return nil, nil
}
func (n noopRecorder) RecentErrorEvents(context.Context, int) ([]ErrorEventSummary, error) {
	return nil, nil
}
func (n noopRecorder) RollupHealth(context.Context, int) ([]RollupHealthSummary, error) {
	return nil, nil
}
func (n noopRecorder) Enabled() bool  { return false }
func (n noopRecorder) Config() Config { return n.cfg }
