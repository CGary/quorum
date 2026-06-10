---
work_package_id: WP02
title: Search Ranking Integration
dependencies:
- WP01
requirement_refs:
- FR-002
- FR-003
- FR-005
- FR-007
- NFR-003
- NFR-004
- NFR-007
- C-001
- C-002
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: main
base_commit: cbad757af0b1f5be2f80e9569e196baa712c24db
created_at: '2026-04-26T06:20:47Z'
subtasks:
- T004
- T005
- T006
- T007
agent: "gemini:1.5-pro:architect:reviewer"
shell_pid: "1839375"
history: []
agent_profile: implementer-ivan
authoritative_surface: src/core/search/
execution_mode: code_change
owned_files:
- src/core/search/fuzzy.go
- tests/modules/search_decay_test.go
- tests/modules/testdata/decay_off_baseline.json
role: implementer
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Integrate time-decay into the existing ranking logic of both `search_fuzzy` and `search_exact` while preserving the current output exactly when decay is disabled.

## Branch Strategy
Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main.

## Execution Constraint
Preserve the current decay-off behavior by construction. Prefer an explicit top-level branch over mathematically neutral “factor = 1” tricks so the byte-equivalence invariant stays auditable in code review.

## Subtasks

### T004: Extend fuzzy-search chunk hydration to fetch `memories.created_at` and preserve the decay-off fast path
**Purpose**: Make timestamp data available to the fuzzy-ranking loop without adding extra DB round-trips.
**Steps**:
1. Extend the existing chunk hydration query to fetch `m.created_at` alongside the current fields.
2. Keep the current join/filter structure intact so status/project behavior does not drift.
3. Avoid changing result ordering or aggregation when `cfg.Enabled == false`.

### T005: Apply the decay factor inside fuzzy chunk scoring before per-memory aggregation
**Purpose**: Honor the spec requirement that chunk-level scores be decayed before `max()` aggregation.
**Steps**:
1. Pull the current decay config once per query, not once per chunk.
2. Multiply the chunk score by `DecayFactor(...)` only when decay is enabled.
3. Preserve the existing highlight and per-memory winner logic.
4. Keep `recall_recent_session` untouched.

### T006: Add decay-aware exact-search ordering for both FTS5 BM25 rows and substring fallback rows
**Purpose**: Make recency a universal dimension across exact and fuzzy search.
**Steps**:
1. Extend the FTS path to select `bm25(memory_chunks_fts)` and `m.created_at`.
2. Verify/reinforce the expected score direction (`ASC` because more relevant BM25 is more negative here).
3. For substring fallback rows, assign the agreed sentinel score and apply the same decay function.
4. Re-sort only in the decay-enabled path; leave decay-off ordering unchanged.
5. Preserve existing output shape for MCP callers.

### T007: Add integration tests and golden fixtures proving byte-equivalence when decay is off and expected reordering when decay is on
**Purpose**: Turn the mission’s most important safety claim into automated evidence.
**Steps**:
1. Add a committed golden fixture for the frozen decay-off baseline.
2. Add a test that compares decay-off results against the golden fixture exactly.
3. Add targeted cases showing recency promotion for fuzzy and exact search when decay is on.
4. Include at least one case covering duplicate exact-title results.

## Implementation Sketch
- Extend timestamp/score hydration first.
- Wire fuzzy decay second.
- Wire exact-search decay third.
- Finish with the golden and behavioral integration tests.

## Risks
- Accidentally changing decay-off ordering by always computing/re-sorting scores.
- Misreading BM25 sign semantics and reversing the intended ranking.
- Fallback substring rows interleaving unexpectedly with FTS rows if the sentinel is chosen poorly.

## Definition of Done
- `search_fuzzy` and `search_exact` both support decay.
- Decay-off output is provably identical to the frozen baseline.
- Exact-search collisions can be broken by recency when decay is on.
- No unrelated search surfaces are modified.

## Activity Log

- 2026-04-26T06:32:02Z – gemini:1.5-pro:architect:implementer – shell_pid=1834184 – Started implementation via action command
- 2026-04-26T06:35:48Z – gemini:1.5-pro:architect:implementer – shell_pid=1834184 – Search ranking integration complete
- 2026-04-26T06:35:49Z – gemini:1.5-pro:architect:reviewer – shell_pid=1839375 – Started review via action command
- 2026-04-26T06:35:50Z – gemini:1.5-pro:architect:reviewer – shell_pid=1839375 – Review passed
