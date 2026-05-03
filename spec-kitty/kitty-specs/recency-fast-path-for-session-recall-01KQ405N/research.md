# Phase 0 Research: Recency Fast Path for Session Recall

**Mission**: `recency-fast-path-for-session-recall-01KQ405N`
**Date**: 2026-04-26

All decisions were locked during spec discovery. No unresolved clarification markers were generated. This document records the rationale for each choice for future traceability.

---

## R-001: New file `recency.go` vs extending `fuzzy.go`

**Decision**: New file `src/core/search/recency.go`.

**Rationale**: `fuzzy.go` already hosts `FuzzySearch` and `ExactSearch` (315 lines). Adding a third conceptually distinct function (chronological lookup, no embedder) into the same file makes the search package harder to navigate. A dedicated file makes the "no embedder, pure SQL" invariant obvious from the filename alone. Pattern already established in the repo: `fuzzy.go` (vector+lexical), `graph.go` (graph traversal), `recency.go` (chronological).

**Alternatives considered**: Appending to `fuzzy.go`. Simpler diff, but conflates semantic and chronological search under one file.

---

## R-002: SQL query design for `recall_recent_session`

**Decision**:
```sql
SELECT id, source_type, project, created_at, raw_content
FROM memories
WHERE source_type = 'session_summary'
  AND status = 'active'
  AND superseded_by IS NULL
  [AND project = ?]       -- conditional on project param
ORDER BY created_at DESC, id DESC
LIMIT ?                   -- always capped to min(requested, 50)
```

**Rationale**:
- `source_type = 'session_summary'` — canonical filter per FR-002 and the domain language definition.
- `status = 'active'` — excludes deleted/superseded memories already marked at the row level.
- `superseded_by IS NULL` — belt-and-suspenders exclusion of superseded entries even when `status` has not been updated yet (defensive against race conditions).
- `created_at DESC, id DESC` — primary sort on time, secondary sort on PK for deterministic ordering when two rows share the same timestamp (C-002 tiebreaker).
- The index `(source_type, created_at DESC)` covers the first WHERE clause and the ORDER BY entirely. SQLite can serve this query with an index scan rather than a full table scan + sort.

**Alternatives considered**:
- Sorting only by `id DESC`: cheaper but incorrect — imported memories from legacy have accurate `created_at` that is more meaningful than insertion order.
- Using `updated_at DESC`: wrong semantics — a memory updated later is not necessarily newer.

---

## R-003: Server-side cap at 50

**Decision**: The cap is enforced in the SQL (`LIMIT MIN(limit, 50)` expressed as Go-side `min(requestedLimit, 50)` before passing to the query). Not enforced by MCP schema validation alone.

**Rationale**: Schema validation can be bypassed or can fail silently in some MCP clients. Enforcing the cap in the SQL is the only guarantee that the DB never returns an unbounded result set. The MCP schema documents `maximum: 50` for informational/client-side purposes.

**Alternatives considered**: Returning an error on `limit > 50`. Rejected — degrading silently to 50 is more ergonomic and the cap is documented.

---

## R-004: Where the index lives in db.go

**Decision**: Add the new `CREATE INDEX IF NOT EXISTS idx_memories_source_type_created ON memories(source_type, created_at DESC)` call inside `InitDB`, immediately after the existing `idx_memories_project` unconditional creation (line ~407 post-Mission-1).

**Rationale**: All schema evolution for the `memories` table lives in `InitDB`. This index is idempotent (`IF NOT EXISTS`) and cheap to check on startup. Adding it outside `InitDB` would create a split between schema setup and index setup that would confuse future maintainers.

**Alternatives considered**: Running it as a one-off migration in `cmd/migrate-legacy`. Rejected — this index is a permanent part of the schema, not a one-time data operation.

---

## R-005: Broken test fix strategy

**Decision** (chosen by user during planning, Option A): Fix the 5 broken test files as part of Mission 2. The fix is mechanical: add `""` as the `project` argument to all `StoreContext`, `FuzzySearch`, and `ExactSearch` calls in tests. No test logic changes.

**Rationale**: Mission 2's success criterion #8 requires all existing tests to pass. The 5 broken files prevent `go test ./tests/modules/...` from compiling at all, which means there is no CI green state to verify against. Fixing them is a prerequisite for any meaningful test run.

**Files and exact changes needed**:
| File | Issue | Fix |
|------|-------|-----|
| `tests/modules/search_test.go` | `StoreContext` missing `project`; `FuzzySearch` missing `project`; `ExactSearch` missing `project` | Add `""` as 4th arg to `StoreContext`, 6th arg to `FuzzySearch`, 5th arg to `ExactSearch` |
| `tests/modules/indexer_test.go` | `StoreContext` missing `project` (×4) | Add `""` as 4th arg |
| `tests/modules/search_project_filter_test.go` | Unused `"database/sql"` import; unused `r` variable; `StoreContext` missing `project` | Remove import, fix unused var, add `""` |
| `tests/modules/migrate_legacy_orphans_test.go` | Unused `"database/sql"` and `"path/filepath"` imports | Remove both imports |
| `tests/modules/migrate_legacy_restore_test.go` | Unused `sqlite` import | Remove import |

**Alternatives considered**: Creating a separate "test-debt cleanup" mission. Rejected — the debt is trivially small and mechanically fixed. Bundling it here avoids carrying a broken test suite indefinitely.

---

## R-006: `CLAUDE.md` update scope

**Decision**: Add one paragraph to the HSME protocol section (`<!-- hsme-semantic-memory-protocol -->` block) in `CLAUDE.md`. No other section touched.

**Rationale**: The HSME protocol section already contains mandatory save/search triggers. Adding the `recall_recent_session` guidance there is the natural place — agents reading the protocol will see both the save triggers and the retrieval strategy in one place.

The exact addition goes into the `### SEARCH & RETRIEVAL STRATEGY` subsection:
- **Recency**: Use `recall_recent_session(project?, limit=5)` FIRST for any query whose intent is "what was the last X" — do NOT use `search_fuzzy` for recency queries.

**Alternatives considered**: Adding to a new section. Rejected — the existing structure covers it.

---

## Open Items — None

All decisions resolved before or during planning. Phase 1 can proceed.
