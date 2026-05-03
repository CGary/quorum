# Specification: Recency Fast Path for Session Recall

**Mission ID**: 01KQ405N8N65EAS0WAWFXPS0AY
**Mission slug**: recency-fast-path-for-session-recall-01KQ405N
**Mission type**: software-dev
**Target branch**: main
**Created**: 2026-04-26
**Depends on**: `engram-legacy-cutover-and-corpus-restoration-01KQ2SJK` (merged and executed)

---

## Purpose (Stakeholder Summary)

**TL;DR**: Add a dedicated tool that retrieves recent session summaries in chronological order, and update the agent protocol so recency queries bypass semantic search entirely.

**Context**: Semantic search ranks results by relevance, not time. When an agent asks "what did we do last session?" it gets the most semantically similar memory — which could be weeks old — instead of the chronologically newest one. Mission 1 restored real timestamps for the entire corpus (241 session summaries spanning 2026-04-04 to 2026-04-25). This mission capitalizes on that: a new lightweight tool `recall_recent_session` returns the N most recent `session_summary` memories, filtered optionally by project, ordered by creation date. No embedder, no vector math — a fast SQL lookup. Alongside the tool, the HSME agent protocol in `CLAUDE.md` is updated to direct agents to call `recall_recent_session` first for recency-style questions, reserving `search_fuzzy` for semantic exploration. Existing search tools are untouched.

---

## User Scenarios & Testing

### Primary scenario — Agent recovers last session context

**Actor**: A coding agent (Claude Code or similar) starting a new work session on the `aibbe` project.
**Trigger**: The agent needs to know what happened in the last session before proceeding.
**Today (broken)**: Agent calls `search_fuzzy("what did we do last session aibbe")`. Gets a semantically matched result — perhaps the most detailed session summary ever written — but it is weeks old. The agent picks up stale context.
**After this mission (success)**: Agent calls `recall_recent_session(project="aibbe", limit=1)`. Gets the single most recent `session_summary` for `aibbe`, ordered by real `created_at`. No relevance guessing. Result is available in under 100ms.

### Primary scenario — Agent lists recent sessions for a project

**Actor**: A coding agent reviewing the recent history of the `frontend-erp` project.
**Trigger**: User asks "summarize what we did this week".
**After this mission (success)**: Agent calls `recall_recent_session(project="frontend-erp", limit=5)`. Gets the 5 most recent session summaries for that project in reverse chronological order. Agent assembles a coherent week-in-review.

### Exception scenario — No session summaries exist for a project

**Trigger**: Agent calls `recall_recent_session(project="new-project", limit=3)`.
**Outcome**: Tool returns an empty list with 0 results. No error. Agent falls back to `search_fuzzy` if needed.

### Exception scenario — Limit exceeds maximum

**Trigger**: Agent calls `recall_recent_session(limit=500)`.
**Outcome**: Tool silently caps the result to 50. Returns at most 50 results. No error, no warning needed in the result payload — the cap is documented in the tool schema.

### Edge cases

- Superseded session summaries are excluded from results. If an agent stored a summary then immediately updated it (creating a `superseded_by` chain), only the active version appears.
- `project` omitted: returns the N most recent session summaries across all projects, ordered by `created_at DESC`.
- Two session summaries with identical `created_at` (clock collision): results are additionally ordered by `id DESC` so output is deterministic.
- Agent calls `recall_recent_session` then `search_fuzzy` for the same project: no side effects — both tools are independent read operations.

---

## Domain Language

| Term | Canonical meaning | Synonyms to avoid |
|------|-------------------|-------------------|
| Session summary | A memory with `source_type='session_summary'` — a human or agent-written recap of a completed work session. | "session recap", "summary", "session note" — imprecise |
| Recency query | A question whose answer depends on chronological order rather than semantic relevance. Examples: "what did we do last?", "last session", "recent work". | — |
| `recall_recent_session` | The new MCP tool introduced by this mission. | "recency tool", "session lookup" — use the exact tool name |

---

## Functional Requirements

| ID | Requirement | Status |
|----|-------------|--------|
| FR-001 | The system SHALL expose a new MCP tool named `recall_recent_session` that accepts two optional parameters: `project` (string) and `limit` (integer, default 5, maximum 50). | Drafted |
| FR-002 | `recall_recent_session` SHALL return memories where `source_type='session_summary'` and `status='active'`, ordered by `created_at DESC`, with `id DESC` as a tiebreaker. Superseded memories (`superseded_by IS NOT NULL`) SHALL be excluded. | Drafted |
| FR-003 | When `project` is supplied and non-empty, results SHALL be restricted to memories where `memories.project = <project>`. When omitted or empty, results span all projects. | Drafted |
| FR-004 | The `limit` parameter SHALL be silently capped at 50 server-side. A caller passing `limit=200` receives at most 50 results with no error. | Drafted |
| FR-005 | `recall_recent_session` SHALL NOT invoke the embedding model at any point. The response time budget is defined exclusively by SQL execution. | Drafted |
| FR-006 | The HSME agent protocol section in `CLAUDE.md` SHALL be updated to direct agents to call `recall_recent_session` BEFORE `search_fuzzy` whenever the user's intent is recency-oriented (questions containing "last", "latest", "recent", "what did we do", "last session", "last time"). | Drafted |
| FR-007 | Existing tools `search_fuzzy`, `search_exact`, and `explore_knowledge_graph` SHALL remain unchanged in behavior and signature. | Drafted |
| FR-008 | The composite index `(source_type, created_at DESC)` on the `memories` table SHALL exist to support the query. If already present, this requirement is satisfied; if not, it SHALL be created by this mission. | Drafted |

---

## Non-Functional Requirements

| ID | Requirement | Threshold | Status |
|----|-------------|-----------|--------|
| NFR-001 | P50 response latency for `recall_recent_session` with any valid inputs | ≤ 100ms measured on the production corpus (999 memories as of 2026-04-26) | Drafted |
| NFR-002 | Result correctness: given a fixture corpus with 50 session summaries spanning at least 30 days, `recall_recent_session(limit=1)` MUST return the chronologically newest entry | 100% accuracy — no wrong top-1 allowed | Drafted |
| NFR-003 | Result correctness with project filter: given a fixture corpus with summaries across 3 projects, `recall_recent_session(project="X", limit=10)` returns ONLY summaries for project X | 100% accuracy — zero cross-project leakage | Drafted |
| NFR-004 | Search latency for `search_fuzzy` and `search_exact` after the index addition | No regression — within ±5% of pre-mission baseline | Drafted |

---

## Constraints

| ID | Constraint | Status |
|----|------------|--------|
| C-001 | The tool SHALL NOT use the embedding model. Any code path that triggers the Ollama client is a defect. | Drafted |
| C-002 | The `limit` cap of 50 SHALL be enforced server-side, not just documented. The SQL query MUST include `LIMIT MIN(requested, 50)` or equivalent guard. | Drafted |
| C-003 | The index added in FR-008 SHALL be created with `CREATE INDEX IF NOT EXISTS` so it is idempotent on fresh installs and on databases that already have it. | Drafted |
| C-004 | The `CLAUDE.md` update (FR-006) SHALL apply only to the HSME protocol section — no other sections of the file are modified. | Drafted |
| C-005 | The result payload format SHALL be consistent with the existing MCP tool responses so callers need no schema change to consume it. | Drafted |

---

## Success Criteria

1. `recall_recent_session(project="aibbe", limit=1)` returns the memory with `id=994` (or whatever is newest at test time) as the top result, verified against `created_at DESC`.
2. `recall_recent_session(project="nonexistent", limit=5)` returns an empty list with no error.
3. `recall_recent_session(limit=500)` returns exactly 50 results (cap enforced).
4. Response time for any valid call measured on the production corpus ≤ 100ms (NFR-001).
5. `search_fuzzy` and `search_exact` latency unchanged after index creation (NFR-004).
6. The `CLAUDE.md` HSME protocol section contains an explicit instruction to call `recall_recent_session` before `search_fuzzy` for recency queries.
7. An agent starting a fresh session on project `mcp-semantic-memory` can retrieve the latest session summary in one tool call without using `search_fuzzy`.
8. All existing tests pass — no regressions introduced.

---

## Key Entities

### Memory (`memories` table — read-only for this tool)

Relevant fields:
- `id` (INT, PK) — tiebreaker in sort
- `source_type` (TEXT) — filter: `='session_summary'`
- `project` (TEXT, nullable) — optional filter
- `created_at` (DATETIME) — primary sort key
- `superseded_by` (INT, nullable) — exclusion: `IS NULL` for active memories only
- `status` (TEXT) — filter: `='active'`
- `raw_content` (TEXT) — returned as the memory body

### New index

`CREATE INDEX IF NOT EXISTS idx_memories_source_type_created ON memories(source_type, created_at DESC);`

Supports the primary query pattern without a full table scan.

---

## Assumptions

1. The production corpus at mission execution will have at least 50 session summaries (today: 241). NFR-002's fixture test uses a controlled temp DB, not production, so this is only relevant for the end-to-end smoke test.
2. The `project` column is already populated for ≥99% of session summaries (verified: 239/241 = 99.2% post-Mission-1). The 2 NULL-project summaries will surface in unfiltered calls but not in project-filtered calls.
3. The `CLAUDE.md` update targets only the HSME protocol block. No other sections of the file require changes for this feature.
4. Agents that currently call only `search_fuzzy` will continue to work correctly — this mission adds a new path, it does not remove or alter the existing one.
5. The Ollama service does not need to be running for `recall_recent_session` to function. The tool is fully operational with Ollama offline.

---

## Out of Scope

- RRF time-decay in `search_fuzzy` ranking — deferred to Mission 3.
- Session ID format enforcement inside summary content — explicitly deferred (user chose free-form, see Q3 decision in discovery).
- Pagination / cursor-based pagination for large result sets — the 50-item cap makes this unnecessary for now.
- Exposing `recall_recent_session` as a queryable dimension in `explore_knowledge_graph`.
- Any changes to how agents write session summaries — the convention (`source_type='session_summary'`) already exists and this mission only adds a reader.
