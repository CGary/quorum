# Implementation Plan: Universal Time-Decay for Search Results

**Branch**: `main` | **Date**: 2026-04-26 | **Spec**: [spec.md](spec.md)
**Mission**: `universal-time-decay-for-search-results-01KQ4631`
**Mission ID**: `01KQ4631CA66WB1HCKH3MRDPKZ`

---

## Summary

Apply a multiplicative exponential time-decay factor to BOTH `search_fuzzy` and `search_exact` under a single shared flag/half-life pair, defaulting to OFF. For `search_fuzzy`, the decay multiplies each chunk's RRF score before per-memory aggregation. For `search_exact`, the decay multiplies the FTS5 BM25 score (verified available in the build) before final ordering; the substring fallback uses a negative sentinel score multiplied by the same decay so the ordering function is uniform. A new binary `cmd/bench-decay/` runs the frozen evaluation set against both tools with decay-on and decay-off, producing rank-delta reports under `data/benchmarks/<run_id>/`. README documents the flags. The decay path is gated by an explicit `if cfg.TimeDecayEnabled { ... }` branch at the top of each function so byte-equivalence with decay-off is provably safe.

## Technical Context

| Item | Value |
|------|-------|
| **Language/Version** | Go (existing module). |
| **Primary Dependencies** | `mattn/go-sqlite3` with build tags `sqlite_fts5 sqlite_vec`. **`bm25()` confirmed available** in this build (verified 2026-04-26 against production DB). No new third-party dependencies. |
| **Storage** | SQLite at `data/engram.db`. Read-only for the new tools. No schema changes. No new indexes (existing FTS5 already supports `bm25()` queries). |
| **Testing** | Go tests under `tests/modules/` with `-tags "sqlite_fts5 sqlite_vec"`. Add unit tests for the decay function, integration tests for `search_fuzzy` and `search_exact` decay-on vs decay-off byte-equivalence, harness smoke test. |
| **Target Platform** | Linux (existing). |
| **Project Type** | Single Go module. |
| **Performance Goals** | Decay overhead ≤5% latency on either tool (NFR-001). The decay is one float multiplication per chunk/result row + one age computation. |
| **Constraints** | Default OFF (C-001). Single shared knob (C-002). Pure decay function (C-003). Future timestamps clamped to factor=1.0 (C-004). Reproducible harness (C-005). Frozen eval set immutable (C-006). `recall_recent_session` untouched (C-007). |
| **Scale/Scope** | 2 search functions modified, 1 new binary, 2 new env vars, 1 README section, ~5-7h total (matches the original draft estimate). |

## Charter Check

**SKIPPED** — `.kittify/charter/charter.md` does not exist. Standard engineering safeguards apply: tests pass, defaults preserve existing behavior, no schema migration, additive flag.

## Project Structure

### Documentation (this mission)

```
kitty-specs/universal-time-decay-for-search-results-01KQ4631/
├── spec.md
├── plan.md                # This file
├── research.md
├── data-model.md
├── contracts/
│   ├── decay-function.md       # decay() pure function contract
│   ├── search-tool-changes.md  # how FuzzySearch / ExactSearch evolve
│   └── bench-harness.md        # cmd/bench-decay CLI contract
├── quickstart.md
├── checklists/requirements.md  # already created
└── tasks/                # /spec-kitty.tasks output
```

### Source Code Changes

```
src/
└── core/
    └── search/
        ├── fuzzy.go         # MODIFIED — add decay branch in FuzzySearch + ExactSearch
        ├── recency.go       # UNCHANGED (Mission 2's tool, decay-irrelevant)
        ├── graph.go         # UNCHANGED
        └── decay.go         # NEW — pure decay function + config loading

cmd/
├── hsme/
│   └── main.go              # MODIFIED — read RRF_TIME_DECAY / RRF_HALF_LIFE_DAYS at startup, init search package config
├── worker/                  # UNCHANGED
├── ops/                     # UNCHANGED
├── migrate-legacy/          # UNCHANGED
└── bench-decay/             # NEW — A/B harness binary
    ├── main.go                  # CLI entry + flag parser
    ├── runner.go                # eval-set load + run queries + capture results
    ├── delta.go                 # paired comparison off vs on
    └── report.go                # JSON + MD writers

tests/
└── modules/
    ├── decay_test.go            # NEW — pure-function unit tests for decay()
    ├── search_decay_test.go     # NEW — integration: byte-equivalence off, expected reordering on
    └── bench_decay_test.go      # NEW — smoke test for the harness

README.md                        # MODIFIED — new section "Time-Decay Configuration"

data/
└── benchmarks/                  # NEW operational output, gitignored
    └── <run_id>/                # one dir per harness run
```

### Existing files NOT touched

- `src/storage/sqlite/db.go` — no schema changes.
- `src/core/search/recency.go` — `recall_recent_session` is decay-irrelevant (C-007).
- `cmd/migrate-legacy/`, `cmd/worker/`, `cmd/ops/` — unrelated.
- `kitty-specs/recency-fast-path-for-session-recall-01KQ405N/`, `kitty-specs/engram-legacy-cutover-and-corpus-restoration-01KQ2SJK/` — prior missions, frozen.
- `docs/future-missions/mission-3-eval-set.yaml` — frozen input (C-006).
- `docs/future-missions/mission-3-baseline.json` — frozen reference (the harness compares decay-off output against this).

## Phase Definition

The implementation breaks into 5 phases. Each phase is independently testable.

| # | Phase | Goal | Outputs |
|---|-------|------|---------|
| **1** | **Decay function + config** | Pure `decay()` function and config loader (env var validation). Standalone, independent of search code. | `src/core/search/decay.go`, unit tests in `tests/modules/decay_test.go` |
| **2** | **Wire decay into `search_fuzzy`** | Add the explicit branch on `RRF_TIME_DECAY`. When on, multiply `chunk.Score` by decay factor BEFORE `memoryScores[memoryID]` aggregation. Extend chunk-fetch SELECT to also pull `created_at` from memories. | Modified `src/core/search/fuzzy.go`, integration tests for byte-equivalence (off) and reordering (on) |
| **3** | **Wire decay into `search_exact`** | Modify `exactSearchFTS` to SELECT `bm25(memory_chunks_fts)` and `created_at`. When decay on, ORDER BY `bm25 * decay_factor ASC`. For `exactSearchSubstring` fallback, use sentinel score `-1e-3 * decay_factor` so ordering is uniform with FTS. | Modified `src/core/search/fuzzy.go` |
| **4** | **A/B harness binary** | New `cmd/bench-decay/` that reads the frozen eval set, runs each query against both tools with decay off and on, computes rank deltas, writes report under `data/benchmarks/<run_id>/`. | `cmd/bench-decay/*.go`, smoke test |
| **5** | **Documentation + verification** | README section explaining the two env vars + harness invocation. Run harness against the corpus, confirm all NFR thresholds. | `README.md` updates, run report committed (NFR audit trail) |

## Phase 0: Outline & Research → research.md

See [research.md](research.md). Key resolved items:

- **R-001**: BM25 availability verified — `bm25()` returns negative floats; ORDER BY ASC = best first; multiplying by decay factor `(0,1]` makes the negative score less negative → memory ranks lower. Math direction validated.
- **R-002**: Decay wiring strategy — explicit `if` branch at the top of each function. Rejected always-multiply-by-1.0 because byte-equivalence is harder to prove when the multiplication path runs unconditionally.
- **R-003**: Substring fallback handling — sentinel score `-1e-3` (just below the lowest realistic BM25 in current corpus, verified) multiplied by `decay_factor` so the ordering function is uniform across FTS and fallback paths. Substring results still always rank below FTS results because their post-decay score remains less negative than any FTS score.
- **R-004**: Configuration loading — read env vars once at `cmd/hsme/main.go` startup, store in a `search.DecayConfig` struct, pass into the search functions OR set as a package-level variable. Picked package-level for minimum signature change.
- **R-005**: Future-timestamp handling — `age_days = max(0, (now - created_at).Hours()/24)`. The `max(0, ...)` clamps future timestamps to age=0 → factor=1.0.
- **R-006**: Multi-chunk memory handling — decay applies to each chunk individually before aggregation. Since aggregation is `max()`, the chunk with the highest `(score * decay)` wins; this naturally honors FR-007 without "penalizing N times".
- **R-007**: Test strategy for byte-equivalence (NFR-007) — Capture decay-off top-10 lists for all 20 frozen queries in a golden file. Test asserts decay-off output matches that golden file exactly. Updates to the golden file require explicit acknowledgment.

## Phase 1: Design & Contracts → data-model.md, contracts/, quickstart.md

See:
- [data-model.md](data-model.md): function signatures, env var contract, report schema.
- [contracts/decay-function.md](contracts/decay-function.md): the pure decay function — inputs, outputs, edge cases.
- [contracts/search-tool-changes.md](contracts/search-tool-changes.md): how `FuzzySearch` and `ExactSearch` evolve.
- [contracts/bench-harness.md](contracts/bench-harness.md): the `cmd/bench-decay` CLI contract.
- [quickstart.md](quickstart.md): operator runbook for opting in to decay and running the A/B harness.

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| `bm25()` returns unexpected scale or sign on some FTS5 queries | Low | Medium | Verified at plan time on production DB. Phase 3 includes a defensive sanity check: assert `bm25_score <= 0` per FTS5 spec; abort with error if violated. |
| Substring fallback sentinel collides with real BM25 values | Low | Low | Sentinel = `-1e-3`. Real BM25 in current corpus ranges from `-12` to `-0.001`. Sentinel chosen to be less-negative than the worst FTS hit (≈ `-1e-6`), keeping fallback below FTS in any practical corpus. R-003 documents the audit. |
| Decay-off path accidentally diverges from current behavior | Medium | High | Mitigated by Phase 1's golden-file byte-equivalence test (NFR-007). The test runs ALL 20 frozen queries with decay off and compares against `mission-3-baseline.json`. |
| Half-life of 14 days is wrong for this corpus | Medium | Low | The default ships and the harness lets the operator tune. NOT a release blocker. The harness output documents which half-life produces best per-category metrics. |
| A/B harness perturbs the production DB during runs | Low | Low | Harness opens DB read-only (`?mode=ro&immutable=1`). Confirmed in contract. |
| `created_at` of a chunk is approximated (uses memory's `created_at`, not a per-chunk timestamp) | Low | Low | Documented as design choice. Memories are the unit of recency, not chunks. The DB schema confirms `memory_chunks` has its own `created_at`, but its semantics are "chunk insertion time" which aligns with the memory's `created_at` for our purposes. Plan uses `memories.created_at` consistently. |

## Definition of Done

1. `go build -tags "sqlite_fts5 sqlite_vec" ./...` compiles clean.
2. `go test -tags "sqlite_fts5 sqlite_vec" ./tests/modules/...` passes — including the new decay tests AND all existing tests.
3. Decay-off byte-equivalence golden test passes against `docs/future-missions/mission-3-baseline.json`.
4. `RRF_TIME_DECAY=on RRF_HALF_LIFE_DAYS=14 ./bench-decay` produces a report showing pure-recency top-3 ≥ 60% AND adversarial top-3 ≥ 80%.
5. With `RRF_HALF_LIFE_DAYS=0`, the HSME server refuses to start with a clear error message.
6. README has a "Time-Decay Configuration" section explaining both env vars and harness invocation.
7. Toggling `RRF_TIME_DECAY` between `on` and `off` requires only an env var change + process restart — verified by manual test.
8. The harness output for at least one decay-on run is committed under `data/benchmarks/` as audit trail (or referenced from the merge commit).
9. No changes to `src/core/search/recency.go`, `kitty-specs/.../*.md` of prior missions, or `docs/future-missions/mission-3-eval-set.yaml`.

## Branch Contract Reaffirmation

- **Current branch:** `main`
- **Planning/base branch:** `main`
- **Final merge target:** `main`
- **`branch_matches_target`:** `true`

---

## Next Step

Run `/spec-kitty.tasks` to break this plan into work packages.
