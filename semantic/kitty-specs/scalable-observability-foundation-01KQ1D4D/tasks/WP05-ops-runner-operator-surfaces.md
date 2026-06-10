---
work_package_id: WP05
title: Ops Runner & Operator Surfaces
dependencies:
- WP01
- WP02
- WP03
- WP04
requirement_refs:
- FR-009
- FR-013
- FR-014
- FR-015
- NFR-005
- NFR-006
- NFR-007
- NFR-008
- NFR-009
- C-002
- C-005
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: main
created_at: '2026-04-25T04:47:15Z'
subtasks:
- T010
- T011
- T012
agent_profile: implementer-ivan
role: implementer
agent: "codex"
authoritative_surface: cmd/ops/
execution_mode: code_change
owned_files:
- cmd/ops/**
- justfile
- README.md
- kitty-specs/scalable-observability-foundation-01KQ1D4D/quickstart.md
history: []
shell_pid: "1132559"
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Introduce the dedicated operations runner that executes observability rollups, retention, and housekeeping, and expose practical operator surfaces/build wiring for local use.

## Branch Strategy
Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main.

## Execution Constraint
Do not run review or tests in this pass. Focus on implementing runnable maintenance and operator-facing hooks.

## Subtasks

### T010: Add a dedicated operations runner binary for rollups, retention, and housekeeping loops
**Purpose**: Create a cleanly separated process boundary for observability maintenance.
**Steps**:
1. Add a new command entrypoint for the operations runner.
2. Load DB, observability config, and maintenance services.
3. Support at least one continuous/scheduled execution mode suitable for local operation.

### T011: Implement maintenance execution flow that emits self-observability traces and advances checkpoints
**Purpose**: Make rollups and cleanup both functional and themselves observable.
**Steps**:
1. Execute raw→minute, minute→hour, hour→day, and retention cleanup jobs from the runner.
2. Advance checkpoints safely.
3. Emit maintenance traces/events so ops work is diagnosable like any other subsystem.

### T012: Expose operator-oriented query helpers/commands and update local operational workflow docs/build wiring
**Purpose**: Make the new subsystem usable by maintainers without external services.
**Steps**:
1. Add basic operator command surfaces or summaries that expose recent slow operations, error streams, or rollup health.
2. Update build/install wiring if new binaries are introduced.
3. Update README/quickstart usage notes to explain how to run the ops runner.
4. Do not implement review/test automation in this pass.

## Risks
- Overcomplicating the runner lifecycle.
- Failing to make maintenance self-observable.
- Forgetting to wire build/install commands for the new binary.

## Definition of Done
- Dedicated ops runner exists and can drive rollups/retention.
- Maintenance emits its own observability traces/events.
- Operator-facing usage/build docs are updated.
- No review/test execution performed yet.

## Activity Log

- 2026-04-25T05:22:39Z – codex – shell_pid=1132559 – Started review via action command
- 2026-04-25T05:22:40Z – codex – shell_pid=1132559 – Approved after pragmatic shared-lane review: implementation inspected in lane-a and full go test ./... passed with sqlite_fts5 sqlite_vec tags.
