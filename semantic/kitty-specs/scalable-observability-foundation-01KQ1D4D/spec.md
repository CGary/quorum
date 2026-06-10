# Specification: Scalable Observability Foundation

**Mission ID**: 01KQ1D4DQCH70WDD1D9HQ7KST6
**Type**: software-dev

## Purpose
**TLDR**: Define a scalable observability foundation for MCP and worker flows with SQL telemetry, internal APIs, and retention/rollup strategy.
**Context**: This mission specifies how HSME should capture, persist, correlate, retain, and summarize observability data across both MCP requests and asynchronous worker execution so operators can diagnose latency, failures, and system health as the project grows.

## Success Criteria
- Operators can identify the end-to-end latency breakdown of any sampled MCP request or worker execution in under 2 minutes using persisted telemetry only.
- The system can correlate at least 95% of persisted MCP request events to their child spans and at least 95% of persisted worker task events to their owning task execution.
- The system retains detailed debug events for a configurable short window and aggregated rollups for a configurable longer window without requiring schema changes.
- At least one daily operational summary can report request counts, error counts, slow-request counts, and latency percentiles for both MCP and worker flows.
- Enabling basic observability does not increase median MCP request latency by more than 5%, and enabling debug observability does not increase median MCP request latency by more than 15% under the same workload.

## Key Entities
- **Observation Trace**: A correlated execution lineage that ties one MCP request, worker task execution, or background maintenance operation to its child spans and emitted events.
- **Observation Span**: A timed unit of work inside a trace, such as request parsing, tool execution, task leasing, embedding generation, or persistence.
- **Observation Event**: A persisted point-in-time record describing a completed span, an error, a state transition, or a sampled diagnostic fact.
- **Metric Rollup Bucket**: A time-bucketed aggregate summarizing counts, errors, and latency statistics for a component, operation, or tool.
- **Retention Policy**: A configurable rule that determines how long raw events, span records, and rollups are preserved.
- **Slow Operation Record**: A persisted event emitted when an operation crosses a configured duration threshold.

## Assumptions
- Observability data is an internal operational surface for HSME maintainers and is allowed to be technical, including exact SQL schema and internal API contracts.
- The first version targets persistence in the existing project database so diagnostics remain available to MCP clients and local operators without external infrastructure.
- Sampling, retention, and rollup windows must be configurable without requiring a schema redesign.
- Existing MCP timing instrumentation in stderr is diagnostic only and may be replaced or complemented by database-backed telemetry.
- Raw event volume can grow significantly over time, so retention and rollups are first-class requirements rather than follow-up enhancements.

## Domain Language
- **Canonical term**: "observability" refers to persisted telemetry used to diagnose latency, failures, throughput, and health.
- **Canonical term**: "trace" means the top-level correlated execution record; avoid using "request log" as a synonym when worker flows are included.
- **Canonical term**: "span" means a timed stage within a trace; avoid using "step" when the record is meant to be measured and aggregated.
- **Canonical term**: "rollup" means a derived aggregate bucket created from raw events; avoid using "summary" for these rows because the system may also store human-readable summaries elsewhere.
- **Canonical term**: "retention" means deletion or compaction policy over time; avoid using it interchangeably with sampling.

## User Scenarios & Testing

**Scenario 1: Diagnose a slow MCP request**
- **Actor**: Operator or AI agent investigating latency
- **Trigger**: A request such as `search_fuzzy` or `search_exact` is reported as slow
- **Action**: The operator queries persisted observability data by trace ID, request ID, or recent slow events to inspect total latency and stage-level durations
- **Outcome**: The operator can identify whether latency came from transport, request parsing, tool execution, formatting, response emission, or an external dependency
- **Exception**: If the request was not sampled at debug level, the operator can still inspect aggregated metrics and the slow-operation record

**Scenario 2: Diagnose a failing worker task**
- **Actor**: Operator or AI agent investigating async task failures
- **Trigger**: A worker task retries repeatedly, exceeds a timeout, or transitions to failed
- **Action**: The operator queries the task trace and span history to inspect lease timing, model call duration, persistence duration, retry count, and captured error payloads
- **Outcome**: The operator can determine whether the failure originated in task acquisition, inference, persistence, or retry handling
- **Exception**: If debug spans were not retained, the operator can still inspect error events and rollup buckets for failure rates by task type

**Scenario 3: Review long-term health trends**
- **Actor**: Maintainer planning capacity or tuning thresholds
- **Trigger**: The maintainer wants to understand latency, error, and volume trends across days or weeks
- **Action**: The maintainer queries rollup buckets by minute, hour, or day for MCP tools and worker task types
- **Outcome**: The maintainer can inspect counts, percentile latency trends, error rates, and slow-operation rates without scanning raw event history

## Requirements

### Functional Requirements

| ID | Description | Status |
|----|-------------|--------|
| FR-001 | The system must persist observability data for both MCP request flow and asynchronous worker flow under a shared correlation model based on trace identifiers. | Draft |
| FR-002 | The system must allow observability capture to be configured by level so operators can disable telemetry, enable lightweight metrics, or enable detailed debug/trace capture without code changes. | Draft |
| FR-003 | The system must persist a top-level trace record for every captured MCP request and every captured worker task execution. | Draft |
| FR-004 | The system must persist timed span records for important execution stages within a trace, including MCP transport stages and worker execution stages. | Draft |
| FR-005 | The system must persist error events and slow-operation events even when full debug capture is not enabled, provided the operation crosses configured thresholds or fails. | Draft |
| FR-006 | The system must support correlating child spans and events to their parent trace, plus secondary identifiers such as request ID, tool name, async task ID, memory ID, and model name when available. | Draft |
| FR-007 | The system must expose an internal observability API that allows code paths to start traces, open spans, finish spans, record errors, emit slow-operation records, and flush aggregated metrics. | Draft |
| FR-008 | The system must support separate capture policies for raw events, sampled debug spans, and aggregated rollups. | Draft |
| FR-009 | The system must generate time-bucketed metric rollups for MCP tools, worker task types, and shared dependencies such as database and inference operations. | Draft |
| FR-010 | The system must provide enough persisted fields to distinguish where latency occurred, including queueing, decode, handler execution, dependency time, formatting, and output emission when those stages are relevant. | Draft |
| FR-011 | The system must define exact SQL tables, indexes, and views that store traces, spans, metric rollups, retention policies, and rollup job checkpoints. | Draft |
| FR-012 | The system must define an internal API contract for observability capture, including configuration loading, trace/span lifecycle, error recording, and rollup execution. | Draft |
| FR-013 | The system must persist retention policies and rollup checkpoints so maintenance jobs can run deterministically and resume safely after interruption. | Draft |
| FR-014 | The system must support summarizing raw data into minute, hour, and day rollups without losing the ability to trace back recent slow or failing operations. | Draft |
| FR-015 | The system must make observability data queryable by operators without requiring access to external services. | Draft |

### Non-Functional Requirements

| ID | Description | Status |
|----|-------------|--------|
| NFR-001 | The system must support at least four observability levels: `off`, `basic`, `debug`, and `trace`. | Draft |
| NFR-002 | At `basic` level, median MCP request latency overhead introduced by observability must remain at or below 5% compared with observability disabled under the same workload. | Draft |
| NFR-003 | At `debug` level, median MCP request latency overhead introduced by observability must remain at or below 15% compared with observability disabled under the same workload. | Draft |
| NFR-004 | The system must preserve 100% of error events and 100% of slow-operation events that cross configured thresholds, regardless of sampling rate for normal successful operations. | Draft |
| NFR-005 | The default retention policy must keep raw span/event records for 7 days, hour-level rollups for 30 days, and day-level rollups for 365 days, with all windows configurable. | Draft |
| NFR-006 | The rollup process must be idempotent so rerunning the same bucket does not duplicate aggregates or corrupt prior totals. | Draft |
| NFR-007 | Rollup jobs must be resumable from persisted checkpoints and recover safely after interruption without reprocessing more than one incomplete bucket. | Draft |
| NFR-008 | The observability schema must support querying the last 24 hours of slow operations for a single component in under 2 seconds on a local deployment with at least 1 million raw observability rows. | Draft |
| NFR-009 | The observability schema must support querying a 30-day hourly rollup trend for one tool or task type in under 2 seconds on a local deployment with at least 365 day-level buckets and 720 hour-level buckets. | Draft |
| NFR-010 | The system must preserve clock timestamps in UTC and store durations in integer microseconds to avoid ambiguity and support stable aggregation. | Draft |

### Constraints

| ID | Description | Status |
|----|-------------|--------|
| C-001 | The first implementation must store observability data inside the existing HSME SQLite database rather than relying on an external telemetry backend. | Draft |
| C-002 | The observability design must remain compatible with current MCP and worker binaries and must not require external agents or collectors to operate. | Draft |
| C-003 | The exact SQL schema and internal API names defined in this specification become contractual surfaces for subsequent design and implementation work unless explicitly revised by a later mission. | Draft |
| C-004 | Sampling configuration, retention windows, rollup windows, and slow-operation thresholds must be configurable through stable configuration fields rather than hardcoded in call sites. | Draft |
| C-005 | Retention cleanup and rollup generation must be executable incrementally so maintenance work can run during normal local operation. | Draft |

## Exact SQL Schema

### Table: `obs_traces`
Represents one captured top-level execution.

```sql
CREATE TABLE obs_traces (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id TEXT NOT NULL UNIQUE,
    trace_kind TEXT NOT NULL CHECK (trace_kind IN ('mcp_request', 'worker_task', 'maintenance')),
    parent_trace_id TEXT,
    request_id TEXT,
    tool_name TEXT,
    task_id INTEGER,
    task_type TEXT,
    memory_id INTEGER,
    component TEXT NOT NULL,
    operation_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('ok', 'error', 'cancelled', 'timeout')),
    obs_level TEXT NOT NULL CHECK (obs_level IN ('off', 'basic', 'debug', 'trace')),
    sampled INTEGER NOT NULL DEFAULT 0 CHECK (sampled IN (0, 1)),
    started_at_utc TEXT NOT NULL,
    ended_at_utc TEXT NOT NULL,
    duration_us INTEGER NOT NULL CHECK (duration_us >= 0),
    error_code TEXT,
    error_message TEXT,
    meta_json TEXT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_obs_traces_started_at ON obs_traces(started_at_utc);
CREATE INDEX idx_obs_traces_kind_started ON obs_traces(trace_kind, started_at_utc);
CREATE INDEX idx_obs_traces_tool_started ON obs_traces(tool_name, started_at_utc);
CREATE INDEX idx_obs_traces_task_started ON obs_traces(task_type, started_at_utc);
CREATE INDEX idx_obs_traces_status_started ON obs_traces(status, started_at_utc);
```

### Table: `obs_spans`
Represents timed child work inside a trace.

```sql
CREATE TABLE obs_spans (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id TEXT NOT NULL,
    span_id TEXT NOT NULL,
    parent_span_id TEXT,
    component TEXT NOT NULL,
    operation_name TEXT NOT NULL,
    stage_name TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('ok', 'error', 'cancelled', 'timeout')),
    started_at_utc TEXT NOT NULL,
    ended_at_utc TEXT NOT NULL,
    duration_us INTEGER NOT NULL CHECK (duration_us >= 0),
    queue_delay_us INTEGER,
    rows_read INTEGER,
    rows_written INTEGER,
    bytes_in INTEGER,
    bytes_out INTEGER,
    error_code TEXT,
    error_message TEXT,
    meta_json TEXT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(trace_id, span_id),
    FOREIGN KEY(trace_id) REFERENCES obs_traces(trace_id) ON DELETE CASCADE
);

CREATE INDEX idx_obs_spans_trace_started ON obs_spans(trace_id, started_at_utc);
CREATE INDEX idx_obs_spans_component_stage ON obs_spans(component, stage_name, started_at_utc);
CREATE INDEX idx_obs_spans_duration ON obs_spans(duration_us, started_at_utc);
```

### Table: `obs_events`
Represents discrete events worth persisting even if a full debug trace is absent.

```sql
CREATE TABLE obs_events (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    trace_id TEXT,
    span_id TEXT,
    event_kind TEXT NOT NULL CHECK (event_kind IN ('error', 'slow_operation', 'state_transition', 'diagnostic')),
    component TEXT NOT NULL,
    operation_name TEXT NOT NULL,
    tool_name TEXT,
    task_id INTEGER,
    task_type TEXT,
    memory_id INTEGER,
    severity TEXT NOT NULL CHECK (severity IN ('debug', 'info', 'warn', 'error')),
    threshold_us INTEGER,
    observed_us INTEGER,
    message TEXT NOT NULL,
    details_json TEXT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY(trace_id) REFERENCES obs_traces(trace_id) ON DELETE SET NULL
);

CREATE INDEX idx_obs_events_created ON obs_events(created_at_utc);
CREATE INDEX idx_obs_events_kind_created ON obs_events(event_kind, created_at_utc);
CREATE INDEX idx_obs_events_component_created ON obs_events(component, created_at_utc);
CREATE INDEX idx_obs_events_tool_created ON obs_events(tool_name, created_at_utc);
CREATE INDEX idx_obs_events_task_created ON obs_events(task_type, created_at_utc);
CREATE INDEX idx_obs_events_trace ON obs_events(trace_id, created_at_utc);
```

### Table: `obs_metric_rollups`
Stores bucketed aggregates for fast trend queries.

```sql
CREATE TABLE obs_metric_rollups (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    bucket_level TEXT NOT NULL CHECK (bucket_level IN ('minute', 'hour', 'day')),
    bucket_start_utc TEXT NOT NULL,
    component TEXT NOT NULL,
    operation_name TEXT NOT NULL,
    tool_name TEXT,
    task_type TEXT,
    trace_kind TEXT NOT NULL CHECK (trace_kind IN ('mcp_request', 'worker_task', 'maintenance')),
    total_count INTEGER NOT NULL DEFAULT 0,
    success_count INTEGER NOT NULL DEFAULT 0,
    error_count INTEGER NOT NULL DEFAULT 0,
    slow_count INTEGER NOT NULL DEFAULT 0,
    sampled_count INTEGER NOT NULL DEFAULT 0,
    duration_total_us INTEGER NOT NULL DEFAULT 0,
    duration_max_us INTEGER NOT NULL DEFAULT 0,
    p50_us INTEGER,
    p95_us INTEGER,
    p99_us INTEGER,
    queue_delay_total_us INTEGER NOT NULL DEFAULT 0,
    bytes_in_total INTEGER NOT NULL DEFAULT 0,
    bytes_out_total INTEGER NOT NULL DEFAULT 0,
    rows_read_total INTEGER NOT NULL DEFAULT 0,
    rows_written_total INTEGER NOT NULL DEFAULT 0,
    last_source_event_at_utc TEXT,
    created_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    UNIQUE(bucket_level, bucket_start_utc, component, operation_name, COALESCE(tool_name, ''), COALESCE(task_type, ''), trace_kind)
);
```

### Table: `obs_retention_policies`
Stores active retention windows and sampling policy.

```sql
CREATE TABLE obs_retention_policies (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    policy_name TEXT NOT NULL UNIQUE,
    scope_kind TEXT NOT NULL CHECK (scope_kind IN ('events', 'traces', 'spans', 'rollups')),
    bucket_level TEXT CHECK (bucket_level IN ('minute', 'hour', 'day')),
    keep_days INTEGER NOT NULL CHECK (keep_days >= 0),
    sample_rate REAL CHECK (sample_rate IS NULL OR (sample_rate >= 0.0 AND sample_rate <= 1.0)),
    slow_threshold_us INTEGER,
    enabled INTEGER NOT NULL DEFAULT 1 CHECK (enabled IN (0, 1)),
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
```

### Table: `obs_rollup_jobs`
Tracks rollup and cleanup progress.

```sql
CREATE TABLE obs_rollup_jobs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_name TEXT NOT NULL UNIQUE,
    source_scope TEXT NOT NULL CHECK (source_scope IN ('raw_to_minute', 'minute_to_hour', 'hour_to_day', 'retention_cleanup')),
    last_completed_bucket_start_utc TEXT,
    last_run_started_at_utc TEXT,
    last_run_finished_at_utc TEXT,
    last_status TEXT NOT NULL DEFAULT 'idle' CHECK (last_status IN ('idle', 'running', 'ok', 'error')),
    last_error TEXT,
    updated_at_utc TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
```

## Required Views

```sql
CREATE VIEW obs_recent_slow_operations AS
SELECT
    e.created_at_utc,
    e.component,
    e.operation_name,
    e.tool_name,
    e.task_type,
    e.observed_us,
    e.threshold_us,
    e.message,
    e.trace_id
FROM obs_events e
WHERE e.event_kind = 'slow_operation';

CREATE VIEW obs_error_events AS
SELECT
    e.created_at_utc,
    e.component,
    e.operation_name,
    e.tool_name,
    e.task_type,
    e.severity,
    e.message,
    e.details_json,
    e.trace_id
FROM obs_events e
WHERE e.event_kind = 'error';
```

## Internal Observability API

### Configuration Contract
```go
type ObservabilityLevel string

const (
    ObservabilityOff   ObservabilityLevel = "off"
    ObservabilityBasic ObservabilityLevel = "basic"
    ObservabilityDebug ObservabilityLevel = "debug"
    ObservabilityTrace ObservabilityLevel = "trace"
)

type ObservabilityConfig struct {
    Level               ObservabilityLevel
    DefaultSampleRate   float64
    SlowThresholds      map[string]time.Duration
    RawRetentionDays    int
    MinuteRetentionDays int
    HourRetentionDays   int
    DayRetentionDays    int
    FlushInterval       time.Duration
}
```

### Core API Contract
```go
type TraceContext struct {
    TraceID      string
    TraceKind    string
    Component    string
    Operation    string
    RequestID    string
    ToolName     string
    TaskID       int64
    TaskType     string
    MemoryID     int64
    Sampled      bool
    StartedAtUTC time.Time
}

type SpanContext struct {
    TraceID       string
    SpanID        string
    ParentSpanID  string
    Component     string
    Operation     string
    StageName     string
    StartedAtUTC  time.Time
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
}
```

### API Arguments Contract
```go
type StartTraceArgs struct {
    TraceKind    string
    Component    string
    Operation    string
    RequestID    string
    ToolName     string
    TaskID       int64
    TaskType     string
    MemoryID     int64
    ParentTraceID string
    Meta         map[string]any
}

type TraceResult struct {
    Status       string
    ErrorCode    string
    ErrorMessage string
    Meta         map[string]any
}

type StartSpanArgs struct {
    TraceID      string
    ParentSpanID string
    Component    string
    Operation    string
    StageName    string
    Meta         map[string]any
}

type SpanResult struct {
    Status        string
    QueueDelay    time.Duration
    RowsRead      int64
    RowsWritten   int64
    BytesIn       int64
    BytesOut      int64
    ErrorCode     string
    ErrorMessage  string
    Meta          map[string]any
}

type ErrorEvent struct {
    TraceID       string
    SpanID        string
    Component     string
    Operation     string
    ToolName      string
    TaskID        int64
    TaskType      string
    MemoryID      int64
    Severity      string
    Message       string
    Details       map[string]any
}

type SlowOperationEvent struct {
    TraceID       string
    SpanID        string
    Component     string
    Operation     string
    ToolName      string
    TaskID        int64
    TaskType      string
    MemoryID      int64
    Threshold     time.Duration
    Observed      time.Duration
    Message       string
    Details       map[string]any
}

type DiagnosticEvent struct {
    TraceID       string
    SpanID        string
    Component     string
    Operation     string
    Message       string
    Details       map[string]any
}
```

## Retention and Rollup Scheme

### Default Retention Policy Rows
```sql
INSERT INTO obs_retention_policies (policy_name, scope_kind, bucket_level, keep_days, sample_rate, slow_threshold_us, enabled) VALUES
    ('raw-traces-default', 'traces', NULL, 7, 0.10, NULL, 1),
    ('raw-spans-default', 'spans', NULL, 7, 0.10, NULL, 1),
    ('raw-events-default', 'events', NULL, 14, 1.0, 100000, 1),
    ('minute-rollups-default', 'rollups', 'minute', 7, NULL, NULL, 1),
    ('hour-rollups-default', 'rollups', 'hour', 30, NULL, NULL, 1),
    ('day-rollups-default', 'rollups', 'day', 365, NULL, NULL, 1);
```

### Rollup Rules
- Raw traces and spans feed minute buckets.
- Minute buckets feed hour buckets.
- Hour buckets feed day buckets.
- Cleanup may delete raw traces/spans only after their source buckets are successfully materialized.
- Error events and slow-operation events are never dropped early due to sampling; they are controlled only by retention windows.
- If a rollup job fails mid-bucket, the next run must restart that same bucket and overwrite the prior partial aggregate.

### Required Rollup Jobs
- `raw_to_minute`
- `minute_to_hour`
- `hour_to_day`
- `retention_cleanup`

## Dependencies and Operational Rules
- MCP request instrumentation and worker instrumentation must use the same `Recorder` contract so both flows emit compatible traces and spans.
- The same trace may include dependency spans for SQLite, Ollama, queue leasing, formatting, and transport emission when those stages are measurable.
- Slow thresholds must be configurable per operation, not globally only.
- Maintenance jobs must emit their own traces so retention and rollup work is itself observable.

