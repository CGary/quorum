# Data Model: Recency Fast Path for Session Recall

**Mission**: `recency-fast-path-for-session-recall-01KQ405N`
**Date**: 2026-04-26

---

## 1. New function: `RecallRecentSession`

**Package**: `src/core/search`
**File**: `src/core/search/recency.go`

### Signature

```go
type RecentSessionResult struct {
    ID          int64
    Project     string
    CreatedAt   string
    RawContent  string
}

func RecallRecentSession(
    ctx     context.Context,
    db      *sql.DB,
    project string,   // empty string = no filter
    limit   int,      // capped to 50 server-side
) ([]RecentSessionResult, error)
```

### SQL (canonical)

```sql
SELECT id, COALESCE(project, ''), created_at, raw_content
FROM memories
WHERE source_type = 'session_summary'
  AND status = 'active'
  AND superseded_by IS NULL
  -- conditional clause added only when project != "":
  -- AND project = ?
ORDER BY created_at DESC, id DESC
LIMIT ?
```

The Go implementation passes `min(limit, 50)` as the LIMIT argument. The `project` WHERE clause is added dynamically when `project != ""`.

### Invariants

- `limit` is clamped to `[1, 50]` before the query. A caller passing 0 or negative receives an empty result without error.
- An empty result (no matching memories) returns `([]RecentSessionResult{}, nil)` — not an error.
- The function does not call the Ollama client at any point. Any code path that introduces an embedder call is a defect (C-001).

---

## 2. New index in `src/storage/sqlite/db.go`

```sql
CREATE INDEX IF NOT EXISTS idx_memories_source_type_created
ON memories(source_type, created_at DESC);
```

Added inside `InitDB`, after the existing `idx_memories_project` line. Idempotent on every startup.

**Query pattern it covers**: The primary `recall_recent_session` query. SQLite uses this index to resolve `WHERE source_type = 'session_summary' ORDER BY created_at DESC` without a full table scan or a separate sort step.

---

## 3. MCP tool registration in `cmd/hsme/main.go`

New case in the tool-dispatch block:

```go
case "recall_recent_session":
    project, _ := args["project"].(string)
    limit := 5
    if l, ok := args["limit"].(float64); ok {
        limit = int(l)
    }
    results, err := search.RecallRecentSession(ctx, db, project, limit)
    if err != nil {
        return errorResponse(req.ID, -32603, err.Error())
    }
    return toolResponse(req.ID, formatRecentSessions(results))
```

`formatRecentSessions` formats each result using the same MCP text-content wrapper as existing tools.

---

## 4. Test cases for `tests/modules/recall_test.go`

Table-driven tests using a temp DB seeded with controlled data.

| Test name | Setup | Call | Expected outcome |
|-----------|-------|------|-----------------|
| `TestRecallRecentSession_HappyPath` | 3 session_summaries with known created_at across 3 days | `RecallRecentSession(ctx, db, "", 3)` | Returns 3 results, `results[0].CreatedAt` is the newest |
| `TestRecallRecentSession_ProjectFilter` | 3 summaries: 2 for "aibbe", 1 for "other" | `RecallRecentSession(ctx, db, "aibbe", 10)` | Returns exactly 2, both with project="aibbe" |
| `TestRecallRecentSession_EmptyResult` | No session_summaries in DB | `RecallRecentSession(ctx, db, "nonexistent", 5)` | Returns empty slice, nil error |
| `TestRecallRecentSession_LimitCap` | 60 session_summaries | `RecallRecentSession(ctx, db, "", 200)` | Returns exactly 50 |
| `TestRecallRecentSession_SupersededExcluded` | 1 active summary + 1 superseded summary | `RecallRecentSession(ctx, db, "", 10)` | Returns 1 (only active) |
| `TestRecallRecentSession_DeterministicOrder` | 2 summaries with identical created_at | `RecallRecentSession(ctx, db, "", 2)` | Result order is stable across multiple calls (id DESC tiebreaker) |
| `TestRecallRecentSession_NoEmbedderCalled` | Mock embedder that panics if called | Any call to `RecallRecentSession` | Does not panic (embedder never called) |

---

## 5. `CLAUDE.md` change — exact diff intent

**Section**: `<!-- hsme-semantic-memory-protocol -->` → `### SEARCH & RETRIEVAL STRATEGY`

**Before**:
```markdown
### SEARCH & RETRIEVAL STRATEGY
- **Discovery**: Use `mcp_hsme_search_fuzzy` for semantic experience retrieval.
- **Reference**: Use `mcp_hsme_search_exact` for precise technical lookups.
- **Dependency Tracking**: Execute `mcp_hsme_explore_knowledge_graph` during the Research phase...
```

**After** (new bullet prepended):
```markdown
### SEARCH & RETRIEVAL STRATEGY
- **Recency**: Use `recall_recent_session(project?, limit=5)` FIRST for any query whose intent is "what was the last X", "last session", "recent work", or "what did we do". Do NOT use `search_fuzzy` for recency queries — it ranks by relevance, not time.
- **Discovery**: Use `mcp_hsme_search_fuzzy` for semantic experience retrieval.
- **Reference**: Use `mcp_hsme_search_exact` for precise technical lookups.
- **Dependency Tracking**: Execute `mcp_hsme_explore_knowledge_graph` during the Research phase...
```

No other lines in `CLAUDE.md` are touched.

---

## 6. Backwards compatibility

| Change | Impact on existing callers |
|--------|---------------------------|
| New MCP tool `recall_recent_session` | None — additive. Existing tool names unchanged. |
| New DB index | None — indexes are transparent to queries. Existing queries may run faster (no regression). |
| `CLAUDE.md` update | None for code. Agents reading the protocol get new guidance. |
| Fixed test files | Tests that were broken become green. No functional logic changes. |
