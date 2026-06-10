---
work_package_id: WP04
title: Search Project Filter Surface
dependencies:
- WP01
requirement_refs:
- C-006
- FR-006
- NFR-004
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-engram-legacy-cutover-and-corpus-restoration-01KQ2SJK
created_at: '2026-04-26T02:52:52Z'
subtasks:
- T015
- T016
- T017
agent: "gemini:1.5-pro:architect:reviewer"
shell_pid: "1596609"
history: []
agent_profile: implementer-ivan
authoritative_surface: src/core/search/
execution_mode: code_change
owned_files:
- src/core/search/fuzzy.go
- src/mcp/handler.go
- cmd/hsme/main.go
- tests/modules/search_project_filter_test.go
role: implementer
tags: []
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Add the optional `project` filter to `search_fuzzy` and `search_exact` end-to-end while preserving the existing unfiltered behavior, payload shape, and MCP compatibility.

## Branch Strategy
Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge back into main. Execution worktrees will later be allocated per computed lane from `lanes.json`; do not assume this WP runs directly in the root checkout.

## Context
This WP is intentionally additive. Existing clients that omit `project` must behave exactly as before. The filter should constrain both lexical and semantic search legs consistently so cross-project results cannot leak through ranking/fusion.

## Detailed Guidance

### T015: Add optional `project` filtering to core `FuzzySearch` and `ExactSearch` SQL paths
**Purpose**: Implement the actual restriction logic in the search core.

**Steps**:
1. Update the core search function signatures to accept `project string`.
2. Apply the filter at the `memories` table level through the SQL queries/join paths.
3. Treat empty string / omitted project as "no filter".
4. Ensure both lexical and semantic candidate paths in fuzzy search use the same filter.
5. Preserve existing ranking/order semantics when no filter is present.

### T016: Thread the optional `project` argument through MCP tool schemas and handlers
**Purpose**: Expose the filter without breaking existing MCP callers.

**Steps**:
1. Update MCP tool schemas/registration so `project` appears as an optional input field for `search_fuzzy` and `search_exact`.
2. Extract the argument safely in handlers with empty-string fallback when missing.
3. Pass the argument into the updated core search functions.
4. Keep JSON-RPC errors unchanged; invalid/missing `project` should not create a new required-path failure.

### T017: Add search tests for filtered, unfiltered, and empty-result queries, including a rough latency guard
**Purpose**: Prove the additive filter behaves correctly and does not blow up latency.

**Steps**:
1. Add `tests/modules/search_project_filter_test.go`.
2. Cover:
   - unfiltered calls matching baseline behavior
   - project-filtered calls returning only the target project
   - nonexistent project returning zero results cleanly
   - both fuzzy and exact paths
3. Add a coarse latency assertion/bound that is robust enough not to be flaky.
4. Use fixture data that clearly separates two projects.

## Implementation Sketch
- Extend core search signatures/SQL first.
- Update MCP schemas and handlers second.
- Finish with fixtures and tests.

## Risks
- Applying the filter in only one search leg.
- Breaking existing callers by making `project` required.
- Accidentally changing response shape instead of just filtering behavior.

## Definition of Done
- Both search tools accept optional `project`.
- Unfiltered behavior remains stable.
- Filtered results stay within the target project.
- Tests cover behavior and a rough performance bound.

## Reviewer Guidance
Verify that:
- omitted `project` is a no-op
- both fuzzy and exact paths enforce the same filter semantics
- no unrelated MCP contracts changed

## Activity Log

- 2026-04-26T03:29:48Z – gemini:1.5-pro:architect:implementer – shell_pid=1589202 – Started implementation via action command
- 2026-04-26T03:35:01Z – gemini:1.5-pro:architect:implementer – shell_pid=1589202 – Search project filter surface implemented. search_fuzzy and search_exact now accept optional project argument, and StoreContext now persists project metadata.
- 2026-04-26T03:35:14Z – gemini:1.5-pro:architect:reviewer – shell_pid=1596609 – Started review via action command
- 2026-04-26T03:35:28Z – gemini:1.5-pro:architect:reviewer – shell_pid=1596609 – Review passed: Project filtering is correctly implemented across all search paths (lexical, semantic, and exact) and exposed via MCP tools without breaking existing contracts.
