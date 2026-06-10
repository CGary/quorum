# Contract: MCP `search_fuzzy` and `search_exact` — `project` filter extension

**Mission**: `engram-legacy-cutover-and-corpus-restoration-01KQ2SJK`

This mission adds an optional `project` parameter to the two existing MCP search tools. The change is **additive** and **backwards-compatible**: omitting the parameter preserves today's behavior exactly.

---

## `search_fuzzy`

### Updated signature

```jsonc
{
  "name": "search_fuzzy",
  "description": "Hybrid lexical + semantic search with RRF fusion",
  "inputSchema": {
    "type": "object",
    "properties": {
      "query":   { "type": "string", "description": "Natural-language query" },
      "limit":   { "type": "integer", "default": 10, "minimum": 1, "maximum": 100 },
      "project": { "type": "string", "description": "Optional. Restrict results to memories with memories.project = this value." }
    },
    "required": ["query"]
  }
}
```

### Behavior

- **`project` omitted or empty string**: behavior identical to today. Search spans the full corpus.
- **`project` provided and non-empty**: the SQL applied at the memory-level WHERE adds `AND m.project = ?`. Only chunks belonging to memories of that project participate in RRF fusion.
- The filter is applied at the `memories` table level via JOIN, BEFORE chunk-level scoring, so:
  - Chunks from other projects cannot leak via aggregation.
  - The candidate set for both lexical and semantic legs of RRF is identical.

### Function-core change

In `src/core/search/fuzzy.go`:

```go
// Before
func FuzzySearch(ctx context.Context, db *sql.DB, embedder Embedder, query string, limit int) ([]Result, error)

// After
func FuzzySearch(ctx context.Context, db *sql.DB, embedder Embedder, query string, limit int, project string) ([]Result, error)
```

Pass `project=""` from existing call sites; the empty-string check inside the function bypasses the WHERE clause addition.

### Latency budget

Per NFR-004: P50 latency must remain within ±10% of the pre-mission baseline. The added WHERE clause uses `idx_memories_project`, so worst-case impact is one additional index lookup per memory candidate. Expected delta: <1ms on the production corpus size.

---

## `search_exact`

### Updated signature

```jsonc
{
  "name": "search_exact",
  "description": "Lexical FTS5 keyword search",
  "inputSchema": {
    "type": "object",
    "properties": {
      "keyword": { "type": "string", "description": "Keyword or phrase to match" },
      "limit":   { "type": "integer", "default": 10, "minimum": 1, "maximum": 100 },
      "project": { "type": "string", "description": "Optional. Restrict results to memories with memories.project = this value." }
    },
    "required": ["keyword"]
  }
}
```

### Behavior

- Same as `search_fuzzy`: optional, additive, backwards-compatible.
- Existing FTS5 MATCH semantics unchanged.
- The filter is applied via JOIN with `memories.id = mc.memory_id` plus `WHERE memories.project = ?`.

### Function-core change

In `src/core/search/fuzzy.go` (the function lives in the same file as `FuzzySearch`):

```go
// Before
func ExactSearch(ctx context.Context, db *sql.DB, keyword string, limit int) ([]Result, error)

// After
func ExactSearch(ctx context.Context, db *sql.DB, keyword string, limit int, project string) ([]Result, error)
```

---

## What this mission does NOT change

- `explore_knowledge_graph` is not modified. The graph is intentionally cross-project (architectural relations span projects).
- `store_context` is not modified. Today it does not accept a `project` parameter; future-you can add one in a later mission, but for now the migrator handles project assignment for the orphans, and new ingestions through MCP will leave `project` NULL until that mission lands. This is documented in the README update.
- Result ordering: this mission does not change RRF or ranking. That belongs to Mission 3 (RRF time-decay).
- Result payload shape: each `Result` row gains nothing new in this mission. `project` is filterable but not exposed in the response (it lives in `raw_content` already as the wrapper line).

## Tests required

Under `tests/modules/search_test.go`:

1. `search_fuzzy(query, limit, project="")` returns identical results to the pre-mission behavior — golden test against a fixture corpus.
2. `search_fuzzy(query, limit, project="aibbe")` returns ONLY rows whose `memories.project='aibbe'`.
3. `search_fuzzy(query, limit, project="nonexistent")` returns 0 results without error.
4. Mirror tests for `search_exact`.
5. Latency: `search_fuzzy` with `project` filter completes within 10% of the unfiltered call on the same fixture (rough — set a generous bound to avoid CI flakiness, e.g., +50ms).

## MCP handler change

In `src/mcp/handler.go`, the tool registration in `cmd/hsme/main.go` updates to pull the optional `project` argument:

```go
proj, _ := args["project"].(string)   // empty string when missing
// ... call into core ...
results, err := search.FuzzySearch(ctx, db, embedder, query, limit, proj)
```

No new error paths are introduced. JSON-RPC error mapping unchanged.
