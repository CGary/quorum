# Contract: `search_fuzzy` and `search_exact` evolution

**Mission**: `universal-time-decay-for-search-results-01KQ4631`

This contract specifies exactly how `FuzzySearch` and `ExactSearch` change. Public signatures and MCP tool schemas are unchanged.

---

## `FuzzySearch`

### Public signature — unchanged

```go
func FuzzySearch(ctx context.Context, db *sql.DB, embedder Embedder,
                 query string, limit int, project string) ([]MemorySearchResult, error)
```

### Internal change

Modify the existing batch chunk-fetch SQL to include `m.created_at`:

```diff
- SELECT c.id, c.memory_id, c.chunk_index, c.chunk_text, m.status
+ SELECT c.id, c.memory_id, c.chunk_index, c.chunk_text, m.status, m.created_at
  FROM memory_chunks c
  JOIN memories m ON m.id = c.memory_id
  WHERE c.id IN (...)
```

Extend the `chunkMeta` struct accordingly. Then in the existing aggregation loop:

```go
cfg := GetDecayConfig()

for _, chunk := range fusedChunks {
    cm, ok := chunkByID[chunk.ID]
    if !ok { continue }
    memoryStatus[cm.memoryID] = cm.memoryStatus

    score := chunk.Score
    if cfg.Enabled {
        createdAt, err := parseCreatedAt(cm.createdAt)
        if err == nil {
            ageDays := ageInDays(time.Now(), createdAt)
            score *= DecayFactor(ageDays, cfg.HalfLifeDays)
        }
        // on parse error, score remains chunk.Score (decay=1.0 fallback)
    }

    if score > memoryScores[cm.memoryID] {
        memoryScores[cm.memoryID] = score
    }
    // highlights logic unchanged
}
```

### Byte-equivalence guarantee (NFR-007)

When `cfg.Enabled == false`:
- The `if cfg.Enabled { ... }` block is skipped entirely.
- `score` remains exactly `chunk.Score` — same value the existing code computes.
- All downstream comparisons, sorting, and serialization are unchanged.
- Output is byte-identical to pre-mission behavior.

### MCP tool schema — unchanged

`search_fuzzy` accepts the same `{query, limit, project}` parameters. The decay configuration is server-side (env var), not exposed to MCP clients.

---

## `ExactSearch`

### Public signature — unchanged

```go
func ExactSearch(ctx context.Context, db *sql.DB, keyword string,
                 limit int, project string) ([]ExactMatchResult, error)
```

### Changes to `exactSearchFTS`

**SQL**: extend SELECT to include `m.created_at` and `bm25(memory_chunks_fts) AS bm25_score`. Existing project-filter WHERE clause unchanged. ORDER BY changes from the current rank-based ordering to `bm25(memory_chunks_fts) ASC`:

```diff
- SELECT mc.id, mc.memory_id, mc.chunk_text, m.status
+ SELECT mc.id, mc.memory_id, mc.chunk_text, m.status, m.created_at,
+        bm25(memory_chunks_fts) AS bm25_score
  FROM memory_chunks_fts fts
  JOIN memory_chunks mc ON mc.id = fts.rowid
  JOIN memories m ON m.id = mc.memory_id
  WHERE memory_chunks_fts MATCH ?
    -- AND m.project = ? (when project supplied)
- ORDER BY rank
+ ORDER BY bm25(memory_chunks_fts) ASC
  LIMIT ?
```

The `ExactMatchResult` struct gains two internal fields: `createdAt string`, `bm25Score float64`. These are NOT exposed to MCP clients — they exist only inside the package for ordering decisions.

### Changes to `exactSearchSubstring`

**SQL**: extend SELECT to include `m.created_at`. The existing query and ordering remain otherwise unchanged.

For each row from this fallback, assign `bm25Score = -1e-3` (sentinel — see R-003 in research.md).

### Combined ordering rule

After both phases run:

| Decay state | Ordering |
|-------------|----------|
| `cfg.Enabled == false` | Existing behavior: FTS results first (in BM25 order), substring fallback appended in current order. **No re-sort.** |
| `cfg.Enabled == true` | All results unified. For each result: `effectiveScore = bm25Score * DecayFactor(ageDays, halfLifeDays)`. Re-sort the combined list by `effectiveScore ASC`. |

Pseudocode:
```go
func ExactSearch(...) ([]ExactMatchResult, error) {
    // ... existing fts and substring fetches ...
    results := append(ftsResults, substringResults...)

    cfg := GetDecayConfig()
    if !cfg.Enabled {
        return results[:min(limit, len(results))], nil   // existing behavior
    }

    // Decay path: assign effective scores and re-sort
    now := time.Now()
    for i := range results {
        createdAt, err := parseCreatedAt(results[i].createdAt)
        factor := 1.0
        if err == nil {
            factor = DecayFactor(ageInDays(now, createdAt), cfg.HalfLifeDays)
        }
        results[i].effectiveScore = results[i].bm25Score * factor
    }
    sort.SliceStable(results, func(i, j int) bool {
        return results[i].effectiveScore < results[j].effectiveScore
    })
    return results[:min(limit, len(results))], nil
}
```

### Byte-equivalence guarantee (NFR-007)

When `cfg.Enabled == false`:
- The whole decay branch is skipped — no `effectiveScore` computation, no re-sort.
- Output order matches the existing append behavior: FTS first (in BM25 ASC order), substring fallback after.
- Top-10 lists are byte-identical to the pre-mission baseline (validated by golden test in `tests/modules/testdata/decay_off_baseline.json`).

### MCP tool schema — unchanged

`search_exact` accepts the same `{keyword, limit, project}` parameters. Output payload shape unchanged.

---

## What does NOT change

- `recall_recent_session` (Mission 2's tool) is untouched. C-007.
- `explore_knowledge_graph` is untouched.
- `store_context` is untouched.
- The MCP tool registration in `cmd/hsme/main.go` for the existing tools needs only to add the startup-time call to `search.LoadDecayConfig()` and `search.SetDecayConfig()`. Tool handlers themselves are unchanged.
- The `MemorySearchResult` and `ExactMatchResult` exported fields are unchanged. Internal-only fields are added (`createdAt`, `bm25Score`, `effectiveScore`) but these are lowercase and not serialized to MCP responses.

---

## Defensive checks added by this mission

1. `parseCreatedAt` parse failure → log at debug level, default to factor=1.0 for that memory. The memory still ranks based on its base score, just without decay influence. No exception propagated.
2. BM25 score positivity assertion: in `exactSearchFTS`, after scanning, assert `bm25Score <= 0` per FTS5 spec. If a row violates (defensive against future SQLite changes), log warning and treat as sentinel.
3. `ORDER BY bm25(...) ASC` is the only ordering for FTS5 path. The previous ORDER BY (`rank`) is replaced by the explicit BM25 ordering — verify in Phase 3 that this does NOT change top-10 lists for the 20 frozen queries when decay is off (the byte-equivalence test catches this).
