# Specification: Universal Time-Decay for Search Results

**Mission ID**: 01KQ4631CA66WB1HCKH3MRDPKZ
**Mission slug**: universal-time-decay-for-search-results-01KQ4631
**Mission type**: software-dev
**Target branch**: main
**Created**: 2026-04-26
**Depends on**: Mission 1 (`engram-legacy-cutover-and-corpus-restoration-01KQ2SJK`) merged AND executed; Mission 2 (`recency-fast-path-for-session-recall-01KQ405N`) merged.
**Pre-flight artifacts** (frozen, do NOT modify):
- `docs/future-missions/mission-3-eval-set.yaml` — 20 evaluation queries across 4 categories.
- `docs/future-missions/mission-3-baseline.json` — measured search_fuzzy hit-rates against the eval set (commit `c81b9cff141f`, corpus = 1004 memories).
- `docs/future-missions/mission-3-baseline.md` — human-readable baseline summary.

---

## Purpose (Stakeholder Summary)

**TL;DR**: Make recency a universal dimension of search ranking by applying a multiplicative time-decay factor to BOTH `search_fuzzy` and `search_exact` results under a single shared `RRF_TIME_DECAY` flag and `RRF_HALF_LIFE_DAYS` knob, so that fresh memories outrank obsolete ones across every search interface — without burying ancient high-signal records.

**Context**: Mission 1 restored real timestamps. Mission 2 ships `recall_recent_session` for the explicit "what was the last X" use case. Neither solves the implicit recency expectation in mixed-intent queries: when an agent asks "latest changes to the schema" or searches by an exact title, today's hybrid ranking returns the most relevant memory regardless of age. The frozen baseline measurement confirms the gap: pure-recency queries hit top-3 0% of the time on `search_fuzzy`. At the same time, adversarial queries (very old but vital architectural memories) hit top-3 80% of the time today — that's the line we must not cross when introducing decay. This mission introduces a multiplicative exponential decay factor applied universally, controlled by a single flag and a single half-life knob, defaulting to OFF so existing behavior is unchanged until the operator opts in.

**`search_exact` is included by explicit operator directive**: exact search does not guarantee unique winners — duplicate ticket titles, project names, and recurring section headers produce real collisions today. Recency must arbitrate those collisions consistently with how `search_fuzzy` handles them. A single shared knob avoids configuration fragmentation across the search surfaces.

---

## User Scenarios & Testing

### Primary scenario — Mixed-intent query surfaces fresh result

**Actor**: A coding agent asking "latest changes to the schema".
**Today (broken)**: `search_fuzzy` returns the most semantically relevant schema memory regardless of when it was written. The actual newest schema-related entry (memory id 988) is absent from the top 10 (rank: null per baseline).
**After this mission (success, decay ON)**: Same query, `search_fuzzy` returns the newest schema-related memory in the top 3. The semantically relevant older memories still appear, just not in the top spot.

### Primary scenario — Exact-title collision broken by recency

**Actor**: A coding agent calling `search_exact("Session summary: hsme")`.
**Today (broken)**: Multiple memories carry literally the same title. FTS5 returns them in BM25 order, which can place an older summary above a newer one.
**After this mission (success, decay ON)**: The most recent matching memory ranks first. Older entries still appear below it but are ordered by combined relevance + recency.

### Primary scenario — Operator opts in safely

**Actor**: The operator running HSME locally.
**Trigger**: Operator wants to compare decay-on vs decay-off.
**Sequence**:
1. With `RRF_TIME_DECAY=off` (default), behavior is byte-identical to the pre-mission baseline.
2. Operator runs the A/B harness against the frozen eval set. Harness produces a side-by-side report.
3. Operator sets `RRF_TIME_DECAY=on` and `RRF_HALF_LIFE_DAYS=14`. Both `search_fuzzy` and `search_exact` now apply decay.
4. Operator can revert by flipping the flag back. No data migration involved.

### Exception scenario — Future timestamps (clock skew)

**Trigger**: A memory has `created_at` in the future relative to the query time (clock skew, bad data).
**Outcome**: Decay factor = 1.0 (zero-age treatment). The memory does not get a magnified score from "negative age". This is a defensive default, not a feature.

### Exception scenario — Adversarial query against ancient architectural record

**Trigger**: Agent asks "original HSME architecture overview" (matches old memory id 7, 2.34 days old at baseline measurement).
**Today (decay off, baseline)**: memory 7 appears at rank 1 for some adversarial queries, missed in top 10 for others. Top-3 hit-rate is 80% across the adversarial category.
**After this mission (decay ON with default half-life)**: top-3 hit-rate for adversarial queries MUST remain ≥ 80%. The decay does not bury vital old memories; relevance still dominates when it is high enough.

### Edge cases

- Memory with `created_at` exactly equal to query time: factor = 1.0 (newest possible).
- Memory with multiple chunks: decay applied at chunk level BEFORE aggregation, so a multi-chunk memory is not penalized N times.
- `RRF_HALF_LIFE_DAYS=0` or negative: rejected at startup with a clear error message; decay would otherwise be undefined.
- Two memories with literally identical scores AND identical `created_at`: deterministic tiebreak by memory id descending (matches existing convention).
- A/B harness run while the database is being written to: harness operates against a consistent SQLite snapshot; results reflect the corpus at run time and the harness records the corpus snapshot timestamp.

---

## Domain Language

| Term | Canonical meaning | Synonyms to avoid |
|------|-------------------|-------------------|
| Time-decay factor | A multiplicative scalar in `(0, 1]` computed from a memory's `created_at` and the query time, used to weight the memory's search score. Newer memories get factor ≈ 1; older memories get a smaller factor. | "decay weight", "freshness multiplier" — accepted but use the canonical term in code. |
| Half-life (in days) | The number of days after which the decay factor reaches `0.5`. Configured via `RRF_HALF_LIFE_DAYS`. | — |
| `RRF_TIME_DECAY` | The single global flag controlling whether time-decay is applied. Values: `on`, `off`. Default: `off`. Scope: applies to BOTH `search_fuzzy` and `search_exact`. | "decay flag" — colloquial only |
| `RRF_HALF_LIFE_DAYS` | The single global half-life knob. Positive number. Default: `14`. Scope: same as the flag. | — |
| Adversarial preservation | The invariant that old-but-vital memories continue to rank within the established top-N when decay is enabled. Concretely: adversarial top-3 hit-rate with decay ON ≥ 80% (matches baseline). | — |
| Frozen eval set | The 20-query set at `docs/future-missions/mission-3-eval-set.yaml`. Treated as immutable input for this mission's measurement and verification. | "test queries" — too vague |

---

## Functional Requirements

| ID | Requirement | Status |
|----|-------------|--------|
| FR-001 | The system SHALL implement a multiplicative exponential time-decay function `decay(age_days, half_life_days) = 0.5 ^ (age_days / half_life_days)` returning a value in `(0, 1]`. | Drafted |
| FR-002 | When `RRF_TIME_DECAY=on`, `search_fuzzy` SHALL apply the decay factor to each chunk's RRF score BEFORE aggregating chunk scores into a memory-level score. | Drafted |
| FR-003 | When `RRF_TIME_DECAY=on`, `search_exact` SHALL apply the decay factor to each result row's underlying FTS5 BM25 score (or its rank-based equivalent if BM25 is not directly accessible) before final ordering. | Drafted |
| FR-004 | The decay factor SHALL be computed against the memory's `created_at`, NOT `updated_at`. | Drafted |
| FR-005 | When `RRF_TIME_DECAY=off` (default), the output of both `search_fuzzy` and `search_exact` SHALL be byte-identical to the pre-mission behavior for any given input. | Drafted |
| FR-006 | When a memory's `created_at` is in the future relative to the query time, the decay factor SHALL be `1.0` (zero-age treatment) — no magnification effect. | Drafted |
| FR-007 | The decay SHALL be applied at the chunk level (multi-chunk memories receive the factor once per chunk in the score function, not multiplied N times against the aggregated score). | Drafted |
| FR-008 | A new environment variable `RRF_TIME_DECAY` SHALL accept the values `on` or `off`. Default: `off`. Any other value rejected at startup with a clear error. | Drafted |
| FR-009 | A new environment variable `RRF_HALF_LIFE_DAYS` SHALL accept a positive number (integer or float). Default: `14`. Zero or negative rejected at startup with a clear error. | Drafted |
| FR-010 | Both `RRF_TIME_DECAY` and `RRF_HALF_LIFE_DAYS` SHALL configure both `search_fuzzy` and `search_exact`. No per-tool flag or knob exists. | Drafted |
| FR-011 | An A/B benchmark harness SHALL exist as an executable (binary, script, or `go test` target) that runs the frozen eval set against both `search_fuzzy` and `search_exact` with decay-off and decay-on, producing a side-by-side report comparing rank deltas per query. | Drafted |
| FR-012 | The A/B harness SHALL include `search_exact` samples — at least 5 of the 20 frozen eval queries SHALL be run through `search_exact` in addition to `search_fuzzy`. | Drafted |
| FR-013 | The A/B harness SHALL produce a report file under `data/benchmarks/<run_id>/` with both JSON and human-readable summaries, mirroring the structure of `docs/future-missions/mission-3-baseline.json`. | Drafted |
| FR-014 | README SHALL gain a section explaining `RRF_TIME_DECAY`, `RRF_HALF_LIFE_DAYS`, the A/B harness invocation, and how to interpret the report. | Drafted |
| FR-015 | The frozen eval set at `docs/future-missions/mission-3-eval-set.yaml` SHALL be treated as immutable input. Any change to it requires a separate mission. | Drafted |

---

## Non-Functional Requirements

| ID | Requirement | Threshold | Status |
|----|-------------|-----------|--------|
| NFR-001 | End-to-end latency overhead with `RRF_TIME_DECAY=on` for either tool, measured against the frozen eval set | ≤ 5% increase over the decay-off baseline (median across the 20 eval queries) | Drafted |
| NFR-002 | Pure-recency top-3 hit-rate with decay ON, measured by the A/B harness | ≥ 60% (baseline: 0%) | Drafted |
| NFR-003 | Adversarial top-3 hit-rate with decay ON | ≥ 80% (baseline: 80% — MUST NOT regress) | Drafted |
| NFR-004 | Pure-relevance top-10 hit-rate with decay ON | ≥ 60% (baseline: 60% — MUST NOT regress) | Drafted |
| NFR-005 | Mixed top-3 hit-rate with decay ON | ≥ 60% (baseline: 60% — MUST NOT regress) | Drafted |
| NFR-006 | A/B harness `search_exact` sample run completes without runtime error or anomaly (defined as: any returned memory id not present in the corpus, or any negative score) | 0 anomalies across the search_exact samples in the harness output | Drafted |
| NFR-007 | Default-off behavioral equivalence: with `RRF_TIME_DECAY=off`, the entire frozen eval set produces identical top-10 result lists compared to the pre-mission baseline | 100% list equality on all 20 queries (decay-off must be byte-identical to baseline) | Drafted |

---

## Constraints

| ID | Constraint | Status |
|----|------------|--------|
| C-001 | The default ship value of `RRF_TIME_DECAY` SHALL be `off`. Operators opt in explicitly. | Drafted |
| C-002 | A single shared flag (`RRF_TIME_DECAY`) and single shared half-life knob (`RRF_HALF_LIFE_DAYS`) SHALL configure both search tools. No fragmentation into per-tool flags. | Drafted |
| C-003 | The decay function SHALL be pure: deterministic given `created_at`, query time, and half-life. No I/O, no random behavior, no hidden state. | Drafted |
| C-004 | Future-dated memories SHALL be treated as zero-age (factor = 1.0). The system SHALL NOT magnify scores via negative age. | Drafted |
| C-005 | The A/B harness SHALL be reproducible: re-running it against the same corpus snapshot and the same frozen eval set produces identical reports. | Drafted |
| C-006 | The frozen eval set at `docs/future-missions/mission-3-eval-set.yaml` SHALL NOT be modified by this mission. | Drafted |
| C-007 | `recall_recent_session` (Mission 2's tool) SHALL NOT be modified by this mission — it remains an exact-recency tool independent of the decay knob. | Drafted |
| C-008 | The chunk-level decay application (FR-007) SHALL be implemented in such a way that turning the flag off is provably a no-op (achievable by multiplying by `1.0` when off, or by branching off the decay path entirely). | Drafted |

---

## Success Criteria

1. With `RRF_TIME_DECAY=off`, the A/B harness produces top-10 lists for all 20 eval queries that are byte-identical to `docs/future-missions/mission-3-baseline.json` (NFR-007).
2. With `RRF_TIME_DECAY=on` and `RRF_HALF_LIFE_DAYS=14`, the A/B harness reports pure-recency top-3 hit-rate ≥ 60% (NFR-002).
3. With decay ON, adversarial top-3 hit-rate ≥ 80% — same as the baseline (NFR-003).
4. With decay ON, pure-relevance top-10 hit-rate ≥ 60% — same as the baseline (NFR-004).
5. With decay ON, mixed top-3 hit-rate ≥ 60% — same as the baseline (NFR-005).
6. End-to-end latency with decay ON adds ≤ 5% over decay-off (NFR-001).
7. The A/B harness includes at least 5 `search_exact` samples and reports zero anomalies (NFR-006, FR-012).
8. README documents the two environment variables, the harness command, and how to interpret the report.
9. Setting `RRF_HALF_LIFE_DAYS=0` causes the server to refuse to start with a clear error message (FR-009).
10. Toggling `RRF_TIME_DECAY` between `on` and `off` requires only an env-var change and a process restart — no data migration, no schema change, no index rebuild.

---

## Key Entities

### Memory (`memories` table — read-only for this mission)
- `id` (INT, PK)
- `created_at` (DATETIME) — input to the decay function.
- All other fields untouched.

### Time-Decay Factor (new conceptual entity, no persistence)
- A pure function: `decay(age_days, half_life_days) → float in (0, 1]`.
- Inputs: `age_days = max(0, (now - created_at) / 86400)`; `half_life_days` from `RRF_HALF_LIFE_DAYS`.
- Output: scalar applied multiplicatively to per-chunk scores.

### Benchmark Run Report (new, written to `data/benchmarks/<run_id>/`)
- `run_id` = `<timestamp>-<flag_state>` (e.g., `20260427T100000Z-on` and `20260427T100015Z-off` for paired runs).
- `report.json` — schema mirrors `docs/future-missions/mission-3-baseline.json`, with one results entry per query × tool combination.
- `report.md` — human summary, ≤ 2 pages.
- `delta.json` — paired comparison: per-query rank delta from off → on.

### Configuration (new env vars)
- `RRF_TIME_DECAY` — {`on`, `off`}, default `off`.
- `RRF_HALF_LIFE_DAYS` — positive number, default `14`.
- Validated at startup; invalid values reject the process boot with a clear stderr message.

---

## Assumptions

1. **Implementation interpretation of "tie-breaker" for `search_exact`**: per the operator's official directive, the same multiplicative decay function used in `search_fuzzy` is applied to `search_exact` scores. The "tie-breaker" framing in the directive is the operator's mental model for *why* exact search benefits from decay (real collisions exist among duplicate titles); the implementation is universal multiplicative decay, not a literal-tie-only branch. If the operator intended a stricter tie-break-only behavior, this assumption must be revisited before plan-time.
2. The corpus state at execution time will be substantively similar to the baseline measurement (1004 memories, date range 2026-04-03 → 2026-04-26). NFR thresholds derived from the baseline assume the underlying corpus characteristics (relative density of recent vs old memories per category) do not shift dramatically between baseline and verification.
3. The A/B harness runs against a consistent SQLite snapshot. Concurrent writes by the MCP server during a harness run can change ranking for queries that match newly-written memories, but the harness records the corpus snapshot timestamp so deltas remain auditable.
4. `search_exact` baseline numbers will be measured AS PART OF THIS MISSION (the existing baseline in `mission-3-baseline.json` covers only `search_fuzzy`). The first deliverable of the harness is the new `search_exact` baseline; only after that do the NFR comparisons apply to that tool.
5. Default `RRF_HALF_LIFE_DAYS=14` is a reasonable starting point. Operators are expected to tune it via the A/B harness against their own use patterns. The mission ships the default, not a tuned value.
6. The frozen eval set is representative of expected usage. If real-world queries diverge significantly, the eval set will need to grow — but that is a separate future effort, not within this mission.

---

## Out of Scope

- **Per-query, per-source-type, or per-project decay weighting** (e.g., "session summaries decay faster than architecture notes"). One global half-life only in v1.
- **Replacing RRF with a different fusion algorithm** (e.g., LightGBM, learning-to-rank). The decay is multiplicative on top of the existing RRF/BM25 outputs.
- **Cross-encoder reranking, MMR, or other post-retrieval techniques.**
- **Changes to `recall_recent_session`** — that tool already returns chronologically ordered results without semantic ranking, so decay is irrelevant to it (C-007).
- **Pagination or cursor-based result sets.**
- **Re-tuning the eval set** — the frozen set is immutable for this mission (C-006).
- **Auto-tuning of `RRF_HALF_LIFE_DAYS`** based on telemetry or feedback. Manual tuning only.
- **Telemetry on decay's impact in production**. Operators run the A/B harness when they want to measure, not continuously.
- **Backwards compatibility shim for some pre-1.0 RRF score format**. The decay applies to current code paths only.
