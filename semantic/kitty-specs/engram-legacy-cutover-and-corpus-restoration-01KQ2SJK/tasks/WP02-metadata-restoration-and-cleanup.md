---
work_package_id: WP02
title: Metadata Restoration and Cleanup Phases
dependencies:
- WP01
requirement_refs:
- C-002
- C-003
- C-004
- C-005
- C-007
- FR-002
- FR-003
- FR-004
- FR-009
- FR-010
- FR-012
- NFR-001
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-engram-legacy-cutover-and-corpus-restoration-01KQ2SJK
created_at: '2026-04-26T02:52:52Z'
subtasks:
- T006
- T007
- T008
- T009
- T010
agent: "gemini:1.5-pro:architect:reviewer"
shell_pid: "1583658"
history: []
agent_profile: implementer-ivan
authoritative_surface: cmd/migrate-legacy/
execution_mode: code_change
owned_files:
- cmd/migrate-legacy/matcher.go
- cmd/migrate-legacy/restore.go
- cmd/migrate-legacy/cleanup.go
- tests/modules/migrate_legacy_restore_test.go
role: implementer
tags: []
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Implement the heart of the restoration workflow: exact content matching against legacy observations, metadata-only backfill of matched rows, explicit retagging of born-in-HSME summaries, and deterministic garbage cleanup.

## Branch Strategy
Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main. Execution worktrees will later be allocated per computed lane from `lanes.json`; do not assume this WP runs directly in the root checkout.

## Context
This WP must preserve the integrity of the current HSME corpus while fixing bad metadata. The design center is:
- exact content equality after parsing the wrapper
- metadata-only updates for matched rows
- no rewriting of `raw_content`, `content_hash`, chunks, FTS rows, or vectors
- clean idempotent reruns with actionable reporting

## Detailed Guidance

### T006: Implement wrapper parsing plus legacy-observation loading/indexing for exact content matches
**Purpose**: Build the match engine that identifies which migrated rows correspond to real legacy observations.

**Steps**:
1. Create `cmd/migrate-legacy/matcher.go`.
2. Implement the wrapper parser documented in `data-model.md`.
3. Load legacy observations from the read-only Engram DB, excluding deleted rows as specified.
4. Build an efficient lookup keyed by legacy `content` for exact byte-equality matching.
5. Make parser/matcher results rich enough to distinguish:
   - matched rows
   - unparseable wrapper rows
   - no-match rows
   - rows whose legacy counterpart was deleted

**Guardrails**:
- Preserve multi-line content exactly.
- Avoid trimming, normalizing, or fuzzy-matching payloads.
- Treat the malformed empty row as a normal parse/match input here; deletion belongs in phase 5 logic.

### T007: Implement the matched-memory backfill transaction with threshold enforcement and mapping capture
**Purpose**: Apply legacy `created_at`, `source_type`, and `project` values onto the 842 matched migrated rows.

**Steps**:
1. Add a restore-phase implementation file such as `cmd/migrate-legacy/restore.go`.
2. Query HSME for rows still carrying migration source types.
3. For matched rows, update only:
   - `created_at`
   - `source_type`
   - `project`
   - `updated_at` if needed by existing conventions
4. Preserve `raw_content` exactly.
5. Run the restore pass transactionally.
6. Enforce the unmatched-threshold check and record the result in the report.
7. Emit mapping/audit rows so reviewers can trace `{hsme_id, legacy_id, action}`.

**Implementation Notes**:
- This is where idempotency matters most: reruns should naturally skip already-restored rows.
- Keep the SQL explicit and reviewable.

### T008: Implement born-in-HSME retagging and project backfill from wrapper metadata
**Purpose**: Reclassify the 62 fabricated migration-session summaries into normal `session_summary` memories.

**Steps**:
1. Identify rows still tagged `engram_session_migration` after the matched backfill pass.
2. Update those rows to `source_type='session_summary'`.
3. Parse `project` from the wrapper and backfill it if present.
4. Preserve their current `created_at` values per spec.
5. Record counts/actions in the run report.

**Guardrails**:
- Do not lump these rows into the generic unmatched bucket.
- Do not guess projects from fuzzy heuristics; use wrapper metadata only.

### T009: Implement malformed-row cleanup and per-phase idempotent resume guards
**Purpose**: Remove the known garbage row and make reruns predictable after partial completion.

**Steps**:
1. Create cleanup logic in `cmd/migrate-legacy/cleanup.go`.
2. Delete the malformed empty memory by the verified id-or-rule contract from the spec.
3. Rely on existing cascade/triggers for dependent cleanup instead of manual chunk/FTS deletions.
4. Add resume guards so already-restored/already-retagged/already-deleted work naturally no-ops on rerun.
5. Record delete counts and any anomalies in the report.

### T010: Add matcher/backfill tests covering matched, unmatched, malformed, and rerun scenarios
**Purpose**: Lock down the restoration semantics before orphan ingestion builds on top of them.

**Steps**:
1. Add `tests/modules/migrate_legacy_restore_test.go`.
2. Cover:
   - wrapper parsing success/failure
   - exact content match behavior
   - matched-row metadata restoration without `raw_content` mutation
   - retagging of born-in-HSME summaries
   - garbage-row deletion
   - rerun idempotency
3. Use fixture corpora small enough to inspect by hand.
4. Assert audit/report counts where practical.

## Implementation Sketch
- Build parser and lookup logic first.
- Land the transactional matched restore second.
- Add retagging and cleanup third.
- Finish with strong idempotency tests.

## Risks
- Accidentally mutating content-bearing columns.
- Miscounting born-in-HSME summaries.
- Failing to record enough audit data for a reviewer to trust the restoration.

## Definition of Done
- Exact-content matcher exists and distinguishes match outcomes clearly.
- Matched rows restore legacy metadata in place without content rewrites.
- Born-in-HSME summaries are retagged correctly.
- Garbage row cleanup is explicit and idempotent.
- Tests prove reruns settle to the same final state.

## Reviewer Guidance
Verify that:
- `raw_content` never changes for matched rows
- born-in-HSME retagging is not inferred from vague heuristics
- cleanup relies on existing cascades/triggers rather than bespoke SQL fan-out
- rerunning the phase yields no duplicate work

## Activity Log

- 2026-04-26T03:23:26Z – gemini:1.5-pro:architect:implementer – shell_pid=1579799 – Started implementation via action command
- 2026-04-26T03:26:03Z – gemini:1.5-pro:architect:implementer – shell_pid=1579799 – Metadata restoration and cleanup phases implemented: exact-content matcher, backfill transaction, retagging of HSME summaries, and garbage cleanup.
- 2026-04-26T03:26:08Z – gemini:1.5-pro:architect:reviewer – shell_pid=1583658 – Started review via action command
- 2026-04-26T03:26:20Z – gemini:1.5-pro:architect:reviewer – shell_pid=1583658 – Review passed: Metadata restoration and cleanup logic is sound, respects dry-run mode, and follows the data model specification precisely.
