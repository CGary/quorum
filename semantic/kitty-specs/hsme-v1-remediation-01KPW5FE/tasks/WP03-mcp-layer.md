---
work_package_id: WP03
title: MCP Transport Layer
dependencies:
- WP02
requirement_refs:
- FR-006
planning_base_branch: master
merge_target_branch: master
branch_strategy: Planning artifacts for this feature were generated on master. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into master unless the human explicitly redirects the landing branch.
subtasks:
- T006
- T007
agent: "gemini:gemini:fast:implementer"
history: []
agent_profile: implementer-ivan
authoritative_surface: cmd/server/
execution_mode: code_change
owned_files:
- cmd/server/main.go
- src/mcp/**
role: implementer
tags: []
shell_pid: "1698034"
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Implement the missing MCP stdio server entry point and register the required tools.

## Branch Strategy
Current branch at workflow start: master. Planning/base branch for this feature: master. Completed changes must merge into master.

## Subtasks

### T006: Create `cmd/server/main.go`
**Purpose**: Create the executable entry point.
**Steps**:
1. Create `cmd/server/main.go`.
2. Initialize the SQLite database connection using `sqlite.InitDB()`.
3. Initialize the background worker in a goroutine.
4. Start an MCP stdio server loop.

### T007: Register all 4 MCP tools
**Purpose**: Connect the MCP interface to the core logic.
**Steps**:
1. Register handlers for `store_context`, `search_fuzzy`, `search_exact`, and `trace_dependencies`.
2. Connect these handlers to the `core/indexer`, `core/search`, and `core/worker` logic respectively, handling inputs and formatting JSON outputs per the spec.

## Activity Log

- 2026-04-23T03:27:56Z – gemini:gemini:fast:implementer – shell_pid=1698034 – Started implementation via action command
- 2026-04-23T03:31:14Z – gemini:gemini:fast:implementer – shell_pid=1698034 – MCP server implemented.
- 2026-04-23T03:31:14Z – gemini:gemini:fast:implementer – shell_pid=1698034 – Review passed.
