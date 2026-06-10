# Work Packages: Engram Legacy Cutover & Corpus Restoration

**Mission**: `engram-legacy-cutover-and-corpus-restoration-01KQ2SJK`
**Planning branch**: `main`
**Merge target**: `main`

## Subtask Index
| ID | Description | WP | Parallel |
|---|---|---|---|
| T001 | Create the `cmd/migrate-legacy` CLI entrypoint, mode parsing, and phase orchestration skeleton | WP01 | | [D] |
| T002 | Implement preflight validation and hot-backup invocation with fail-fast error handling | WP01 | | [D] |
| T003 | Add run-report generation (`report.json`, `report.txt`, TSV outputs) and stable run directory creation | WP01 | [D] |
| T004 | Extend SQLite initialization with the nullable `memories.project` column and project index, idempotently | WP01 | | [D] |
| T005 | Add migrator foundation tests for preflight, schema migration, and report persistence | WP01 | | [D] |
| T006 | Implement wrapper parsing plus legacy-observation loading/indexing for exact content matches | WP02 | | [D] |
| T007 | Implement the matched-memory backfill transaction with threshold enforcement and mapping capture | WP02 | | [D] |
| T008 | Implement born-in-HSME retagging and project backfill from wrapper metadata | WP02 | | [D] |
| T009 | Implement malformed-row cleanup and per-phase idempotent resume guards | WP02 | | [D] |
| T010 | Add matcher/backfill tests covering matched, unmatched, malformed, and rerun scenarios | WP02 | | [D] |
| T011 | Implement orphan discovery and wrapper reconstruction for legacy-only observations | WP03 | | [D] |
| T012 | Ingest orphan observations through `indexer.StoreContext`, then restore legacy metadata and support `--mode=delta` | WP03 | | [D] |
| T013 | Persist and reuse legacy snapshots so delta mode can ingest only post-cutover writes | WP03 | | [D] |
| T014 | Add cutover verification script plus orphan/delta ingestion tests | WP03 | | [D] |
| T015 | Add optional `project` filtering to core `FuzzySearch` and `ExactSearch` SQL paths | WP04 | | [D] |
| T016 | Thread the optional `project` argument through MCP tool schemas and handlers | WP04 | | [D] |
| T017 | Add search tests for filtered, unfiltered, and empty-result queries, including a rough latency guard | WP04 | | [D] |
| T018 | Add `justfile` targets for building/running the migrator and keep migration outputs ignored from Git | WP05 | [D] |
| T019 | Update repository-facing documentation for the migrator workflow, backup expectations, and delta cutover sequence | WP05 | | [D] |
| T020 | Document the new `project` filter and the single-source-of-truth cutover outcome for operators | WP05 | | [D] |
| T021 | Add a concise operator checklist artifact for T0/T+24h cutover verification from the repo root | WP05 | [D] |

## WP01: Migrator Foundation, Backup, and Schema
**Goal**: Establish the migrator's executable foundation, fail-safe backup/report infrastructure, and the schema change that introduces `memories.project`.
**Prompt**: `tasks/WP01-migrator-foundation-backup-schema.md` (~320 lines)
**Dependencies**: None
**Independent test**: A temporary HSME DB can run migrator preflight/schema/report steps repeatedly, with backup failure aborting cleanly and the `project` column/index appearing exactly once.
**Included Subtasks**:
- [x] T001 Create the `cmd/migrate-legacy` CLI entrypoint, mode parsing, and phase orchestration skeleton (WP01)
- [x] T002 Implement preflight validation and hot-backup invocation with fail-fast error handling (WP01)
- [x] T003 Add run-report generation (`report.json`, `report.txt`, TSV outputs) and stable run directory creation (WP01)
- [x] T004 Extend SQLite initialization with the nullable `memories.project` column and project index, idempotently (WP01)
- [x] T005 Add migrator foundation tests for preflight, schema migration, and report persistence (WP01)
**Implementation Sketch**:
- Stand up the CLI/config/report plumbing first so later phases can log deterministically.
- Add schema migration helpers in the storage layer before wiring phase execution.
- Finish with temp-DB tests that prove preflight, backup refusal, and idempotent schema/report behavior.
**Parallel Opportunities**: T003 can proceed in parallel with T002 once run configuration primitives exist.
**Risks**:
- Accidentally making backup optional in real runs.
- Schema migration logic becoming non-idempotent across reruns.
- Report formats drifting from the contracts already documented for operators.

## WP02: Metadata Restoration and Cleanup Phases
**Goal**: Restore legacy metadata onto matched migrated rows, retag born-in-HSME session summaries, and remove the malformed garbage row without rewriting `raw_content`.
**Prompt**: `tasks/WP02-metadata-restoration-and-cleanup.md` (~340 lines)
**Dependencies**: WP01
**Independent test**: A fixture corpus with wrapped migrated rows and a read-only legacy DB restores matched metadata, retags fabricated summaries, deletes the garbage row, and produces zero extra mutations on rerun.
**Included Subtasks**:
- [x] T006 Implement wrapper parsing plus legacy-observation loading/indexing for exact content matches (WP02)
- [x] T007 Implement the matched-memory backfill transaction with threshold enforcement and mapping capture (WP02)
- [x] T008 Implement born-in-HSME retagging and project backfill from wrapper metadata (WP02)
- [x] T009 Implement malformed-row cleanup and per-phase idempotent resume guards (WP02)
- [x] T010 Add matcher/backfill tests covering matched, unmatched, malformed, and rerun scenarios (WP02)
**Implementation Sketch**:
- Build the wrapper parser and exact-match index first.
- Execute phase 3 in a single transaction that only mutates metadata columns.
- Follow with explicit phase-4 retagging and phase-5 deletion logic, then lock it down with idempotency tests.
**Parallel Opportunities**: None recommended; the matcher and cleanup phases are tightly coupled and should land together.
**Dependencies / Notes**: This WP assumes the `project` column and report infrastructure from WP01 already exist.
**Risks**:
- Misclassifying born-in-HSME summaries into the generic unmatched bucket.
- Rewriting `raw_content` or touching chunks/FTS indirectly.
- Incomplete audit mappings that make restoration hard to review.

## WP03: Orphan Ingestion, Delta Mode, and Cutover Verification
**Goal**: Ingest legacy-only observations through the normal HSME ingest path, support post-cutover delta replay, and give operators a verification script for the 24-hour no-writes check.
**Prompt**: `tasks/WP03-orphan-ingestion-delta-and-verification.md` (~300 lines)
**Dependencies**: WP01, WP02
**Independent test**: Given legacy observations absent from HSME, full mode ingests them once, delta mode only ingests rows newer than the saved snapshot baseline, and `scripts/verify_cutover.sh` emits the documented TSV contract.
**Included Subtasks**:
- [x] T011 Implement orphan discovery and wrapper reconstruction for legacy-only observations (WP03)
- [x] T012 Ingest orphan observations through `indexer.StoreContext`, then restore legacy metadata and support `--mode=delta` (WP03)
- [x] T013 Persist and reuse legacy snapshots so delta mode can ingest only post-cutover writes (WP03)
- [x] T014 Add cutover verification script plus orphan/delta ingestion tests (WP03)
**Implementation Sketch**:
- Reuse the matcher output to derive the anti-join set of orphans.
- Persist pre-cutover snapshot metadata into the run report in full mode.
- Implement delta-mode replay against that baseline and close with script/test coverage.
**Parallel Opportunities**: The shell verification script can be drafted while Go-side delta logic is being implemented, but final validation depends on the snapshot/report format.
**Dependencies / Notes**: Requires WP02 match outputs and WP01 reporting primitives.
**Risks**:
- Accidentally bypassing `StoreContext` and recreating the original migration indexing bug.
- Delta mode choosing the wrong baseline report or re-ingesting older rows.
- Telemetry script mutating or locking the legacy DB instead of reading it safely.

## WP04: Search Project Filter Surface
**Goal**: Add the optional `project` filter end-to-end for `search_fuzzy` and `search_exact` without changing unfiltered behavior or result contracts.
**Prompt**: `tasks/WP04-search-project-filter-surface.md` (~260 lines)
**Dependencies**: WP01
**Independent test**: Search results are unchanged when `project` is omitted, restricted correctly when `project` is supplied, and empty cleanly for nonexistent projects.
**Included Subtasks**:
- [x] T015 Add optional `project` filtering to core `FuzzySearch` and `ExactSearch` SQL paths (WP04)
- [x] T016 Thread the optional `project` argument through MCP tool schemas and handlers (WP04)
- [x] T017 Add search tests for filtered, unfiltered, and empty-result queries, including a rough latency guard (WP04)
**Implementation Sketch**:
- Extend the search-function signatures first, keeping empty-string behavior as a no-op.
- Update MCP input schemas and argument extraction second.
- Finish with fixture-backed tests covering both tools and a coarse performance guard.
**Parallel Opportunities**: None recommended; schema/handler/test updates are tightly coupled.
**Dependencies / Notes**: Depends on the `project` column existing, but can otherwise proceed independently of the migrator phases.
**Risks**:
- Filtering one leg of RRF but not the other, causing cross-project leakage.
- Accidentally changing unfiltered result ordering.
- Exposing a new required MCP parameter instead of an optional additive one.

## WP05: Operator Build & Documentation Handoff
**Goal**: Make the migration operable from the repo root by adding build/run targets, repo-facing docs, and a concise operator checklist for cutover verification.
**Prompt**: `tasks/WP05-operator-build-and-documentation-handoff.md` (~220 lines)
**Dependencies**: WP03, WP04
**Independent test**: A fresh operator can discover the migrator build target, follow the repo docs to run full + delta + verification steps, and find the `project` filter documented without opening mission planning artifacts.
**Included Subtasks**:
- [x] T018 Add `justfile` targets for building/running the migrator and keep migration outputs ignored from Git (WP05)
- [x] T019 Update repository-facing documentation for the migrator workflow, backup expectations, and delta cutover sequence (WP05)
- [x] T020 Document the new `project` filter and the single-source-of-truth cutover outcome for operators (WP05)
- [x] T021 Add a concise operator checklist artifact for T0/T+24h cutover verification from the repo root (WP05)
**Implementation Sketch**:
- Add the operator-facing build/run targets first.
- Update README and any lightweight ops doc/checklist second so the repository itself explains the cutover workflow.
- Ensure the documentation explicitly states that Claude Code should write only to HSME after cutover.
**Parallel Opportunities**: T018 and T021 can proceed in parallel once command names and output paths are final.
**Dependencies / Notes**: Wait for WP03/WP04 so command names, filter behavior, and cutover steps are final before documenting them.
**Risks**:
- Docs drifting from actual CLI flags or report paths.
- Forgetting to mention the manual Claude MCP reconfiguration boundary.
- Accidentally tracking generated migration outputs in Git.
