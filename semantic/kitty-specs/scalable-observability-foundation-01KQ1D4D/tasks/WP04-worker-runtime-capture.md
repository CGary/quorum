---
work_package_id: WP04
title: Worker Runtime Capture
dependencies:
- WP01
- WP02
requirement_refs:
- FR-001
- FR-003
- FR-004
- FR-005
- FR-006
- FR-007
- FR-010
- FR-015
- NFR-004
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: main
created_at: '2026-04-25T04:47:15Z'
subtasks:
- T008
- T009
agent_profile: implementer-ivan
role: implementer
agent: "codex"
authoritative_surface: src/core/worker/
execution_mode: code_change
owned_files:
- src/core/worker/**
- cmd/worker/main.go
- tests/modules/worker_test.go
history: []
shell_pid: "1132559"
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Instrument async worker leasing and task execution so semantic processing emits correlated observability traces, spans, and failure/slow events.

## Branch Strategy
Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main.

## Execution Constraint
Do not run review or tests in this pass.

## Subtasks

### T008: Integrate recorder lifecycle into worker leasing and task execution stages
**Purpose**: Persist the internal timing and failure shape of worker task processing.
**Steps**:
1. Add recorder hooks around lease acquisition and task execution.
2. Emit spans for meaningful stages such as loading memory, embedding, graph extraction, rechunking, persistence, and completion/error handling where applicable.
3. Ensure task IDs, task types, memory IDs, and retry/failure data remain attached to traces/events.
4. Preserve current worker semantics while layering observability in.

### T009: Wire observability configuration and recorder bootstrap into the worker binary
**Purpose**: Ensure the worker process starts with the correct recorder and config.
**Steps**:
1. Load observability config in `cmd/worker/main.go`.
2. Instantiate the recorder (or no-op implementation) after DB initialization.
3. Pass recorder dependencies into the worker constructor without disturbing current model/bootstrap validation.

## Risks
- Capturing too much detail in hot worker loops.
- Forgetting to trace failure exits and retry transitions.
- Mixing semantic processing logic with observability logic too tightly.

## Definition of Done
- Worker lease/execution flow emits traces/spans/events.
- Worker binary bootstraps recorder/config.
- No review/test execution performed yet.

## Activity Log

- 2026-04-25T05:22:38Z – codex – shell_pid=1132559 – Started review via action command
- 2026-04-25T05:22:38Z – codex – shell_pid=1132559 – Approved after pragmatic shared-lane review: implementation inspected in lane-a and full go test ./... passed with sqlite_fts5 sqlite_vec tags.
