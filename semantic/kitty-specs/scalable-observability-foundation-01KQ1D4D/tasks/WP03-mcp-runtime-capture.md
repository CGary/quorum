---
work_package_id: WP03
title: MCP Runtime Capture
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
- NFR-002
- NFR-003
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: main
created_at: '2026-04-25T04:47:15Z'
subtasks:
- T006
- T007
agent_profile: implementer-ivan
role: implementer
agent: "codex"
authoritative_surface: src/mcp/
execution_mode: code_change
owned_files:
- src/mcp/**
- cmd/hsme/main.go
- cmd/hsme/main_test.go
history: []
shell_pid: "1132559"
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Instrument the MCP server so request handling persists observability traces and spans for request read/parse/dispatch/handler/format/write stages, and bootstrap the recorder in the MCP binary.

## Branch Strategy
Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main.

## Execution Constraint
Do not run review or tests in this pass. Preserve current MCP behavior while adding observability.

## Subtasks

### T006: Integrate recorder lifecycle into MCP request handling stages
**Purpose**: Capture per-request latency breakdowns inside the MCP transport layer.
**Steps**:
1. Introduce recorder hooks into `src/mcp/handler.go`.
2. Map request lifecycle stages to spans/events using the new shared contract.
3. Preserve existing stderr diagnostics only if compatible with the new design or gate them behind observability level/config.
4. Ensure request IDs, tool names, and error conditions are persisted.

### T007: Wire observability configuration and recorder bootstrap into the MCP binary
**Purpose**: Ensure the MCP server starts with the correct recorder implementation and config.
**Steps**:
1. Load observability config in `cmd/hsme/main.go`.
2. Instantiate the recorder (or no-op implementation) after DB initialization.
3. Pass the recorder into the MCP server setup.
4. Keep existing tool registration behavior intact.

## Risks
- Adding latency or noise to request handling.
- Breaking MCP response behavior while instrumenting stages.
- Duplicating responsibility between handler timing logs and recorder traces.

## Definition of Done
- MCP lifecycle emits traces/spans/events through the shared recorder.
- MCP binary loads observability config and bootstraps the recorder.
- No review/test execution performed yet.

## Activity Log

- 2026-04-25T05:22:36Z – codex – shell_pid=1132559 – Started review via action command
- 2026-04-25T05:22:37Z – codex – shell_pid=1132559 – Approved after pragmatic shared-lane review: implementation inspected in lane-a and full go test ./... passed with sqlite_fts5 sqlite_vec tags.
