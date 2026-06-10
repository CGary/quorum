# Data Model: Scalable Observability Foundation

## 1. Observation Trace
Represents one correlated top-level execution.

### Fields
- `trace_id` — stable unique correlation identifier
- `trace_kind` — one of `mcp_request`, `worker_task`, `maintenance`
- `parent_trace_id` — optional parent for nested maintenance or chained work
- `request_id` — optional MCP request identifier
- `tool_name` — optional MCP tool name
- `task_id` — optional async task identifier
- `task_type` — optional async task type
- `memory_id` — optional affected memory
- `component` — runtime component owning the trace
- `operation_name` — business/operational action name
- `status` — final status
- `obs_level` — capture level in effect
- `sampled` — whether detailed trace/span capture occurred
- `started_at_utc`, `ended_at_utc`, `duration_us`
- `error_code`, `error_message`, `meta_json`

### Invariants
- `trace_id` is globally unique within the database.
- `duration_us` must equal or exceed zero.
- Every persisted span or event attached to a trace must reference an existing `trace_id` unless retention has already compacted the parent trace by policy.

## 2. Observation Span
Represents a timed child unit of work within a trace.

### Fields
- `trace_id`, `span_id`, `parent_span_id`
- `component`, `operation_name`, `stage_name`
- `status`
- `started_at_utc`, `ended_at_utc`, `duration_us`
- `queue_delay_us`, `rows_read`, `rows_written`, `bytes_in`, `bytes_out`
- `error_code`, `error_message`, `meta_json`

### Invariants
- `span_id` is unique within a trace.
- Parent span, if present, belongs to the same trace.
- Stage names must map to measurable execution boundaries.

## 3. Observation Event
Represents a discrete persisted fact.

### Fields
- `trace_id`, `span_id`
- `event_kind` — `error`, `slow_operation`, `state_transition`, `diagnostic`
- `component`, `operation_name`
- `tool_name`, `task_id`, `task_type`, `memory_id`
- `severity`
- `threshold_us`, `observed_us`
- `message`, `details_json`, `created_at_utc`

### Invariants
- Error and slow-operation events must be preserved regardless of normal sample rate.
- `observed_us` should be populated for duration-related event kinds.

## 4. Metric Rollup Bucket
Represents aggregated telemetry over a time bucket.

### Fields
- `bucket_level` — `minute`, `hour`, `day`
- `bucket_start_utc`
- `component`, `operation_name`, `tool_name`, `task_type`, `trace_kind`
- count fields
- cumulative duration/IO fields
- percentile fields
- source freshness fields

### Invariants
- One unique bucket row exists per dimension set.
- Rollups are overwritten idempotently for a bucket if recomputed.

## 5. Retention Policy
Represents how long data is kept and sampled.

### Fields
- `policy_name`
- `scope_kind`
- `bucket_level`
- `keep_days`
- `sample_rate`
- `slow_threshold_us`
- `enabled`
- `updated_at_utc`

### Invariants
- Policy names are unique.
- Disabled policies are ignored by maintenance jobs but preserved in storage.

## 6. Rollup Job Checkpoint
Represents progress and state of maintenance work.

### Fields
- `job_name`
- `source_scope`
- `last_completed_bucket_start_utc`
- `last_run_started_at_utc`, `last_run_finished_at_utc`
- `last_status`, `last_error`, `updated_at_utc`

### Invariants
- Each job name is unique.
- Only one logical checkpoint row exists per maintenance workflow.

## Relationships
- One trace has many spans.
- One trace has many events.
- One bucket aggregates many traces/spans/events.
- One retention policy can govern many rows of a scope kind.
- One rollup job progresses over many buckets.

## State Transitions
### Trace Status
- running (implicit in memory) → `ok`
- running → `error`
- running → `timeout`
- running → `cancelled`

### Rollup Job Status
- `idle` → `running`
- `running` → `ok`
- `running` → `error`
- `ok`/`error` → `running` on next scheduled execution
