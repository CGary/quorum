# Implementation Plan: Scalable Observability Foundation

**Mission ID**: 01KQ1D4DQCH70WDD1D9HQ7KST6
**Mission Slug**: scalable-observability-foundation-01KQ1D4D
**Type**: software-dev

## Branch Strategy
- Current branch at workflow start: `main`
- Planning/base branch for this change: `main`
- Final merge target for completed changes: `main`
- Branch matches intended target: `true`

## Technical Context
- Existing runtime surfaces:
  - `hsme` MCP server handles synchronous request/response over stdio.
  - `hsme-worker` handles asynchronous semantic enrichment.
- Approved engineering direction:
  - Observability capture is embedded in MCP and worker runtime paths.
  - Rollups, retention, and housekeeping run in a dedicated operations runner.
- Storage backend for V1 observability remains the existing SQLite database.
- Existing stderr timing logs are temporary diagnostics and should be superseded or gated by the new recorder system.
- Observability must be useful both for per-request debugging and long-horizon trend analysis.

## Engineering Alignment
- Shared `Recorder` contract across MCP, worker, and ops runner.
- Raw traces/spans/events optimized for recent diagnosis.
- Minute/hour/day rollups optimized for longer-term trend analysis.
- Dedicated ops runner owns rollups, retention, checkpoint progression, and backfill-safe maintenance.
- Robustness is favored over early simplicity because HSME is intended as long-lived core infrastructure across multiple large projects.

## Charter Check
- Charter file not found at `.kittify/charter/charter.md`.
- Charter governance check skipped for this plan.

## Gates
- [x] Scope and architecture aligned
- [x] No unresolved planning questions remain
- [x] Branch contract confirmed
- [x] Mission remains within single-project scope
- [x] Planning artifacts generated in repository root checkout

## Phase 0: Research Summary
See `kitty-specs/scalable-observability-foundation-01KQ1D4D/research.md` for final design decisions and rejected alternatives.

## Phase 1: Design Summary
Generated artifacts:
- `kitty-specs/scalable-observability-foundation-01KQ1D4D/data-model.md`
- `kitty-specs/scalable-observability-foundation-01KQ1D4D/contracts/observability-recorder.openapi.yaml`
- `kitty-specs/scalable-observability-foundation-01KQ1D4D/contracts/observability-config.schema.json`
- `kitty-specs/scalable-observability-foundation-01KQ1D4D/quickstart.md`

## Implementation Strategy

### Workstream A — Storage and schema foundation
1. Add schema migration for observability tables, indexes, views, default policies, and rollup job rows.
2. Introduce storage helpers for insert/query/upsert patterns required by traces, spans, events, rollups, and checkpoints.
3. Validate retention-safe delete order and rollup idempotency rules.

### Workstream B — Recorder library and runtime capture
1. Create a reusable recorder package implementing config loading, trace/span lifecycle, event persistence, slow-path capture, and rollup flushing hooks.
2. Integrate recorder into MCP request flow with stage-level instrumentation.
3. Integrate recorder into worker task flow with stage-level instrumentation.
4. Add feature flags/config parsing for observability level, thresholds, and sample rates.

### Workstream C — Operations runner
1. Introduce a dedicated ops runner binary or command surface for rollups, retention, and housekeeping.
2. Implement checkpointed jobs for:
   - raw → minute
   - minute → hour
   - hour → day
   - retention cleanup
3. Ensure maintenance jobs emit self-observability traces.

### Workstream D — Queryability and operational workflow
1. Add SQL views and query helpers for recent slow operations, error streams, and trend summaries.
2. Add quick operational commands/tests so maintainers can validate observability data locally.
3. Decide whether/when to surface observability via MCP-facing tools in a later mission.

## Recommended Sequencing
1. Schema migration + storage helpers
2. Recorder core package
3. MCP instrumentation
4. Worker instrumentation
5. Ops runner with rollups/checkpoints
6. Retention cleanup
7. Operational query helpers and validation

## Risks and Mitigations
- **Risk**: Raw event volume grows too quickly.
  - **Mitigation**: sampling policy rows, guaranteed retention windows, dedicated ops runner, rollup-first cleanup rules.
- **Risk**: Observability overhead harms request latency.
  - **Mitigation**: level-based capture, guaranteed preservation only for slow/error paths, measured thresholds in NFRs.
- **Risk**: Rollup corruption or duplication after interruption.
  - **Mitigation**: checkpoint rows, idempotent bucket overwrite semantics, resumable job model.
- **Risk**: Responsibility confusion between worker and ops processes.
  - **Mitigation**: strict division: worker owns semantic jobs; ops runner owns telemetry maintenance.

## Out of Scope for This Mission
- External telemetry backends
- Distributed tracing across multiple hosts
- UI dashboards
- Cross-project federation of observability data
- Advanced anomaly detection

## Ready for Tasks
This plan is ready for `/spec-kitty.tasks`.
