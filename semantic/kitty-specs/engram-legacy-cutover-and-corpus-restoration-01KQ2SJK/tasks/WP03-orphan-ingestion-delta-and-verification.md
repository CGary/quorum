---
work_package_id: WP03
title: Orphan Ingestion, Delta Mode, and Cutover Verification
dependencies:
- WP01
- WP02
requirement_refs:
- C-004
- FR-005
- FR-010
- FR-011
- FR-012
- NFR-003
- NFR-005
- NFR-006
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-engram-legacy-cutover-and-corpus-restoration-01KQ2SJK
created_at: '2026-04-26T02:52:52Z'
subtasks:
- T011
- T012
- T013
- T014
agent: "gemini:1.5-pro:architect:reviewer"
shell_pid: "1588403"
history: []
agent_profile: implementer-ivan
authoritative_surface: cmd/migrate-legacy/
execution_mode: code_change
owned_files:
- cmd/migrate-legacy/orphans.go
- cmd/migrate-legacy/delta.go
- scripts/verify_cutover.sh
- tests/modules/migrate_legacy_orphans_test.go
role: implementer
tags: []
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Bring the missing legacy-only observations into HSME through the normal ingest path, support the post-cutover delta replay window, and provide the operator-facing verification script for the 24-hour no-writes check.

## Branch Strategy
Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge back into main. Execution worktrees will later be allocated per computed lane from `lanes.json`; do not assume this WP runs directly in the root checkout.

## Context
This WP deliberately uses the existing HSME ingestion path instead of raw SQL inserts. That preserves chunking, lexical index sync, and async task enqueue semantics. It also finishes the operational story for the manual cutover by providing:
- a snapshot baseline
- a delta-only replay path
- a tiny verification script for the T0/T+24h check

## Detailed Guidance

### T011: Implement orphan discovery and wrapper reconstruction for legacy-only observations
**Purpose**: Find the 59 observations present only in legacy Engram and prepare them for normal HSME ingestion.

**Steps**:
1. Create `cmd/migrate-legacy/orphans.go`.
2. Use matcher outputs or a direct anti-join strategy to identify legacy observations with no HSME counterpart.
3. Rebuild the wrapper format exactly as documented for new orphan ingestions.
4. Exclude legacy deleted rows and already-ingested rows on rerun.
5. Keep the discovery output explicit enough to surface in reports/tests.

### T012: Ingest orphan observations through `indexer.StoreContext`, then restore legacy metadata and support `--mode=delta`
**Purpose**: Ensure orphan rows enter HSME through the same operational path as normal writes.

**Steps**:
1. Call `indexer.StoreContext` for each orphan instead of raw INSERTs.
2. After each successful store, restore/override legacy metadata required by the spec (`created_at`, `source_type`, `project`) in the same logical flow.
3. Ensure reruns skip already-ingested orphans cleanly.
4. Implement `delta` mode so it reuses the same ingestion path but narrows candidates to post-cutover writes only.
5. Respect existing embedding/worker behavior; phase success here means ingestion succeeded synchronously, not that downstream async tasks have fully drained.

**Guardrails**:
- Do not bypass the indexer.
- Do not require the worker to finish during the command run.
- Keep idempotency centered on HSME state, not on optimistic memory of prior runs.

### T013: Persist and reuse legacy snapshots so delta mode can ingest only post-cutover writes
**Purpose**: Provide a deterministic baseline for the race-window replay after Claude Code MCP cutover.

**Steps**:
1. Record legacy snapshot details (rowcount, max_created_at, filesize as appropriate) in the run report during full runs.
2. Add logic in `cmd/migrate-legacy/delta.go` to locate the prior successful baseline needed for `delta` mode.
3. Narrow delta ingestion to rows newer than the stored baseline.
4. Make baseline lookup and failure cases explicit in stdout/stderr/report output.
5. Keep the implementation simple enough for operators to reason about if a delta run needs manual review.

### T014: Add cutover verification script plus orphan/delta ingestion tests
**Purpose**: Finish the operational contract with automated coverage and a human-usable verification surface.

**Steps**:
1. Add `scripts/verify_cutover.sh` following the documented TSV contract.
2. Make the script accept the legacy DB path flag/default environment handling described in the contract.
3. Add `tests/modules/migrate_legacy_orphans_test.go` covering:
   - orphan detection
   - ingestion through the indexer path
   - delta replay baseline filtering
   - rerun idempotency
4. Add a lightweight script-level assertion if practical (or validate output shape in Go/integration style).

## Implementation Sketch
- Orphan discovery first.
- Shared ingest flow second.
- Delta baseline persistence/lookup third.
- Verification script and tests last.

## Risks
- Reintroducing the original migration bug by skipping chunk/index/task creation.
- Delta mode selecting the wrong baseline report.
- Script output drifting from the runbook/contract and confusing operators.

## Definition of Done
- Legacy-only rows can be identified and wrapped deterministically.
- Full and delta runs ingest only the intended rows.
- Verification script emits the documented single-line TSV snapshot.
- Tests cover full, delta, and rerun behavior.

## Reviewer Guidance
Verify that:
- orphan ingestion goes through `indexer.StoreContext`
- delta mode depends on a persisted baseline rather than wall-clock guesses
- the verification script is read-only and shell-simple

## Activity Log

- 2026-04-26T03:26:26Z – gemini:1.5-pro:architect:implementer – shell_pid=1584086 – Started implementation via action command
- 2026-04-26T03:28:58Z – gemini:1.5-pro:architect:implementer – shell_pid=1584086 – Orphan ingestion, delta mode, and verification script implemented. Delta mode uses baseline reports for safe race-window replays.
- 2026-04-26T03:29:12Z – gemini:1.5-pro:architect:reviewer – shell_pid=1588403 – Started review via action command
- 2026-04-26T03:29:41Z – gemini:1.5-pro:architect:reviewer – shell_pid=1588403 – Review passed: Orphan ingestion is correctly routed through the indexer, and delta mode baseline persistence ensures operational safety during cutover.
