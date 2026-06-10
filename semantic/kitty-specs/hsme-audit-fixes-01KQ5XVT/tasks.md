# Tasks: HSME Audit Fixes — Observability Env, Vector Search, Rollup Catch-up

**Mission**: hsme-audit-fixes-01KQ5XVT
**Created**: 2026-04-26T22:32:09Z
**Spec**: [spec.md](./spec.md)
**Plan**: [plan.md](./plan.md)

## Subtask Index

| ID | Description | WP | Parallel | Dependencies |
|----|-------------|----|----------|--------------|
| T001 | Fix fuzzy.go: restructure project-branch query as CTE (KNN first, then JOIN) | WP01 | no | none | [D] |
| T002 | Add godog BDD test: search_fuzzy with project filter fails pre-fix, passes post-fix | WP01 | no | T001 | [D] |
| T003 | Regression: verify search_fuzzy WITHOUT project filter still works | WP01 | no | T001 | [D] |
| T004 | Fix README.md: change OBS_LEVEL → HSME_OBS_LEVEL | WP02 | no | none | [D] |
| T005 | Add godog BDD test: OBS_LEVEL (wrong) → 0 rows; HSME_OBS_LEVEL (correct) → data | WP02 | no | T004 | [D] |
| T006 | Fix maintenance.go: read last_completed_bucket_start_utc and iterate gaps | WP03 | no | none | [D] |
| T007 | Fix ops/main.go: wire catch-up loop into runRawToMinute | WP03 | no | T006 | [D] |
| T008 | Add godog BDD test: rollup catches up 5 missed buckets post-restart | WP03 | no | T007 | [D] |
| T009 | Edge case: verify gap > retention window is handled gracefully | WP03 | no | T008 | [D] |

---

## WP01: Fix search_fuzzy Project Filter Vector Search

**Summary**: FR-001 fix + BDD coverage (NFR-001) + regression check (NFR-004).

The bug: `src/core/search/fuzzy.go:387` project-branch vector query puts LIMIT after JOIN, which vec0 rejects with "A LIMIT or 'k = ?' constraint is required on vec0 knn queries". Fix: restructure as CTE (KNN scan first with LIMIT, then JOIN to apply project filter).

**Priority**: P1 (highest — primary discovery API broken silently)
**Success criteria**: godog test fails pre-fix with vec0 error + coverage=partial; passes post-fix with coverage=complete
**Test**: godog BDD scenario in `tests/bdd/search_fuzzy_project.feature`

**Subtasks**:
- [x] T001 Fix fuzzy.go: restructure project-branch query as CTE (KNN first, then JOIN)
- [x] T002 Add godog BDD test: search_fuzzy with project filter fails pre-fix, passes post-fix
- [x] T003 Regression: verify search_fuzzy WITHOUT project filter still works

**Implementation sketch**:
1. Read `src/core/search/fuzzy.go` around line 387
2. Find the project-filter branch of VectorSearch
3. Replace direct JOIN+WHERE+LIMIT with CTE: `WITH knn AS (SELECT rowid FROM memory_chunks_vec WHERE embedding = ? LIMIT ?) SELECT ... FROM knn k JOIN memory_chunks c ON ... JOIN memories m ON ... WHERE m.project = ?`
4. Use `LIMIT k*10` to over-fetch and compensate for post-filter row loss
5. Keep non-project branch unchanged (C-001)
6. Validate with existing `go test -tags "sqlite_fts5 sqlite_vec" ./...`

**Risks**: Non-project search must not regress (C-001). CTE over-fetch must not cause OOM on large k values.

**Dependencies**: none

**Estimated prompt size**: ~350 lines

---

## WP02: Fix OBS_LEVEL Documentation Mismatch

**Summary**: FR-002 doc fix + BDD coverage (NFR-002).

The bug: `README.md:127` says `OBS_LEVEL` but the code correctly reads `HSME_OBS_LEVEL`. Fix: change README only. The code is already correct.

**Priority**: P1 (high — entire observability subsystem is invisible to users following docs)
**Success criteria**: godog test shows OBS_LEVEL produces 0 rows; HSME_OBS_LEVEL produces 6+/33+/2+ rows
**Test**: godog BDD scenario in `tests/bdd/observability_env.feature`

**Subtasks**:
- [x] T004 Fix README.md: change OBS_LEVEL → HSME_OBS_LEVEL
- [x] T005 Add godog BDD test: OBS_LEVEL (wrong) → 0 rows; HSME_OBS_LEVEL (correct) → data

**Implementation sketch**:
1. Read `README.md` line ~127
2. Replace all occurrences of `OBS_LEVEL` with `HSME_OBS_LEVEL` in env config examples
3. grep to confirm no remaining `OBS_LEVEL` references
4. Add godog test: start hsme with `OBS_LEVEL=trace`, verify obs_tables empty; restart with `HSME_OBS_LEVEL=trace`, verify 6+/33+/2+ rows

**Risks**: Low — only documentation. Verify grep for any remaining OBS_LEVEL in README.

**Dependencies**: none

**Estimated prompt size**: ~280 lines

---

## WP03: Implement Rollup Catch-up for Missed Buckets

**Summary**: FR-003 fix + BDD coverage (NFR-003) + idempotency/safety (C-003, C-004).

The bug: `runRawToMinute` only processes `now.Truncate(minute)` — never reads `last_completed_bucket_start_utc`. Fix: read checkpoint, calculate gaps, iterate and process each bucket.

**Priority**: P1 (high — data loss when ops process misses a minute)
**Success criteria**: godog test shows 5 missed buckets processed in order after restart
**Test**: godog BDD scenario in `tests/bdd/rollup_catchup.feature`

**Subtasks**:
- [x] T006 Fix maintenance.go: read last_completed_bucket_start_utc and iterate gaps
- [x] T007 Fix ops/main.go: wire catch-up loop into runRawToMinute
- [x] T008 Add godog BDD test: rollup catches up 5 missed buckets post-restart
- [x] T009 Edge case: verify gap > retention window is handled gracefully

**Implementation sketch**:
1. Read `src/observability/maintenance.go` to find runRawToMinute
2. Read `cmd/ops/main.go` to understand how runRawToMinute is invoked
3. Add `getLastCompletedBucket(db)` call to read checkpoint from `obs_rollup_jobs`
4. Replace single-bucket processing with loop: `for ts := lastCheckpoint+1min; ts <= currentBucket; ts += 1min`
5. Update checkpoint after each successful bucket: `updateCheckpoint(db, ts)`
6. Add idempotency guard: check if bucket already processed before calling `processBucket`
7. Handle retention window: skip buckets older than `now - retentionDays`
8. Add godog scenario: simulate 5-minute outage, verify 5 buckets processed in order

**Risks**: Catch-up loop must not OOM on large gaps (e.g., 7-day outage). Idempotency must prevent double-processing. Retention window boundary must be handled correctly.

**Dependencies**: T006 before T007 (code before wiring)

**Estimated prompt size**: ~420 lines

---

## Parallelization Opportunities

- **WP01 and WP02 are fully parallel** (different files, no shared state)
- **WP03 T006/T007 are sequential** (maintenance.go fix before ops/main.go wiring)
- **WP03 T008/T009 can run after T007** (sequential test writing)

## MVP Scope

**WP01** is the MVP — it fixes the core search functionality (primary discovery API) and is the most impactful bug. Recommend implementing WP01 first, then WP02 and WP03 in parallel.

## Work Package Order Recommendation

1. **WP01** first (P1, highest impact: primary discovery API broken silently)
2. **WP02** and **WP03** in parallel (both P1 but independent of each other and of WP01)
