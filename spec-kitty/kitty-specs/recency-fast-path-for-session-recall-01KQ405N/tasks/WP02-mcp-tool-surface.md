---
work_package_id: WP02
title: MCP Tool Surface
dependencies:
- WP01
requirement_refs:
- FR-001
- FR-007
- C-005
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
subtasks:
- T005
- T006
- T007
agent: "gemini:1.5-pro:architect:reviewer"
history: []
agent_profile: implementer-ivan
authoritative_surface: cmd/hsme/
execution_mode: code_change
model: gemini-1.5-pro
owned_files:
- cmd/hsme/main.go
role: implementer
tags: []
shell_pid: "1730403"
---

## ⚡ Do This First: Load Agent Profile

Use the `/ad-hoc-profile-load` skill to load the agent profile specified in the frontmatter, and behave according to its guidance before parsing the rest of this prompt.

- **Profile**: implementer-ivan
- **Role**: implementer
- **Agent/tool**: gemini

If no profile is specified, run `spec-kitty agent profile list` and select the best match for this work package's `task_type` and `authoritative_surface`.

---

## Objective

Expose the newly implemented `RecallRecentSession` function as an MCP tool `recall_recent_session`.

## Context

Agents need a direct tool to bypass semantic relevance and query chronological recent sessions. This WP connects the core function built in WP01 to the MCP API surface in `cmd/hsme/main.go`.

### Subtask T005: Register recall_recent_session tool

**Purpose**: Declare the new MCP tool in the server initialization.

**Steps**:
1. Open `cmd/hsme/main.go` and locate where tools like `search_fuzzy` and `search_exact` are registered.
2. Add `srv.RegisterTool("recall_recent_session", "Retrieve recent session summaries in chronological order", ...)`

**Files**: `cmd/hsme/main.go`
**Validation**: The tool registration compiles.

### Subtask T006: Define tool schema

**Purpose**: Describe the parameters available to the MCP client.

**Steps**:
1. Define the schema to accept two optional parameters: `project` (string) and `limit` (integer, default 5).
2. The schema should not enforce the maximum limit (50), as that is handled server-side, but it should document it in the description.

**Files**: `cmd/hsme/main.go`
**Validation**: Schema definitions use the correct types and properties.

### Subtask T007: Wire the tool handler

**Purpose**: Connect the MCP input to the `RecallRecentSession` function.

**Steps**:
1. Implement the handler closure to parse `json.RawMessage` into a struct containing `Project` and `Limit`.
2. Apply default limit if 0.
3. Call `search.RecallRecentSession(context.Background(), db, p.Limit, p.Project)`.
4. Return the wrapped results using `wrapFuzzySearchResults` or a similar wrapper that is consistent with existing tool output formats.

**Files**: `cmd/hsme/main.go`
**Validation**: The tool handler parses JSON properly, handles empty projects, calls the core function, and wraps the output correctly.

## Definition of Done
- `recall_recent_session` is registered with the MCP server.
- Optional parameters `project` and `limit` are supported.
- `search.RecallRecentSession` is called successfully, and results are returned in the expected MCP JSON format.
- Existing tools (`search_fuzzy`, `search_exact`, etc.) remain unchanged.

## Risks
- Failing to handle JSON parsing properly when parameters are omitted. Use optional types/default fallback correctly.

## Reviewer Guidance
Check that existing tools were untouched. Verify the MCP schema uses the correct object definitions and default limit handling.

## Activity Log

- 2026-04-26T05:07:41Z – gemini:1.5-pro:architect:implementer – shell_pid=1726990 – Started implementation via action command
- 2026-04-26T05:10:05Z – gemini:1.5-pro:architect:implementer – shell_pid=1726990 – MCP tool surface implemented
- 2026-04-26T05:10:15Z – gemini:1.5-pro:architect:reviewer – shell_pid=1730403 – Started review via action command
- 2026-04-26T05:10:16Z – gemini:1.5-pro:architect:reviewer – shell_pid=1730403 – Review passed: MCP tool registered
