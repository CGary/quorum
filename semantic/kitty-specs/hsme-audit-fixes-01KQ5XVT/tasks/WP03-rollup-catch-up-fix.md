---
work_package_id: WP03
title: Implement Rollup Catch-up for Missed Buckets (FR-003)
dependencies: []
requirement_refs:
- C-003
- C-004
- FR-003
- FR-004
- NFR-003
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-hsme-audit-fixes-01KQ5XVT
base_commit: dac3ef13d27e4bdafeedb434cc01af9c2c24f149
created_at: '2026-04-27T11:51:29.662609+00:00'
subtasks:
- T006
- T007
- T008
- T009
agent: "gemini:flash:reviewer:reviewer"
shell_pid: "2823102"
history: []
authoritative_surface: src/observability/
execution_mode: code_change
owned_files:
- src/observability/maintenance.go
- cmd/ops/main.go
- src/storage/sqlite/observability.go
- tests/bdd/rollup_catchup.feature
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

Fix FR-003: `runRawToMinute` and `runDerivedRollup` only process the current bucket (`now.Truncate(minute)`). They never read `last_completed_bucket_start_utc` to determine where to resume. If ops doesn't run for 5 minutes, those 5 buckets are permanently lost when retention expires. The fix reads the checkpoint, calculates gaps, and iterates through all missed buckets.

## Branch Strategy

Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main. Execution worktrees are allocated per computed lane from `lanes.json`.

## Context from Spec and Plan

**Root cause** (confirmed from HSME memory 993, post-merge audit):
- Checkpoint `last_completed_bucket_start_utc` IS persisted to `obs_rollup_jobs` table
- BUT `runRawToMinute` never reads it — only processes `now.Truncate(minute)`
- The catch-up logic needs to be added

**Current behavior** (buggy):
```go
func runRawToMinute(ctx context.Context, db *DB) error {
    currentBucket := now.Truncate(time.Minute)
    return processBucket(ctx, db, currentBucket)
}
```

**Required behavior** (fixed):
```go
func runRawToMinute(ctx context.Context, db *DB) error {
    lastCheckpoint := getLastCompletedBucket(db)   // NEW: read persisted checkpoint
    currentBucket := now.Truncate(time.Minute)

    // Catch-up loop: process all missed buckets
    for ts := lastCheckpoint.Add(1*time.Minute); !ts.After(currentBucket); ts = ts.Add(time.Minute) {
        if ts.After(time.Now().Add(-retentionWindow)) {
            // Only process within retention window
            if err := processBucket(ctx, db, ts); err != nil {
                return err
            }
            updateCheckpoint(db, ts)  // NEW: update after each bucket
        }
    }
    return nil
}
```

**Key constraints**:
- C-003: Must not reprocess buckets already marked in checkpoint
- C-004: Catch-up must be idempotent — running twice for same bucket produces same result

**HSME Memory**: 993 (post-merge audit confirming root cause), 988 (observability runtime)

## Subtasks

### T006: Fix maintenance.go — Read Checkpoint and Iterate Missed Buckets

**Purpose**: Add catch-up loop to `runRawToMinute` in `maintenance.go`.

**Steps**:
1. Read `src/observability/maintenance.go` — find `runRawToMinute` function
2. Read `src/storage/sqlite/observability.go` — find the schema for `obs_rollup_jobs` and `last_completed_bucket_start_utc` field
3. Find the `getLastCompletedBucket(db)` function (or create it if missing) that reads `last_completed_bucket_start_utc` from `obs_rollup_jobs`
4. Modify `runRawToMinute` to:
   - Call `getLastCompletedBucket(db)` to get last processed timestamp
   - Calculate `currentBucket = now.Truncate(time.Minute)`
   - Loop from `lastCheckpoint + 1 minute` up to `currentBucket`
   - For each bucket, call `processBucket(ctx, db, ts)`
   - After successful processing, call `updateCheckpoint(db, ts)` to persist new checkpoint
5. Add idempotency guard: before processing a bucket, check if it's already processed (skip if already in checkpoint)
6. Add retention window guard: skip buckets older than `now - retentionWindow` (default 7 days from AS-003)
7. Run `go build ./cmd/ops/...` to verify compilation

**Files to modify**:
- `src/observability/maintenance.go`
- `src/storage/sqlite/observability.go` (if `getLastCompletedBucket` doesn't exist yet)

**Validation**:
- [ ] `runRawToMinute` reads checkpoint instead of just processing current bucket
- [ ] Catch-up loop iterates all missed buckets
- [ ] Checkpoint is updated after each successful bucket
- [ ] Idempotency: re-running doesn't double-process
- [ ] Retention window: buckets outside window are skipped gracefully

**Risks**:
- Large gap (e.g., 7 days) could cause many iterations — ensure loop is bounded by retention window
- Concurrent execution: two ops processes running simultaneously could race on checkpoint updates — consider using DB transactions

---

### T007: Fix ops/main.go — Wire Catch-up into Rollup Invocation

**Purpose**: Ensure `ops/main.go` correctly calls the updated `runRawToMinute`.

**Steps**:
1. Read `cmd/ops/main.go` — find where `runRawToMinute` is called
2. Verify the catch-up behavior is wired correctly (T006 should have updated the function, this step confirms it's invoked properly)
3. If `ops/main.go` has any special handling (flags, modes, etc.), ensure catch-up is invoked in the right mode
4. Check if `runRawToMinute` is called with a context that has a timeout — catch-up for many buckets could take time; ensure timeout is appropriate
5. Run `go build ./cmd/ops/...` to verify compilation

**Files to verify/modify**:
- `cmd/ops/main.go`

**Validation**:
- [ ] `ops` binary builds successfully: `go build ./cmd/ops/...`
- [ ] Catch-up loop is triggered when `ops` runs rollup mode

**Note**: This subtask may be trivial if `ops/main.go` already calls `runRawToMinute` correctly and the fix in T006 is transparent to the caller. If so, just verify and document.

---

### T008: Add godog BDD Test — Rollup Catches Up 5 Missed Buckets

**Purpose**: NFR-003 — Add BDD scenario that FAILS before fix and PASSES after.

**Steps**:
1. Create `tests/bdd/rollup_catchup.feature`:

```gherkin
Feature: Rollup catch-up for missed buckets

  Scenario: Rollup processes current bucket normally
    Given the hsme-ops service is running with rollup configured
    And no buckets have been processed yet
    When the cron trigger executes runRawToMinute
    Then the bucket of now.Truncate(minute) is processed
    And last_completed_bucket_start_utc is updated to current bucket

  Scenario: Rollup catches up 5 missed buckets after restart
    Given the hsme-ops service has processed up to bucket T-10min
    And the service was down for 5 minutes (buckets T-5min through T-1min missed)
    When the service restarts and executes runRawToMinute
    Then buckets T-5min, T-4min, T-3min, T-2min, T-1min are all processed in order
    And last_completed_bucket_start_utc advances sequentially
    And no bucket is lost due to the gap

  Scenario: Rollup does not reprocess already-completed buckets
    Given the hsme-ops service has processed up to bucket T-5min
    When runRawToMinute executes
    Then bucket T-5min is NOT reprocessed
    And processing starts from T-4min

  Scenario: Idempotency — running twice produces same result
    Given the hsme-ops service has processed bucket T-5min
    When runRawToMinute executes twice in succession
    Then the second run produces identical checkpoint state
    And no duplicate processing occurs
```

2. Create `tests/bdd/rollup_catchup_test.go` — godog test suite that:
   - Simulates outage scenario (process stops, 5 minutes pass, buckets are missed)
   - Verifies checkpoint is NOT updated for missed buckets during outage
   - On restart, verifies all 5 missed buckets ARE processed
   - Verifies idempotency
3. Run `godog run tests/bdd/rollup_catchup.feature` — should FAIL before T006/T007 fix (only current bucket processed)
4. After T006/T007 fix, run godog again — should PASS

**Files to create**:
- `tests/bdd/rollup_catchup.feature`
- `tests/bdd/rollup_catchup_test.go`

**Validation**:
- [ ] godog test FAILS on main (before T006/T007) — only current bucket processed
- [ ] godog test PASSES after fix — 5 missed buckets processed in order
- [ ] Idempotency scenario passes

**Pre-fix expected output**: Only `now.Truncate(minute)` bucket processed, checkpoint jumps
**Post-fix expected output**: All missed buckets processed sequentially

---

### T009: Edge Case — Gap Larger Than Retention Window

**Purpose**: Ensure C-003/C-004 are respected — gaps larger than retention window are handled gracefully.

**Steps**:
1. Add a godog scenario to `tests/bdd/rollup_catchup.feature`:

```gherkin
  Scenario: Gap larger than retention window is handled gracefully
    Given the hsme-ops service was down for more than 7 days
    When the service restarts and executes runRawToMinute
    Then only buckets within the 7-day retention window are processed
    And buckets outside retention window are skipped without error
    And checkpoint is updated to the oldest bucket within retention window
```

2. Add to `tests/bdd/rollup_catchup_test.go`:
   - Simulate outage longer than retention window (e.g., 10 days)
   - Verify only buckets within retention window are processed
   - Verify no error is raised for out-of-window buckets
3. Run `godog run tests/bdd/rollup_catchup.feature` — this scenario should pass with the fix in place

**Validation**:
- [ ] Out-of-retention-window buckets are skipped without error
- [ ] Checkpoint is set to oldest in-window bucket
- [ ] No crash or error on large gaps

**Risk**: If retention window is not configurable or hardcoded incorrectly, this could fail. Verify retention window handling from schema/config.

## Definition of Done

- [ ] `godog run tests/bdd/rollup_catchup.feature` PASSES (post-fix)
- [ ] Catch-up processes all missed buckets within retention window
- [ ] Idempotency verified (same bucket not processed twice)
- [ ] Retention window edge case handled gracefully
- [ ] `go build ./cmd/ops/...` compiles successfully
- [ ] No files outside `owned_files` modified

## Risks

| Risk | Mitigation |
|------|------------|
| Large gap (e.g., 7-day outage) causes many iterations | Bound loop by retention window — only process in-window buckets |
| Concurrent ops processes race on checkpoint | Use DB transaction with row locking on obs_rollup_jobs |
| Retention window not configurable | Check schema — AS-003 says configurable with 7d default; verify |
| Catch-up modifies bucket that is still being written | Coordination with write path (if any) — may need separate analysis |

## Reviewer Guidance

- The diff for `maintenance.go` should add a loop that reads checkpoint and iterates missed buckets
- Verify `getLastCompletedBucket` is called at start of `runRawToMinute`
- Verify `updateCheckpoint` is called after each successful bucket
- Retention window guard should skip buckets older than `now - retentionWindow`
- Idempotency: verify bucket already in checkpoint is not reprocessed
- Run `godog run tests/bdd/rollup_catchup.feature` — must pass
- Run `go build ./cmd/ops/...` — must compile

## Activity Log

- 2026-04-27T15:31:38Z – claude – shell_pid=2504561 – Implemented FR-003 by adding a catch-up loop to rollup jobs. The service now reads the last completed bucket checkpoint and iterates through any missed buckets within the retention window. Added comprehensive BDD tests for catch-up, idempotency, and retention window handling.
- 2026-04-27T15:47:34Z – gemini:flash:reviewer:reviewer – shell_pid=2823102 – Started review via action command
- 2026-04-27T15:49:21Z – gemini:flash:reviewer:reviewer – shell_pid=2823102 – Review passed: Implementation successfully adds catch-up logic to rollup jobs. The generic runJobWithCatchup helper ensures consistency across different rollup levels. Idempotency and retention window guards are correctly implemented. BDD tests verify the catch-up behavior for multiple missed buckets.
