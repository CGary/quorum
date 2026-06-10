package observability

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

type SQLiteRecorder struct {
	db  *sql.DB
	cfg Config
}

func NewSQLiteRecorder(db *sql.DB, cfg Config) Recorder {
	if db == nil || !cfg.Enabled() {
		return NewNoopRecorder(cfg)
	}
	return &SQLiteRecorder{db: db, cfg: cfg}
}

func (r *SQLiteRecorder) Config() Config { return r.cfg }
func (r *SQLiteRecorder) Enabled() bool  { return r.cfg.Enabled() }

func (r *SQLiteRecorder) StartTrace(ctx context.Context, args StartTraceArgs) (TraceContext, context.Context) {
	started := args.StartedAt.UTC()
	if started.IsZero() {
		started = time.Now().UTC()
	}
	sampled := r.cfg.ShouldSample()
	if args.Sampled != nil {
		sampled = *args.Sampled
	}
	trace := TraceContext{
		TraceID:       nextID("trace"),
		TraceKind:     args.TraceKind,
		Component:     args.Component,
		Operation:     args.Operation,
		RequestID:     args.RequestID,
		ToolName:      args.ToolName,
		TaskID:        args.TaskID,
		TaskType:      args.TaskType,
		MemoryID:      args.MemoryID,
		Sampled:       sampled,
		StartedAtUTC:  started,
		ParentTraceID: args.ParentTraceID,
	}
	if !r.Enabled() {
		return trace, ctx
	}
	_, _ = r.db.ExecContext(ctx, `
		INSERT INTO obs_traces(trace_id, trace_kind, parent_trace_id, request_id, tool_name, task_id, task_type, memory_id, component, operation_name, status, obs_level, sampled, started_at_utc, ended_at_utc, duration_us, meta_json)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'ok', ?, ?, ?, ?, 0, ?)
	`, trace.TraceID, trace.TraceKind, nullableString(trace.ParentTraceID), nullableString(trace.RequestID), nullableString(trace.ToolName), nullableInt64(trace.TaskID), nullableString(trace.TaskType), nullableInt64(trace.MemoryID), trace.Component, trace.Operation, string(r.cfg.Level), boolToInt(sampled), formatUTC(started), formatUTC(started), mustJSON(args.Meta))
	return trace, ctx
}

func (r *SQLiteRecorder) FinishTrace(ctx context.Context, trace TraceContext, result TraceResult) error {
	if !r.Enabled() || trace.TraceID == "" {
		return nil
	}
	ended := result.EndedAt.UTC()
	if ended.IsZero() {
		ended = time.Now().UTC()
	}
	duration := ended.Sub(trace.StartedAtUTC)
	_, err := r.db.ExecContext(ctx, `UPDATE obs_traces SET status = ?, ended_at_utc = ?, duration_us = ?, error_code = ?, error_message = ?, meta_json = ? WHERE trace_id = ?`, defaultStatus(result.Status), formatUTC(ended), duration.Microseconds(), nullableString(result.ErrorCode), nullableString(result.ErrorMessage), mustJSON(result.Meta), trace.TraceID)
	if err != nil {
		return err
	}
	if threshold := r.cfg.SlowThreshold(trace.Component + "." + trace.Operation); threshold > 0 && duration >= threshold {
		_ = r.RecordSlowOperation(ctx, SlowOperationEvent{TraceID: trace.TraceID, Component: trace.Component, Operation: trace.Operation, ToolName: trace.ToolName, TaskID: trace.TaskID, TaskType: trace.TaskType, MemoryID: trace.MemoryID, Threshold: threshold, Observed: duration, Message: fmt.Sprintf("slow trace: %s.%s", trace.Component, trace.Operation)})
	}
	return nil
}

func (r *SQLiteRecorder) StartSpan(ctx context.Context, args StartSpanArgs) (SpanContext, context.Context) {
	started := args.StartedAt.UTC()
	if started.IsZero() {
		started = time.Now().UTC()
	}
	span := SpanContext{TraceID: args.TraceID, SpanID: nextID("span"), ParentSpanID: args.ParentSpanID, Component: args.Component, Operation: args.Operation, StageName: args.StageName, StartedAtUTC: started}
	if !r.Enabled() || args.TraceID == "" || !r.cfg.CaptureSpans() {
		return span, ctx
	}
	_, _ = r.db.ExecContext(ctx, `INSERT INTO obs_spans(trace_id, span_id, parent_span_id, component, operation_name, stage_name, status, started_at_utc, ended_at_utc, duration_us, meta_json) VALUES(?, ?, ?, ?, ?, ?, 'ok', ?, ?, 0, ?)`, span.TraceID, span.SpanID, nullableString(span.ParentSpanID), span.Component, span.Operation, span.StageName, formatUTC(span.StartedAtUTC), formatUTC(span.StartedAtUTC), mustJSON(args.Meta))
	return span, ctx
}

func (r *SQLiteRecorder) FinishSpan(ctx context.Context, span SpanContext, result SpanResult) error {
	if !r.Enabled() || span.TraceID == "" || span.SpanID == "" || !r.cfg.CaptureSpans() {
		return nil
	}
	ended := result.EndedAt.UTC()
	if ended.IsZero() {
		ended = time.Now().UTC()
	}
	duration := ended.Sub(span.StartedAtUTC)
	_, err := r.db.ExecContext(ctx, `UPDATE obs_spans SET status = ?, ended_at_utc = ?, duration_us = ?, queue_delay_us = ?, rows_read = ?, rows_written = ?, bytes_in = ?, bytes_out = ?, error_code = ?, error_message = ?, meta_json = ? WHERE trace_id = ? AND span_id = ?`, defaultStatus(result.Status), formatUTC(ended), duration.Microseconds(), nullableDuration(result.QueueDelay), nullableInt64(result.RowsRead), nullableInt64(result.RowsWritten), nullableInt64(result.BytesIn), nullableInt64(result.BytesOut), nullableString(result.ErrorCode), nullableString(result.ErrorMessage), mustJSON(result.Meta), span.TraceID, span.SpanID)
	if err != nil {
		return err
	}
	if threshold := r.cfg.SlowThreshold(span.Component + "." + span.StageName); threshold > 0 && duration >= threshold {
		_ = r.RecordSlowOperation(ctx, SlowOperationEvent{TraceID: span.TraceID, SpanID: span.SpanID, Component: span.Component, Operation: span.Operation, Threshold: threshold, Observed: duration, Message: fmt.Sprintf("slow span: %s.%s", span.Component, span.StageName)})
	}
	return nil
}

func (r *SQLiteRecorder) RecordError(ctx context.Context, event ErrorEvent) error {
	if !r.Enabled() {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO obs_events(trace_id, span_id, event_kind, component, operation_name, tool_name, task_id, task_type, memory_id, severity, message, details_json) VALUES(?, ?, 'error', ?, ?, ?, ?, ?, ?, ?, ?, ?)`, nullableString(event.TraceID), nullableString(event.SpanID), event.Component, event.Operation, nullableString(event.ToolName), nullableInt64(event.TaskID), nullableString(event.TaskType), nullableInt64(event.MemoryID), defaultSeverity(event.Severity), event.Message, mustJSON(event.Details))
	return err
}

func (r *SQLiteRecorder) RecordSlowOperation(ctx context.Context, event SlowOperationEvent) error {
	if !r.Enabled() {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO obs_events(trace_id, span_id, event_kind, component, operation_name, tool_name, task_id, task_type, memory_id, severity, threshold_us, observed_us, message, details_json) VALUES(?, ?, 'slow_operation', ?, ?, ?, ?, ?, ?, 'warn', ?, ?, ?, ?)`, nullableString(event.TraceID), nullableString(event.SpanID), event.Component, event.Operation, nullableString(event.ToolName), nullableInt64(event.TaskID), nullableString(event.TaskType), nullableInt64(event.MemoryID), nullableDuration(event.Threshold), nullableDuration(event.Observed), event.Message, mustJSON(event.Details))
	return err
}

func (r *SQLiteRecorder) RecordDiagnostic(ctx context.Context, event DiagnosticEvent) error {
	if !r.Enabled() || !r.cfg.CaptureDiagnostics() {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO obs_events(trace_id, span_id, event_kind, component, operation_name, severity, message, details_json) VALUES(?, ?, 'diagnostic', ?, ?, 'debug', ?, ?)`, nullableString(event.TraceID), nullableString(event.SpanID), event.Component, event.Operation, event.Message, mustJSON(event.Details))
	return err
}

func (r *SQLiteRecorder) FlushRollups(ctx context.Context, now time.Time) error {
	return NewMaintenanceService(r.db, r.cfg).FlushRollups(ctx, now)
}
func (r *SQLiteRecorder) RunRetention(ctx context.Context, now time.Time) error {
	return NewMaintenanceService(r.db, r.cfg).RunRetention(ctx, now)
}

func (r *SQLiteRecorder) RecentSlowOperations(ctx context.Context, limit int) ([]SlowOperationSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT created_at_utc, component, operation_name, ifnull(tool_name,''), ifnull(task_type,''), ifnull(observed_us,0), ifnull(threshold_us,0), message, ifnull(trace_id,'') FROM obs_recent_slow_operations ORDER BY created_at_utc DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SlowOperationSummary
	for rows.Next() {
		var s SlowOperationSummary
		if err := rows.Scan(&s.CreatedAtUTC, &s.Component, &s.Operation, &s.ToolName, &s.TaskType, &s.ObservedUS, &s.ThresholdUS, &s.Message, &s.TraceID); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *SQLiteRecorder) RecentErrorEvents(ctx context.Context, limit int) ([]ErrorEventSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT created_at_utc, component, operation_name, ifnull(tool_name,''), ifnull(task_type,''), severity, message, ifnull(trace_id,'') FROM obs_error_events ORDER BY created_at_utc DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ErrorEventSummary
	for rows.Next() {
		var e ErrorEventSummary
		if err := rows.Scan(&e.CreatedAtUTC, &e.Component, &e.Operation, &e.ToolName, &e.TaskType, &e.Severity, &e.Message, &e.TraceID); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *SQLiteRecorder) RollupHealth(ctx context.Context, limit int) ([]RollupHealthSummary, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.db.QueryContext(ctx, `SELECT bucket_level, bucket_start_utc, component, operation_name, ifnull(tool_name,''), ifnull(task_type,''), trace_kind, total_count, error_count, slow_count, ifnull(p95_us,0) FROM obs_metric_rollups ORDER BY bucket_start_utc DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RollupHealthSummary
	for rows.Next() {
		var s RollupHealthSummary
		if err := rows.Scan(&s.BucketLevel, &s.BucketStartUTC, &s.Component, &s.Operation, &s.ToolName, &s.TaskType, &s.TraceKind, &s.TotalCount, &s.ErrorCount, &s.SlowCount, &s.P95US); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func mustJSON(v map[string]any) string {
	if len(v) == 0 {
		return "{}"
	}
	b, _ := json.Marshal(v)
	return string(b)
}
func formatUTC(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
func defaultStatus(v string) string {
	if v == "" {
		return "ok"
	}
	return v
}
func defaultSeverity(v string) string {
	if v == "" {
		return "error"
	}
	return v
}
func nullableString(v string) any {
	if v == "" {
		return nil
	}
	return v
}
func nullableInt64(v int64) any {
	if v == 0 {
		return nil
	}
	return v
}
func nullableDuration(v time.Duration) any {
	if v == 0 {
		return nil
	}
	return v.Microseconds()
}

func upsertRollupBucket(ctx context.Context, tx *sql.Tx, bucketLevel, bucketStart, component, operation, toolName, taskType, traceKind string, values map[string]int64, lastSource string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO obs_metric_rollups(bucket_level, bucket_start_utc, component, operation_name, tool_name, task_type, trace_kind, total_count, success_count, error_count, slow_count, sampled_count, duration_total_us, duration_max_us, p50_us, p95_us, p99_us, queue_delay_total_us, bytes_in_total, bytes_out_total, rows_read_total, rows_written_total, last_source_event_at_utc, updated_at_utc) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now')) ON CONFLICT(bucket_level, bucket_start_utc, component, operation_name, ifnull(tool_name, ''), ifnull(task_type, ''), trace_kind) DO UPDATE SET total_count=excluded.total_count, success_count=excluded.success_count, error_count=excluded.error_count, slow_count=excluded.slow_count, sampled_count=excluded.sampled_count, duration_total_us=excluded.duration_total_us, duration_max_us=excluded.duration_max_us, p50_us=excluded.p50_us, p95_us=excluded.p95_us, p99_us=excluded.p99_us, queue_delay_total_us=excluded.queue_delay_total_us, bytes_in_total=excluded.bytes_in_total, bytes_out_total=excluded.bytes_out_total, rows_read_total=excluded.rows_read_total, rows_written_total=excluded.rows_written_total, last_source_event_at_utc=excluded.last_source_event_at_utc, updated_at_utc=strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`, bucketLevel, bucketStart, component, operation, nullableString(toolName), nullableString(taskType), traceKind, values["total_count"], values["success_count"], values["error_count"], values["slow_count"], values["sampled_count"], values["duration_total_us"], values["duration_max_us"], values["p50_us"], values["p95_us"], values["p99_us"], values["queue_delay_total_us"], values["bytes_in_total"], values["bytes_out_total"], values["rows_read_total"], values["rows_written_total"], nullableString(lastSource))
	return err
}

func aggregatePercentiles(values []int64) (int64, int64, int64) {
	if len(values) == 0 {
		return 0, 0, 0
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	pick := func(f float64) int64 {
		idx := int(float64(len(values)-1) * f)
		if idx < 0 {
			idx = 0
		}
		if idx >= len(values) {
			idx = len(values) - 1
		}
		return values[idx]
	}
	return pick(0.50), pick(0.95), pick(0.99)
}
