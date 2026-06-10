---
work_package_id: WP01
title: Core Query and Index
dependencies: []
requirement_refs:
- FR-002
- FR-003
- FR-004
- FR-005
- FR-008
- C-001
- C-002
- C-003
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-recency-fast-path-for-session-recall-01KQ405N
base_commit: d0ee23282d7ed64afd36c0b94b96765df778cac6
created_at: '2026-04-26T05:01:51.281709+00:00'
subtasks:
- T001
- T002
- T003
- T004
agent: "gemini:1.5-pro:architect:reviewer"
shell_pid: "1726383"
history: []
agent_profile: implementer-ivan
authoritative_surface: src/core/search/
execution_mode: code_change
model: gemini-1.5-pro
owned_files:
- src/storage/sqlite/db.go
- src/core/search/recency.go
role: implementer
tags: []
---

## ⚡ Do This First: Load Agent Profile

Use the `/ad-hoc-profile-load` skill to load the agent profile specified in the frontmatter, and behave according to its guidance before parsing the rest of this prompt.

- **Profile**: implementer-ivan
- **Role**: implementer
- **Agent/tool**: gemini

If no profile is specified, run `spec-kitty agent profile list` and select the best match for this work package's `task_type` and `authoritative_surface`.

---

## Objective

Implement the core SQL query for fetching recent sessions and add the supporting database index.

## Context

Semantic search ranks results by relevance, not time. When an agent asks "what did we do last session?", it needs chronological order. This WP implements the `RecallRecentSession` function using pure SQL (no embeddings) and adds a composite index on `(source_type, created_at DESC)` to support it.

### Subtask T001: Add composite index

**Purpose**: Add the database index required for fast chronological lookups.

**Steps**:
1. Modify `src/storage/sqlite/db.go` in the database initialization section.
2. Add the SQL to create the composite index: `CREATE INDEX IF NOT EXISTS idx_memories_source_type_created ON memories(source_type, created_at DESC);`
3. Ensure it runs idempotently during DB setup.

**Files**: `src/storage/sqlite/db.go`
**Validation**: The index should be created without errors on fresh and existing databases.

### Subtask T002: Create recency search file

**Purpose**: Scaffold the new file for recency search.

**Steps**:
1. Create `src/core/search/recency.go`.
2. Add the `package search` declaration and necessary imports (`context`, `database/sql`, etc.).

**Files**: `src/core/search/recency.go`
**Validation**: File compiles.

### Subtask T003: Implement RecallRecentSession

**Purpose**: Implement the core SQL query.

**Steps**:
1. Add `func RecallRecentSession(ctx context.Context, db *sql.DB, limit int, project string) ([]MemorySearchResult, error)` to `recency.go`.
2. Ensure the query filters by `source_type='session_summary'` and `status='active'`.
3. Exclude superseded memories: `superseded_by IS NULL`.

**Files**: `src/core/search/recency.go`
**Validation**: Query logic matches requirements.

### Subtask T004: Apply project filter, limit, and ordering

**Purpose**: Enforce the specific constraints and ordering for the query.

**Steps**:
1. If `project` is provided, add `AND project = ?` to the `WHERE` clause.
2. Add `ORDER BY created_at DESC, id DESC`.
3. Cap `limit` at 50 server-side: `if limit > 50 { limit = 50 }` and `if limit <= 0 { limit = 5 }`.
4. Return the results mapped to `MemorySearchResult`.

**Files**: `src/core/search/recency.go`
**Validation**: Query uses the capped limit and orders correctly.

## Definition of Done
- `db.go` includes the new composite index creation.
- `recency.go` contains `RecallRecentSession` with correct SQL, parameters, and return type.
- Limit is capped at 50.
- No embedder is invoked.

## Risks
- Index creation syntax errors might break DB startup. Use `IF NOT EXISTS`.
- SQL injection if `project` is not parameterized (use `?`).

## Reviewer Guidance
Check that the SQL query exactly matches the constraints (active, session_summary, not superseded) and that the limit is enforced server-side.

## Activity Log

- 2026-04-26T05:07:02Z – gemini – shell_pid=1719731 – Core SQL query for fetching recent sessions and index are implemented.
- 2026-04-26T05:07:15Z – gemini:1.5-pro:architect:reviewer – shell_pid=1726383 – Started review via action command
- 2026-04-26T05:07:16Z – gemini:1.5-pro:architect:reviewer – shell_pid=1726383 – Review passed: pure SQL query implemented, index added correctly.
