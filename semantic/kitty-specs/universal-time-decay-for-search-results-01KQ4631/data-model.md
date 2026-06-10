# Data Model: Universal Time-Decay for Search Results

**Mission**: `universal-time-decay-for-search-results-01KQ4631`
**Date**: 2026-04-26

This mission introduces no schema changes. All entities below are either pure functions (no persistence) or new operational artifacts (benchmark runs).

---

## 1. Pure decay function

**Package**: `src/core/search`
**File**: `src/core/search/decay.go`

### Signature

```go
// DecayFactor returns a multiplier in (0, 1] given a memory's age in days
// and the configured half-life. Negative ages clamp to 0, yielding 1.0.
func DecayFactor(ageDays float64, halfLifeDays float64) float64
```

### Math

```
factor = 0.5 ^ (max(0, age_days) / half_life_days)
```

Implementation uses `math.Pow(0.5, math.Max(0, ageDays)/halfLifeDays)`. Pure function, no I/O, no state.

### Edge cases

| Input | Output |
|-------|--------|
| `ageDays = 0`, any `halfLifeDays` | `1.0` |
| `ageDays = halfLifeDays` | `0.5` |
| `ageDays = 2 * halfLifeDays` | `0.25` |
| `ageDays = -5` (future timestamp) | `1.0` (clamped to 0 via `max`) |
| `halfLifeDays = 0` | undefined — caller MUST validate at config-load time |
| `halfLifeDays < 0` | undefined — caller MUST validate at config-load time |

The function does NOT validate `halfLifeDays`. Validation is the responsibility of `LoadDecayConfig()` (see §2). Any invocation with `halfLifeDays <= 0` is a programming error.

---

## 2. Configuration loading

**Package**: `src/core/search`
**File**: `src/core/search/decay.go`

### Type

```go
type DecayConfig struct {
    Enabled       bool
    HalfLifeDays  float64
}
```

### Loader

```go
// LoadDecayConfig reads RRF_TIME_DECAY and RRF_HALF_LIFE_DAYS from the environment,
// validates them, and returns a DecayConfig. On invalid input it returns an error.
func LoadDecayConfig() (DecayConfig, error)
```

### Validation rules

| Env var | Accepted values | Default | Failure mode |
|---------|----------------|---------|--------------|
| `RRF_TIME_DECAY` | `on`, `off` (case-sensitive) | `off` | Any other value → error like `RRF_TIME_DECAY: invalid value '<x>'; expected 'on' or 'off'` |
| `RRF_HALF_LIFE_DAYS` | Positive number (int or float, parsable by `strconv.ParseFloat`) | `14` | Empty: use default. Non-numeric: error. Zero or negative: error like `RRF_HALF_LIFE_DAYS: must be > 0; got <x>` |

### Wiring point

`cmd/hsme/main.go` calls `search.LoadDecayConfig()` once at startup, immediately after the existing init steps. On error, log to stderr and `os.Exit(2)`. On success, store via `search.SetDecayConfig(cfg)` (package-level variable).

---

## 3. Search package state

**Package-level**:
```go
var decayCfg = DecayConfig{Enabled: false, HalfLifeDays: 14}

func SetDecayConfig(cfg DecayConfig)   // setter, called once at startup
func GetDecayConfig() DecayConfig      // getter, used by FuzzySearch / ExactSearch
```

The package-level variable is set ONCE before the MCP server starts serving requests. Concurrent reads are race-free under the Go memory model (sequential program order before goroutine spawn = happens-before).

---

## 4. `FuzzySearch` modifications

**Existing signature** (unchanged):
```go
func FuzzySearch(ctx context.Context, db *sql.DB, embedder Embedder,
                 query string, limit int, project string) ([]MemorySearchResult, error)
```

**Internal change** — pseudocode:
```go
cfg := GetDecayConfig()
// ... existing code through fusedChunks computation ...

// Modified batch-fetch SQL: add m.created_at
chunkRows, err := db.QueryContext(ctx, fmt.Sprintf(`
    SELECT c.id, c.memory_id, c.chunk_index, c.chunk_text, m.status, m.created_at
      FROM memory_chunks c
      JOIN memories m ON m.id = c.memory_id
     WHERE c.id IN (%s)`, placeholders), chunkIDs...)
// ... scan now includes createdAt per chunk ...

// Modified aggregation loop:
for _, chunk := range fusedChunks {
    cm, ok := chunkByID[chunk.ID]
    if !ok { continue }
    memoryStatus[cm.memoryID] = cm.memoryStatus

    score := chunk.Score
    if cfg.Enabled {                                              // <-- explicit branch
        ageDays := ageInDays(time.Now(), cm.createdAt)
        score *= DecayFactor(ageDays, cfg.HalfLifeDays)
    }

    if score > memoryScores[cm.memoryID] {
        memoryScores[cm.memoryID] = score
    }
    // ... highlights logic unchanged ...
}
```

When `cfg.Enabled == false`, the entire `if` block is skipped — the existing `chunk.Score` flows unchanged into the aggregation. Byte-equivalence with pre-mission behavior is preserved by construction.

---

## 5. `ExactSearch` modifications

**Existing signature** (unchanged):
```go
func ExactSearch(ctx context.Context, db *sql.DB, keyword string,
                 limit int, project string) ([]ExactMatchResult, error)
```

### `exactSearchFTS` changes

**Current** (simplified):
```sql
SELECT mc.id, mc.memory_id, mc.chunk_text, m.status
FROM memory_chunks_fts fts
JOIN memory_chunks mc ON mc.id = fts.rowid
JOIN memories m ON m.id = mc.memory_id
WHERE memory_chunks_fts MATCH ?
  -- AND m.project = ? (when project supplied)
ORDER BY rank
LIMIT ?
```

**With decay support**:
```sql
SELECT mc.id, mc.memory_id, mc.chunk_text, m.status, m.created_at,
       bm25(memory_chunks_fts) AS bm25_score
FROM memory_chunks_fts fts
JOIN memory_chunks mc ON mc.id = fts.rowid
JOIN memories m ON m.id = mc.memory_id
WHERE memory_chunks_fts MATCH ?
  -- AND m.project = ? (when project supplied)
ORDER BY bm25(memory_chunks_fts)
LIMIT ?
```

Then, when `cfg.Enabled`:
```go
ageDays := ageInDays(time.Now(), result.createdAt)
result.effectiveScore = result.bm25Score * DecayFactor(ageDays, cfg.HalfLifeDays)
```

When `cfg.Enabled == false`, no `effectiveScore` is computed, no re-ordering happens. The current `ORDER BY rank` (or the equivalent) controls output order — byte-equivalent to pre-mission.

### `exactSearchSubstring` changes

**Current**: returns rows ordered by some combination of MATCH-rank fallback / id. No explicit relevance score.

**With decay support**:
- Each row receives sentinel score `-1e-3`.
- When `cfg.Enabled`, multiply sentinel by `DecayFactor(ageDays, halfLifeDays)`.
- Combined list (FTS results + substring fallback) is re-sorted by effective score ASC.
- When `cfg.Enabled == false`, NO re-sort happens — the existing append order (FTS first, substring after) is preserved.

### Combined result ordering invariant

When decay is OFF: order is `<FTS results in BM25/MATCH order> ++ <substring fallback in current order>`.
When decay is ON: order is `<all results sorted by effective_score ASC>`.

Smoke test in plan: with `RRF_HALF_LIFE_DAYS=1e9` (effectively infinite half-life, decay factor → 1.0 for all ages), decay-on output equals decay-off output. This is a sanity check, not a functional test.

---

## 6. Benchmark Run Report

**Location**: `data/benchmarks/<run_id>/` where `run_id = <ISO-timestamp>-<flag-state>`.

### Files per run

| File | Format | Purpose |
|------|--------|---------|
| `report.json` | JSON | Machine-readable summary, mirrors structure of `docs/future-missions/mission-3-baseline.json` |
| `report.md` | Markdown | Human summary, ≤ 2 pages |
| `delta.json` | JSON | Paired comparison off→on: per-query rank delta + summary |

### `report.json` schema

```json
{
  "schema_version": 1,
  "run_id": "20260427T100000Z-on",
  "started_at": "2026-04-27T10:00:00Z",
  "finished_at": "2026-04-27T10:00:42Z",
  "tool_version": "<commit SHA at run time>",
  "config": {
    "rrf_time_decay": "on",
    "rrf_half_life_days": 14
  },
  "corpus_snapshot": {
    "hsme_db": "/home/gary/dev/hsme/data/engram.db",
    "total_memories": 1004,
    "min_created_at": "...",
    "max_created_at": "..."
  },
  "results": [
    {
      "id": "rec-01",
      "category": "pure_recency",
      "tool": "search_fuzzy",
      "query": "what did we do in the last session",
      "expected_winner_id": 994,
      "actual_top_10_ids": [994, ...],
      "expected_winner_rank": 1,
      "in_top_10": true, "in_top_3": true, "in_top_1": true
    }
    // ... and additional entries for "tool": "search_exact" on the eval-subset that runs through search_exact (≥5 queries per FR-012)
  ],
  "summary": {
    "by_tool_and_category": {
      "search_fuzzy": {
        "pure_recency":   { "n": 5, "top1_hit_rate": ..., "top3_hit_rate": ..., "top10_hit_rate": ... },
        // ... other categories
      },
      "search_exact": {
        "subset_size": 5,
        "top10_hit_rate": ...
      }
    }
  }
}
```

### `delta.json` schema

```json
{
  "schema_version": 1,
  "off_run": "20260427T095900Z-off",
  "on_run":  "20260427T100000Z-on",
  "deltas": [
    {
      "id": "rec-01",
      "tool": "search_fuzzy",
      "rank_off": null,
      "rank_on": 1,
      "delta": "+missing→1",
      "in_top_3_off": false,
      "in_top_3_on": true
    }
    // ... one entry per query × tool
  ],
  "summary": {
    "queries_promoted_into_top_3": 8,
    "queries_demoted_out_of_top_3": 0,
    "adversarial_top_3_off": 0.80,
    "adversarial_top_3_on":  0.80
  }
}
```

The `summary.adversarial_top_3_on >= 0.80` is the NFR-003 audit point — the harness output IS the proof of the NFR.

---

## 7. New environment variables (operator-facing)

| Variable | Purpose | Validation | Default | Failure mode |
|----------|---------|-----------|---------|--------------|
| `RRF_TIME_DECAY` | Master switch for decay across both search tools | Must be `on` or `off` | `off` | Process refuses to start (exit 2) |
| `RRF_HALF_LIFE_DAYS` | Decay rate (days for factor to halve) | Must be a positive number | `14` | Process refuses to start (exit 2) |

Both vars are read once at `cmd/hsme/main.go` startup. The `cmd/bench-decay` binary reads them too, with the same validation, so operators can run the harness with explicit overrides:

```bash
RRF_TIME_DECAY=on RRF_HALF_LIFE_DAYS=7 ./bench-decay --paired
```

---

## 8. Backwards compatibility audit

| Surface | Impact | Behavior with `RRF_TIME_DECAY=off` |
|---------|--------|-----------------------------------|
| MCP `search_fuzzy` JSON-RPC contract | None — additive | Byte-identical to pre-mission |
| MCP `search_exact` JSON-RPC contract | None — additive | Byte-identical to pre-mission |
| MCP `recall_recent_session` | Untouched (C-007) | Identical |
| MCP `explore_knowledge_graph` | Untouched | Identical |
| Database schema | None | Identical |
| Existing test suite | Must continue to pass with default env | Tests run with `RRF_TIME_DECAY=off` (the default), preserving golden behavior |
| README | New "Time-Decay Configuration" section | N/A |

---

## 9. What this mission does NOT model

- The decay function does not learn or auto-tune. The half-life is a knob the operator turns based on the harness output.
- There is no per-memory or per-source-type decay override. One global half-life only.
- There is no telemetry tracking decay's impact in production runs (the harness is the only measurement surface).
- There is no result-level field exposed to MCP clients indicating "this memory was decayed by factor X". Decay is internal to the ranking; clients see only the final ordering.
