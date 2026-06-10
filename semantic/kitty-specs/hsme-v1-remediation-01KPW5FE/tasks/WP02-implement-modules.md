---
work_package_id: WP02
title: Implement Missing Modules
dependencies: []
requirement_refs:
- FR-002
- FR-003
- FR-004
- FR-005
- FR-007
- NFR-001
planning_base_branch: master
merge_target_branch: master
branch_strategy: Planning artifacts for this feature were generated on master. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into master unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-hsme-v1-remediation-01KPW5FE
base_commit: 1a9cf590d320e1fdd2ae2798f22910a183b4f7ff
created_at: '2026-04-23T03:24:37.930101+00:00'
subtasks:
- T003
- T004
- T005
agent: claude
shell_pid: '1691942'
history: []
agent_profile: implementer-ivan
authoritative_surface: src/core/
execution_mode: code_change
owned_files:
- src/core/worker/**
- src/core/search/**
- tests/modules/**
role: implementer
tags: []
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Implement the `worker` and `search` core modules which were marked as done in the previous mission but were actually missing, causing test compilation failures.

## Branch Strategy
Current branch at workflow start: master. Planning/base branch for this feature: master. Completed changes must merge into master.

## Subtasks

### T003: Create `src/core/worker` module
**Purpose**: Implement the asynchronous task leasing loop.
**Steps**:
1. Create `src/core/worker/interfaces.go` with `Embedder` and `GraphExtractor` interfaces.
2. Create `src/core/worker/loop.go` with a `NewWorker` function and a `LeaseNextTask` method that queries `async_tasks` using the 5-minute lease logic.
3. Create `src/core/worker/tasks.go` with an `ExecuteTask` method.

### T004: Create `src/core/search` module
**Purpose**: Implement the Reciprocal Rank Fusion (RRF) and dependency tracing logic.
**Steps**:
1. Create `src/core/search/fuzzy.go` implementing `search_fuzzy` logic (combining FTS5 and `vec0` queries with RRF score `1/(60+rank)`).
2. Create `src/core/search/graph.go` implementing `trace_dependencies` using a recursive CTE (`WITH RECURSIVE`).

### T005: Ensure tests compile and pass
**Purpose**: Fix the compilation errors reported in the mission review.
**Steps**:
1. Ensure the package names and imports in `tests/modules/worker_test.go` and `tests/modules/search_test.go` correctly align with the newly created modules.
2. Run `go test ./tests/modules/...` (or simulate it). The code should be structurally complete even if local `vec0` failures occur.

## Activity Log

- 2026-04-23T03:27:05Z – claude – shell_pid=1691942 – Modules implemented.
- 2026-04-23T03:27:11Z – claude – shell_pid=1691942 – Review passed.
