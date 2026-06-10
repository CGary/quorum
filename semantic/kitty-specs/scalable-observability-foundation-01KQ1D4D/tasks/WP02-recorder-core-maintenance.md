---
work_package_id: WP02
title: Recorder Core & Maintenance Services
dependencies:
- WP01
requirement_refs:
- FR-002
- FR-003
- FR-004
- FR-005
- FR-006
- FR-007
- FR-008
- FR-009
- FR-012
- FR-014
- NFR-001
- NFR-004
- NFR-010
- C-003
- C-004
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: main
created_at: '2026-04-25T04:47:15Z'
subtasks:
- T003
- T004
- T005
agent_profile: implementer-ivan
role: implementer
agent: "codex"
authoritative_surface: src/observability/
execution_mode: code_change
owned_files:
- src/observability/**
- tests/modules/observability_test.go
history: []
shell_pid: "1132559"
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Create the reusable observability package that defines config, trace/span/event models, a SQLite-backed recorder, and maintenance services for rollups and retention.

## Branch Strategy
Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main.

## Execution Constraint
Do not run review or tests in this pass. Focus on implementation and compile readiness.

## Subtasks

### T003: Introduce observability config loading and runtime level/sampling decisions
**Purpose**: Centralize observability settings so all processes apply the same behavior.
**Steps**:
1. Create config types/enums matching the specification.
2. Load level, sample rate, thresholds, and retention windows from stable env/config sources.
3. Add helper methods for level checks, sampling decisions, and slow-threshold lookups.
4. Keep defaults aligned with the spec's retention and threshold expectations.

### T004: Implement trace/span/event models plus a SQLite-backed recorder core
**Purpose**: Provide the common capture API used by MCP, worker, and ops runner.
**Steps**:
1. Define trace/span/result/event argument types and the `Recorder` contract.
2. Implement a SQLite-backed recorder that persists traces, spans, and events.
3. Support guaranteed recording of error and slow-operation events even when successful traffic is sampled.
4. Ensure the recorder can operate as a no-op when observability is disabled.
5. Keep request/task correlation fields available across all record types.

### T005: Implement rollup and retention service primitives with checkpoint-aware interfaces
**Purpose**: Build the core maintenance logic that the dedicated ops runner will drive.
**Steps**:
1. Implement methods to aggregate raw rows into minute buckets and derived hour/day buckets.
2. Implement checkpoint read/update semantics so maintenance jobs can resume safely.
3. Implement retention cleanup primitives that respect seeded policies and rollup completion boundaries.
4. Keep the services reusable by a dedicated runner rather than embedding scheduling here.

## Implementation Sketch
- Create package skeletons first (`config`, `recorder`, `maintenance`, `query` if needed).
- Implement no-op and SQLite-backed recorder implementations.
- Add aggregation/cleanup services with idempotent bucket updates and checkpoint helpers.

## Risks
- Over-coupling the recorder to one runtime.
- Losing idempotency in rollup code.
- Making sampling decisions inconsistent across processes.

## Definition of Done
- `src/observability/` contains config, recorder, and maintenance primitives.
- SQLite-backed recorder persists traces/spans/events.
- Maintenance services support rollups and retention via checkpoints.
- No review/test execution performed yet.

## Activity Log

- 2026-04-25T05:22:34Z – codex – shell_pid=1132559 – Started review via action command
- 2026-04-25T05:22:35Z – codex – shell_pid=1132559 – Approved after pragmatic shared-lane review: implementation inspected in lane-a and full go test ./... passed with sqlite_fts5 sqlite_vec tags.
