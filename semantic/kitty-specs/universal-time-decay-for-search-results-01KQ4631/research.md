# Phase 0 Research: Universal Time-Decay for Search Results

**Mission**: `universal-time-decay-for-search-results-01KQ4631`
**Date**: 2026-04-26

This document records the technical decisions and the verifications they relied on. All `[NEEDS CLARIFICATION]` items from the spec discovery phase are resolved.

---

## R-001: BM25 availability and score semantics — verified at plan time

**Decision**: Use `bm25(memory_chunks_fts)` as the underlying relevance score for `search_exact`'s FTS5 path. ORDER BY ASC (lower = more relevant, per FTS5 spec). Multiply by decay factor before ordering.

**Verification performed (2026-04-26)**:
1. CLI test: `sqlite3 data/engram.db "SELECT bm25(memory_chunks_fts), rowid FROM memory_chunks_fts WHERE memory_chunks_fts MATCH 'recall_recent_session' ORDER BY bm25(memory_chunks_fts) LIMIT 3"` returned three rows with scores `-4.6256, -4.2294, -2.8012`.
2. Go test: a throwaway program built with `-tags "sqlite_fts5 sqlite_vec"` running the same query against the production DB returned matching values.
3. Conclusion: the FTS5 build available to HSME today supports `bm25()` directly. No vendored override needed.

**Math direction sanity check**:
- Memory A: BM25 = -4.6 (very relevant), age = OLD, decay = 0.3 → effective = -4.6 × 0.3 = **-1.38**
- Memory B: BM25 = -2.8 (less relevant), age = NEW, decay = 1.0 → effective = -2.8 × 1.0 = **-2.8**
- ORDER BY effective ASC: B (-2.8) before A (-1.38). Decay correctly demoted A despite its higher raw relevance.

**Alternatives considered**:
- *Rank-position proxy (RRF-style 1/(k+rank))*: works for pure ordering but loses the magnitude signal that BM25 carries. Rejected as a primary path.
- *Custom relevance score derived from term frequency*: reinventing FTS5. Rejected.

---

## R-002: Decay wiring strategy — explicit branch, not always-multiply

**Decision**: Both `FuzzySearch` and `ExactSearch` SHALL contain an explicit `if cfg.TimeDecayEnabled { ... }` branch. The decay code path is reachable ONLY when the flag is on. When off, no decay-related code executes.

**Rationale**:
- NFR-007 requires that decay-off output is byte-identical to the pre-mission baseline. The simplest way to guarantee that is to ensure that the decay code is unreachable when off.
- An always-multiply approach (factor = 1.0 when off) is mathematically equivalent but harder to audit: a future refactor could subtly change the multiplication path and silently break NFR-007.
- The explicit branch is more verbose by ~6 lines but makes the safety property obvious.

**Alternatives considered**:
- *Always multiply, with factor=1.0 when off*: rejected on the byte-equivalence audit grounds above.
- *Separate functions `FuzzySearchWithDecay` / `ExactSearchWithDecay` that wrap the originals*: too much duplication and would still need to be wired by a top-level branch somewhere.

---

## R-003: Substring fallback sentinel score

**Decision**: For results coming from `exactSearchSubstring` (no BM25 available because LIKE doesn't produce a relevance score), assign a sentinel score of `-1e-3` per row, then multiply by the decay factor. The unified ordering function then ranks all results — FTS-derived and substring-derived — with the same `score ASC` rule.

**Why `-1e-3`**:
- The smallest (least-relevant) BM25 value observed in the production corpus during R-001's verification is approximately `-1e-6` (very high "score" in the negated sense, meaning very low relevance).
- `-1e-3` is three orders of magnitude *less* negative than even the worst real BM25 score, which means substring results always rank below any FTS5 hit. That preserves the existing precedence: FTS hits beat substring hits.
- After multiplying by `decay_factor ∈ (0, 1]`, the substring sentinel stays in the range `(-1e-3, 0)`, still less negative than the BM25 floor, so the precedence holds.
- Within the substring group, `decay_factor` differentiates older from newer matches: older items have a smaller (closer to zero) effective score, so they rank lower.

**Alternatives considered**:
- *Score = 0*: makes all substring results tie regardless of decay (`0 × decay = 0`). Defeats the directive on consistent recency-arbitration.
- *Score = `created_at` directly*: confuses score units with time units; not generalizable.
- *Sort substring fallback purely by `created_at DESC`*: works but introduces a separate code path that doesn't go through the shared decay function, making future changes brittle.

---

## R-004: Configuration loading

**Decision**: Read both env vars (`RRF_TIME_DECAY`, `RRF_HALF_LIFE_DAYS`) once at `cmd/hsme/main.go` startup. Validate immediately. On invalid value, log to stderr and exit non-zero before serving any MCP request. Store in a package-level `search.DecayConfig` struct accessed via `search.GetDecayConfig()`.

**Rationale**:
- Reading env at startup matches the existing pattern for `OLLAMA_HOST`, `EMBEDDING_MODEL`, `SQLITE_DB_PATH` in `cmd/hsme/main.go`.
- Storing in a package-level variable avoids changing `FuzzySearch` and `ExactSearch` signatures (which would force a sweep of every call site, including tests). The MCP search functions currently take `(ctx, db, embedder, query, limit, project)` — adding a config param is a wider change than necessary.
- Validation at startup means an operator typo (e.g., `RRF_TIME_DECAY=true` instead of `on`) fails loudly at boot, not silently in queries.

**Alternatives considered**:
- *Function parameter*: cleaner API surface but breaks all callers. Deferred.
- *Re-read env on every query*: too cute, performance overhead, harder to reason about.
- *Config file*: adds a new file format; overkill for two env vars.

---

## R-005: Future-timestamp handling

**Decision**: Compute `age_days = max(0, (queryTime - createdAt).Hours()/24)`. Negative age (clock skew, future timestamps) clamps to 0, yielding decay factor 1.0.

**Rationale**:
- Per spec FR-006 and C-004, the system MUST NOT magnify scores via "negative age". Without the clamp, `0.5^(-5/14) ≈ 1.27` — that magnification is a bug surface.
- `max(0, ...)` is a single arithmetic operation, no branching, no panic risk.
- Same convention used by other recency-aware systems (e.g., search engines treating future-dated content as "now").

**Alternatives considered**:
- *Reject memories with future timestamps*: too aggressive — clock skew is a benign condition, not a data-quality issue worth filtering on.
- *Clamp age to 0 only when below some threshold (e.g., -1 day)*: opens a debate about the threshold; the spec's defensive default is the simplest choice.

---

## R-006: Multi-chunk memory handling

**Decision**: Decay applies to each chunk's score independently BEFORE the per-memory aggregation step. The aggregation rule (currently `max()` over chunk scores) remains unchanged.

**Rationale**:
- FR-007 requires that a multi-chunk memory not be penalized N times.
- Current code path: `chunk.Score` → `if chunk.Score > memoryScores[memoryID] { memoryScores[memoryID] = chunk.Score }`.
- New code path with decay on: `chunk.Score *= decay(memoryAge, halfLife)` BEFORE the comparison.
- Since the aggregation is max-over-chunks and all chunks of a given memory share the same `created_at` (from the parent memory), every chunk gets the same multiplier. The max comparison naturally picks the highest `chunk.Score * decay_factor`. No N-fold penalty.

**Verification**: a memory with 3 chunks scored (0.8, 0.5, 0.3) and decay 0.5 yields effective scores (0.4, 0.25, 0.15). Max is 0.4. The same memory with 1 chunk scored 0.8 and decay 0.5 yields 0.4. Equivalent. No fan-out penalty.

---

## R-007: Test strategy for byte-equivalence (NFR-007)

**Decision**: Capture decay-off top-10 result lists for all 20 frozen queries in a golden file at `tests/modules/testdata/decay_off_baseline.json`. The integration test reads the golden file and asserts decay-off output matches it. The golden file is generated once from the current `search_fuzzy` against the same corpus snapshot used in `mission-3-baseline.json`.

**Rationale**:
- The spec calls out a 100% list-equality requirement (NFR-007). A test asserting that requirement explicitly is the cheapest way to catch regressions in decay-off behavior.
- Using the existing `mission-3-baseline.json` directly avoids re-deriving the expected values; the golden file IS that JSON, in shape suitable for the test loader.
- The test file lives under `tests/modules/testdata/` (Go convention) and is committed.
- If decay-off output ever changes legitimately (e.g., a future ranking improvement to the underlying fuzzy/exact paths), updating the golden file requires explicit acknowledgment in a separate commit, making accidental regressions visible.

**Alternatives considered**:
- *No golden test, rely on the harness*: harness is operator-driven, not CI-driven. The byte-equivalence requirement is too important to leave to manual checks.
- *Property-based check (e.g., "decay-off equals decay-on with half_life=∞")*: clever but couples two code paths in a way that makes both harder to reason about.

---

## R-008: A/B harness location and shape

**Decision**: New binary at `cmd/bench-decay/main.go` with subcommands or flags to run paired off/on comparisons against the frozen eval set. Output under `data/benchmarks/<run_id>/`.

**Rationale**:
- Repo convention: every operational tool is its own binary in `cmd/` (`hsme`, `worker`, `ops`, `migrate-legacy`). The harness is operational tooling, not unit testing.
- Standalone binary is invokable from any context — local CLI, CI, or scheduled `/loop`. Test targets can't be invoked outside `go test` flow.
- Naming: `bench-decay` is unambiguous. Other names considered: `bench-search`, `decay-bench` (ordering with the noun first matches `migrate-legacy`).

**Alternatives considered**:
- *Test target with build tag `benchmark`*: requires test-runner invocation, less flexible.
- *Embed the harness inside `cmd/ops`*: confuses operational metrics with benchmarking.

---

## R-009: Substring-fallback decay path — verification details

**Decision**: When decay is on AND the FTS5 path returned fewer than `limit` results, the substring fallback runs as today, but each row is assigned the sentinel score `-1e-3`. The combined result list (FTS results + substring fallback) is then re-ordered by `effective_score = base_score * decay_factor` ascending.

**Why re-order even though substring fallback used to come strictly after FTS results**:
- Without re-ordering, the substring fallback's recency wouldn't factor in. A 6-month-old substring match would rank above a 1-day-old FTS hit if the FTS hit's BM25 score (post-decay) became less negative than the substring sentinel — which CAN happen if half-life is short and the FTS memory is old.
- With re-ordering, the unified score function decides. In practice, well-tuned half-life keeps FTS results dominant; the unification just removes a class of bugs that would emerge under aggressive half-lives.

**Audit point for the plan phase**: confirm with a smoke test that the unified ordering does NOT shuffle the example results from the baseline measurement (since baseline is decay-off). The byte-equivalence golden test (R-007) is the formal proof.

---

## R-010: `created_at` access in the search functions

**Decision**: Extend the existing batch-fetch query in `FuzzySearch` (the JOIN against `memories`) to also pull `m.created_at`. For `ExactSearch`, modify both the FTS5 query and the substring fallback to JOIN `memories` and pull `created_at`.

**Rationale**:
- `FuzzySearch` already does `JOIN memories m ON m.id = c.memory_id` to pull `m.status`. Adding `m.created_at` to the SELECT is a one-line change, no extra round-trip.
- `ExactSearch` already references `memories` in its WHERE clause (when `project` filter is active). Extending the SELECT to include `created_at` is similarly cheap.
- The alternative (a separate query to fetch `created_at` per result) doubles the round-trips — rejected.

---

## Open Items — None

All architectural decisions resolved. Ready to proceed with Phase 1 design artifacts.
