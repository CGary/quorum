---
work_package_id: WP03
title: Async Worker & Interfaces
dependencies:
- WP01
requirement_refs:
- FR-008
- NFR-002
planning_base_branch: master
merge_target_branch: master
branch_strategy: Planning artifacts for this feature were generated on master. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into master unless the human explicitly redirects the landing branch.
subtasks:
- T008
- T009
- T010
- T011
agent: "gemini:gemini-2.5-pro:reviewer:reviewer"
history: []
agent_profile: implementer-ivan
authoritative_surface: src/core/worker/
execution_mode: code_change
owned_files:
- src/core/worker/**
- tests/modules/worker_test.go
role: implementer
tags: []
shell_pid: "1651355"
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Develop the background polling worker that fetches tasks from `async_tasks` using a leasing mechanism, and processes embeddings and graph extraction.

## Branch Strategy
Current branch at workflow start: master. Planning/base branch for this feature: master. Completed changes must merge into master.

## Testing Constraint
**CRITICAL**: You must follow the BDD / Module-level testing strategy. Write the tests for the module FIRST to define its behavior, verify they fail, and only then write the implementation code.

## Subtasks

### T008: Create worker block tests
**Purpose**: Validate leasing and error handling mechanisms.
**Steps**:
1. Create `tests/modules/worker_test.go`.
2. Test `TestLeasingLogic`: Ensure tasks transition from `pending` to `processing` and set `leased_until` correctly. Ensure failed tasks are retired after `attempt_count` reaches 5.
3. Test `TestWorkerExecution`: Ensure completed tasks are marked `completed` and successfully write into the mock vector or node tables.

### T009: Define Embedder and GraphExtractor interfaces
**Purpose**: Decouple the local inference models from the worker.
**Steps**:
1. In `src/core/worker/interfaces.go`, define `Embedder` interface with `GenerateVector` and `Dimension()`.
2. Define `GraphExtractor` interface with `ExtractEntities`.

### T010: Implement polling worker logic
**Purpose**: Create the polling loop.
**Steps**:
1. In `src/core/worker/loop.go`, implement a routine that periodically runs an `UPDATE ... RETURNING` query to acquire tasks.
2. Filter for `status = 'pending'` or `status = 'processing' AND leased_until < :now`.

### T011: Implement `embed` and `graph_extract` execution
**Purpose**: Wire the task processors.
**Steps**:
1. In `src/core/worker/tasks.go`, implement the embed logic: use `INSERT OR REPLACE INTO memory_chunks_vec`.
2. Implement the graph extract logic: resolve `kg_nodes` with `canonical_name` and insert into `kg_edge_evidence`.
3. Validate tests pass.

## Activity Log

- 2026-04-23T02:57:47Z – gemini:gemini-2.5-pro:implementer-ivan:implementer – shell_pid=1645872 – Started implementation via action command
- 2026-04-23T03:00:36Z – gemini:gemini-2.5-pro:implementer-ivan:implementer – shell_pid=1645872 – Ready for review. Tests created.
- 2026-04-23T03:00:55Z – gemini:gemini-2.5-pro:reviewer:reviewer – shell_pid=1651355 – Started review via action command
- 2026-04-23T03:02:32Z – gemini:gemini-2.5-pro:reviewer:reviewer – shell_pid=1651355 – Review passed: worker leasing and interfaces correctly implemented.
