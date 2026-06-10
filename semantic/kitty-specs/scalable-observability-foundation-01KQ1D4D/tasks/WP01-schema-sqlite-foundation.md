---
work_package_id: WP01
title: Schema & SQLite Foundation
dependencies: []
requirement_refs:
- FR-001
- FR-011
- FR-013
- NFR-005
- NFR-006
- NFR-007
- C-001
- C-004
- C-005
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-scalable-observability-foundation-01KQ1D4D
base_commit: d7cebdc9ef3781844848799c6688ef73b0aa5a15
created_at: '2026-04-25T04:50:18.852725+00:00'
subtasks:
- T001
- T002
agent: "codex"
shell_pid: "1131805"
history: []
agent_profile: implementer-ivan
authoritative_surface: src/storage/sqlite/
execution_mode: code_change
owned_files:
- src/storage/sqlite/**
- tests/modules/storage_test.go
role: implementer
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Implement the SQLite schema and persistence foundation required for observability. This WP owns schema expansion, seeded policy rows, views, and the storage-side helper functions that the recorder and maintenance runner will depend on.

## Branch Strategy
Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main.

## Execution Constraint
Do not run review or tests in this pass. You may add or update test files only if needed for future validation, but do not execute them now.

## Subtasks

### T001: Extend SQLite schema with observability tables, views, default policies, and rollup job seeds
**Purpose**: Materialize the exact SQL contract from the specification so the rest of the system can persist traces, spans, events, rollups, retention policy rows, and job checkpoints.
**Steps**:
1. Extend `src/storage/sqlite/db.go` schema to include the observability tables and indexes from the specification.
2. Add required views for recent slow operations and error streams.
3. Seed default retention policy rows and rollup job rows idempotently during initialization.
4. Keep schema application idempotent for repeated startups.
5. Preserve existing HSME tables and startup behavior.

### T002: Add storage-level helpers for observability configuration, policy reads, and transactional write support
**Purpose**: Provide low-level DB helpers so higher-level observability packages do not duplicate SQL plumbing.
**Steps**:
1. Add helper functions/types under `src/storage/sqlite/` for loading active retention policies and job checkpoints.
2. Add helper support for observability writes that need transaction boundaries or upsert semantics.
3. Keep the helpers generic enough to support MCP, worker, and ops-runner call sites.
4. Avoid placing recorder business logic in the SQLite layer; keep it as persistence primitives only.

## Implementation Sketch
- Expand schema constants first.
- Add seed/upsert helper functions second.
- Verify all new tables can be initialized from a clean DB and that repeated initialization remains safe.

## Risks
- SQLite syntax portability around views, indexes, and upserts.
- Accidentally making startup non-idempotent.
- Mixing business logic into the storage package.

## Definition of Done
- Observability schema objects exist in `src/storage/sqlite/`.
- Default policies/checkpoints are seeded safely.
- Storage helpers exist for recorder and maintenance code to consume.
- No review/test execution performed yet.

## Activity Log

- 2026-04-25T05:22:00Z – codex – shell_pid=1131805 – Started review via action command
- 2026-04-25T05:22:29Z – codex – shell_pid=1131805 – Approved after pragmatic shared-lane review: schema/helpers implemented, diff inspected, full go test ./... passed with sqlite_fts5 sqlite_vec tags.
