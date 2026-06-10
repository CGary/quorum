# Research: Scalable Observability Foundation

## Decision 1: Use SQLite-backed observability as the initial telemetry backend
- **Decision**: Persist traces, spans, events, policies, and rollups in the existing HSME SQLite database.
- **Rationale**: Keeps observability queryable by local operators and MCP-adjacent workflows without introducing external infrastructure.
- **Alternatives considered**:
  - External metrics backend: rejected for operational complexity and mismatch with current project constraints.
  - Filesystem logs only: rejected because correlation, retention, and rollups become fragile and hard to query.

## Decision 2: Separate capture from maintenance responsibilities
- **Decision**: Embed capture into MCP and worker flows, but run rollups/retention/cleanup in a dedicated operations runner.
- **Rationale**: Capture must be close to runtime stages, but maintenance should be isolated to avoid mixing latency-sensitive paths with consolidation and cleanup work.
- **Alternatives considered**:
  - Reuse `hsme-worker` for all maintenance: possible but rejected for this mission because the user prioritizes long-term robustness and clean subsystem boundaries.
  - Run maintenance inside MCP process: rejected because it risks latency jitter and muddled process responsibilities.

## Decision 3: Model observability with traces, spans, events, and rollup buckets
- **Decision**: Treat traces, spans, discrete events, and aggregate rollup buckets as separate entities with distinct retention rules.
- **Rationale**: Recent diagnosis needs raw lineage, while trend analysis needs small, stable aggregates.
- **Alternatives considered**:
  - Single table of logs: rejected because it scales poorly for correlation and percentile reporting.
  - Rollups only: rejected because it loses request/task-level debugging value.

## Decision 4: Use level-based observability with guaranteed preservation for errors and slow paths
- **Decision**: Support `off`, `basic`, `debug`, and `trace` levels, but always preserve errors and threshold-crossing slow events.
- **Rationale**: Normal success traffic can be sampled while still guaranteeing preservation of diagnostically valuable failures and regressions.
- **Alternatives considered**:
  - Always-on full tracing: rejected for storage and latency cost.
  - Errors only: rejected because it misses degradation before failure.

## Decision 5: Use minute/hour/day rollups with explicit checkpoint rows
- **Decision**: Raw events feed minute buckets; minute feeds hour; hour feeds day; every stage uses persisted checkpoint state.
- **Rationale**: Supports scalable queries and resumable maintenance without rescanning all raw data every time.
- **Alternatives considered**:
  - Recompute all aggregates on demand: rejected for local performance cost.
  - Day-only aggregation: rejected because it hides useful operational detail.

## Decision 6: Keep schema and API contracts explicit in the plan artifacts
- **Decision**: Treat schema and recorder contract as stable design surfaces, not incidental implementation details.
- **Rationale**: Subsequent tasks and work packages need an exact target for migrations, interfaces, tests, and process boundaries.
- **Alternatives considered**:
  - Leave naming and contract shape open until implementation: rejected because it would delay core architectural choices and weaken future task breakdown quality.
