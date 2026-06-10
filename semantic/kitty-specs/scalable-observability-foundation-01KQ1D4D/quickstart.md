# Quickstart: Scalable Observability Foundation

## Goal
Validate that HSME can capture runtime observability for MCP and worker flows, and that a dedicated ops runner can maintain rollups and retention.

## 1. Prepare configuration
- Set observability level to `basic` or `debug`.
- Configure default sample rate and slow thresholds.
- Ensure default retention policies are present in the database.

## 2. Apply schema
- Run the observability schema migration.
- Verify that the following tables exist:
  - `obs_traces`
  - `obs_spans`
  - `obs_events`
  - `obs_metric_rollups`
  - `obs_retention_policies`
  - `obs_rollup_jobs`

## 3. Validate MCP capture
- Start the MCP server.
- Run an MCP tool such as `search_exact` or `search_fuzzy`.
- Confirm that at least one trace row and one or more span rows are written.
- If the request crosses the configured threshold, confirm a `slow_operation` event exists.

## 4. Validate worker capture
- Start the semantic worker.
- Process at least one async task.
- Confirm a worker trace exists with stage spans for task acquisition and execution.
- If a task fails, confirm an `error` event is persisted.

## 5. Validate ops runner
- Start the dedicated operations runner.
- Trigger or wait for one rollup cycle.
- Confirm minute rollups are created.
- Trigger retention cleanup in a safe test dataset.
- Confirm old raw rows are only removed after their source buckets are materialized.

## 6. Validate queryability
- Query `obs_recent_slow_operations` for recent latency outliers.
- Query `obs_error_events` for recent failures.
- Query hourly/day rollup buckets for one tool and one task type.

## 7. Regression checks
- Compare median MCP latency with observability `off`, `basic`, and `debug`.
- Verify that error and slow-operation events persist regardless of sample rate.
- Verify rollup reruns do not duplicate or inflate aggregates.
