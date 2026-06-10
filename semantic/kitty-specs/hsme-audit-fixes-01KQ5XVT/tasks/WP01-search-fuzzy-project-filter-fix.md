---
work_package_id: WP01
title: Fix search_fuzzy Project Filter Vector Search (FR-001)
dependencies: []
requirement_refs:
- FR-001
- FR-004
- NFR-001
- NFR-004
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-hsme-audit-fixes-01KQ5XVT
base_commit: dac3ef13d27e4bdafeedb434cc01af9c2c24f149
created_at: '2026-04-27T11:24:10.607973+00:00'
subtasks:
- T001
- T002
- T003
agent: "gemini:flash:reviewer:reviewer"
shell_pid: "2797548"
history: []
authoritative_surface: src/core/search/
execution_mode: code_change
owned_files:
- src/core/search/fuzzy.go
- tests/bdd/search_fuzzy_project.feature
role: implementer
tags: []
---

## ⚡ Do This First: Load Agent Profile

```bash
spec-kitty agent profile load --id implementer-ivan
```

If implementer-ivan is not available, load the best available implementer profile:
```bash
spec-kitty agent profile list --json
```

## Objective

Fix FR-001: `search_fuzzy(query, project=X)` silently falls back to lexical-only search when a project filter is set. The bug is in `src/core/search/fuzzy.go` around line 387 where the project-branch vector query applies LIMIT after a JOIN — vec0 rejects this with "A LIMIT or 'k = ?' constraint is required on vec0 knn queries". The fix restructures the query as a CTE (KNN scan first with LIMIT, then JOIN to apply project filter). Non-project searches must not regress.

## Branch Strategy

Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main. Execution worktrees are allocated per computed lane from `lanes.json`.

## Context from Spec and Plan

**Bug**: The project-filter branch of `VectorSearch` does:
```sql
SELECT rowid FROM memory_chunks_vec v
  JOIN memory_chunks c ON c.id = v.rowid
  JOIN memories m ON m.id = c.memory_id
WHERE v.embedding MATCH ? AND m.project = ?
LIMIT ?
```
This violates vec0's requirement that LIMIT be applied directly to the KNN scan, not after JOIN.

**Fix**: Use CTE approach — KNN scan first with LIMIT, then JOIN:
```sql
WITH knn AS (
  SELECT rowid FROM memory_chunks_vec
  WHERE embedding MATCH ?
  LIMIT ?   -- vec0 constraint satisfied here
)
SELECT k.rowid FROM knn k
  JOIN memory_chunks c ON c.id = k.rowid
  JOIN memories m ON m.id = c.memory_id
WHERE m.project = ?
```

**Key constraints**:
- C-001: Fix must NOT change behavior of search_fuzzy WITHOUT project filter
- C-004: Catch-up/idempotency — not directly relevant here but remember for WP03

**HSME Memory**: 1014 (root cause analysis), 959 (search behavior confirmed)

## Subtasks

### T001: Fix fuzzy.go — Restructure Project-Branch Vector Query as CTE

**Purpose**: Fix the vec0 constraint violation in the project-filter branch of VectorSearch.

**Steps**:
1. Read `src/core/search/fuzzy.go` — find the `VectorSearch` function and locate the project-filter branch around line 387
2. Identify the exact SQL query being executed (the one with JOIN+LIMIT that fails)
3. Replace the project-branch query with a CTE that:
   - First runs KNN scan with LIMIT directly on memory_chunks_vec: `WITH knn AS (SELECT rowid FROM memory_chunks_vec WHERE embedding = ? LIMIT ?)`
   - Then JOINs to memory_chunks and memories to apply project filter: `SELECT k.rowid FROM knn k JOIN memory_chunks c ON c.id = k.rowid JOIN memories m ON m.id = c.memory_id WHERE m.project = ?`
4. Use `LIMIT k*10` in the KNN subquery to over-fetch and compensate for post-filter row loss (the project filter may reduce results significantly)
5. Verify the non-project branch is UNCHANGED — this is C-001 (no regression)
6. Ensure the fallback behavior (when vec0 is unavailable) still works for the project branch too
7. Run `go test -tags "sqlite_fts5 sqlite_vec" ./...` to verify compilation

**Files to modify**:
- `src/core/search/fuzzy.go`

**Validation**:
- [ ] vec0 no longer logs "A LIMIT or 'k = ?' constraint is required" error for project-filter queries
- [ ] search_fuzzy with project filter returns results with vector coverage=complete
- [ ] search_fuzzy WITHOUT project filter still returns mixed vector+lexical results (C-001)

**Risks**:
- Over-fetching with `LIMIT k*10` could be wasteful for large k values — document this trade-off
- The fallback to lexical when vec0 is unavailable must still work for project-filter case

---

### T002: Add godog BDD Test — search_fuzzy With Project Filter

**Purpose**: NFR-001 — Add a BDD scenario that FAILS before the fix and PASSES after.

**Steps**:
1. Install godog if not available: `go install github.com/cucumber/godog/cmd/godog@latest`
2. Create `tests/bdd/search_fuzzy_project.feature` with Gherkin syntax:

```gherkin
Feature: search_fuzzy with project filter uses vector search

  Scenario: project filter returns vector candidates (post-fix behavior)
    Given a test database with vec0 support and project "acme" with embedded chunks
    When the user calls search_fuzzy with query="consulta test" project="acme" k=10
    Then the search completes without vec0 "LIMIT" error
    And the result coverage is "complete" (vector candidates present)
    And the result includes embeddings from the project "acme"

  Scenario: search without project filter still works (regression check)
    Given a test database with vec0 support and project "acme" with embedded chunks
    When the user calls search_fuzzy with query="consulta test" k=10
    Then the search returns mixed vector + lexical results
    And RRF fusion is applied correctly

  Scenario: project filter falls back to lexical when vec0 unavailable
    Given a test database WITHOUT vec0 support
    When the user calls search_fuzzy with query="consulta test" project="acme" k=10
    Then the search completes without error
    And the result coverage is "partial" (lexical only, graceful degradation)
```

3. Create `tests/bdd/search_fuzzy_project_test.go` — godog test suite that:
   - Sets up a test DB with vec0, embeds test chunks tagged with project "acme"
   - Calls the actual `search_fuzzy` function with project filter
   - Asserts no vec0 error in logs
   - Asserts coverage field indicates vector participation
4. Run `godog run tests/bdd/search_fuzzy_project.feature` — it should FAIL before T001 fix (vec0 error + coverage=partial)
5. After T001 fix is applied, run godog again — it should PASS

**Files to create**:
- `tests/bdd/search_fuzzy_project.feature`
- `tests/bdd/search_fuzzy_project_test.go`

**Validation**:
- [ ] godog test FAILS on main (before T001 fix) with vec0 constraint error
- [ ] godog test PASSES after T001 fix with coverage=complete
- [ ] Regression scenario passes (non-project search still works)

**Pre-fix expected output**: vec0 logs "A LIMIT or 'k = ?' constraint is required on vec0 knn queries", coverage=partial
**Post-fix expected output**: No vec0 error, coverage=complete

---

### T003: Regression — Verify Non-Project search_fuzzy Still Works

**Purpose**: NFR-004 — Ensure the fix doesn't break search_fuzzy without project filter.

**Steps**:
1. Run existing search tests: `go test -tags "sqlite_fts5 sqlite_vec" ./tests/modules/...`
2. Manually verify (or add to godog suite) that `search_fuzzy(query="test", k=10)` without project filter:
   - Returns results from both vector and lexical branches
   - RRF fusion is applied
   - No vec0 errors in output
3. Check that the code diff for T001 only touches the project-filter branch, leaving the no-project branch untouched

**Files to verify**:
- `src/core/search/fuzzy.go` — no changes to no-project code path

**Validation**:
- [ ] All existing tests pass: `go test -tags "sqlite_fts5 sqlite_vec" ./...`
- [ ] No-project search returns mixed results with vector participation

## Definition of Done

- [ ] `godog run tests/bdd/search_fuzzy_project.feature` PASSES (post-fix)
- [ ] All existing tests pass: `go test -tags "sqlite_fts5 sqlite_vec" ./...`
- [ ] No-project search verified working (regression check)
- [ ] No files outside `owned_files` modified
- [ ] Code compiles without errors

## Risks

| Risk | Mitigation |
|------|------------|
| Over-fetch with `LIMIT k*10` causes OOM for large k | Cap max k at reasonable limit (e.g., 1000) or use k*5 instead |
| Fallback to lexical for project filter may have different behavior than vector | Ensure fallback still applies C-001 semantics (graceful degradation, coverage=partial) |
| Non-project search regresses | T003 explicitly validates no regression |

## Reviewer Guidance

- The diff for `fuzzy.go` should ONLY change the project-filter branch query
- Verify the CTE approach is correct: KNN scan with LIMIT directly on vec0, then JOIN for project filter
- Run `godog run tests/bdd/search_fuzzy_project.feature` — must pass
- Run full test suite: `go test -tags "sqlite_fts5 sqlite_vec" ./...` — must pass

## Activity Log

- 2026-04-27T11:39:20Z – claude – shell_pid=2450642 – Fixed FR-001 by restructuring vector query as two-step process to avoid vec0 constraint violation. Implemented graceful degradation coverage reporting as 'partial' when vector search is unavailable. Added BDD scenarios covering all cases.
- 2026-04-27T15:32:25Z – gemini:flash:reviewer:reviewer – shell_pid=2797548 – Started review via action command
- 2026-04-27T15:38:04Z – gemini:flash:reviewer:reviewer – shell_pid=2797548 – Review passed: Implementation correctly addresses the vec0 constraint violation by using a two-step retrieval process for project-filtered searches. Graceful degradation for vector search is correctly handled with appropriate coverage reporting. BDD tests verify all scenarios including regressions.
