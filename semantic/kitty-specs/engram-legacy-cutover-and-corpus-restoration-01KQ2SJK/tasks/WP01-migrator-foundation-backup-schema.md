---
work_package_id: WP01
title: Migrator Foundation, Backup, and Schema
dependencies: []
requirement_refs:
- C-001
- C-006
- C-007
- FR-001
- FR-008
- FR-009
- FR-010
- NFR-002
- NFR-006
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-engram-legacy-cutover-and-corpus-restoration-01KQ2SJK
base_commit: 458ec6df2b09c0e0859f2c80e5ab00d40f95bec7
created_at: '2026-04-26T03:19:07.855398+00:00'
subtasks:
- T001
- T002
- T003
- T004
- T005
agent: "gemini:1.5-pro:architect:reviewer"
shell_pid: "1579176"
history: []
agent_profile: implementer-ivan
authoritative_surface: cmd/migrate-legacy/
execution_mode: code_change
owned_files:
- cmd/migrate-legacy/main.go
- cmd/migrate-legacy/backup.go
- cmd/migrate-legacy/report.go
- cmd/migrate-legacy/preflight.go
- src/storage/sqlite/db.go
- tests/modules/migrate_legacy_foundation_test.go
role: implementer
tags: []
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Create the executable foundation for the one-shot migrator, make backup/reporting mandatory and deterministic, and land the schema support for `memories.project` so every later phase can rely on it.

## Branch Strategy
Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main. Execution worktrees will later be allocated per computed lane from `lanes.json`; do not assume this WP runs directly in the root checkout.

## Context
This WP covers the lowest-level primitives that every other migration phase needs:
- the CLI and mode dispatch for `full`, `delta`, and `dry-run`
- preflight checks and fail-fast backup handling
- persisted run reports under `data/migrations/<run_id>/`
- the SQLite schema change that adds `memories.project`

Keep the implementation intentionally boring and deterministic. Do not start matching, retagging, or orphan ingestion in this WP.

## Detailed Guidance

### T001: Create the `cmd/migrate-legacy` CLI entrypoint, mode parsing, and phase orchestration skeleton
**Purpose**: Establish the executable surface promised in `contracts/migrator-cli.md`.

**Steps**:
1. Create `cmd/migrate-legacy/main.go` as the binary entrypoint.
2. Define a small config/option structure that captures:
   - mode (`full`, `delta`, `dry-run`)
   - HSME DB path
   - legacy DB path
   - migrations dir
   - unmatched threshold
   - backup/script-related settings
   - Ollama host / embedding model if needed later by downstream phases
3. Parse flags exactly as documented in the CLI contract, including sane defaults from environment variables.
4. Validate the mode enum and print usage errors with exit code `1`.
5. Stub phase orchestration so each phase can return a structured result recorded in the run report.
6. Keep the orchestration decoupled from concrete migration logic; later WPs can fill in phase bodies.

**Implementation Notes**:
- Prefer small helper functions over one huge `main()`.
- Make stdout/stderr formatting match the contract from day one.
- It is acceptable for unimplemented phases to return a clear internal sentinel while this WP is in progress, as long as final handoff for this WP leaves the foundation ready for downstream work.

### T002: Implement preflight validation and hot-backup invocation with fail-fast error handling
**Purpose**: Enforce the mission's safety contract before any mutation happens.

**Steps**:
1. Add `cmd/migrate-legacy/preflight.go` and/or `backup.go` helpers for:
   - verifying HSME DB reachability
   - verifying legacy DB reachability in read-only mode
   - checking required tables/columns that must exist before migration
   - checking that the migrations output directory can be created
2. Implement backup invocation via the existing `scripts/backup_hot.sh` mechanism.
3. Refuse to proceed on backup failure in any mutating mode.
4. Allow backup to be skipped only for explicit test-only scenarios (`--skip-backup`), not by accident.
5. Capture backup path and file metadata into the phase result for report persistence.

**Guardrails**:
- Legacy DB must always be opened read-only/immutable.
- `dry-run` should not mutate and should explain when backup is intentionally skipped.
- Do not silently continue after a failed backup.

### T003: Add run-report generation (`report.json`, `report.txt`, TSV outputs) and stable run directory creation
**Purpose**: Produce auditable outputs for every run, including failures.

**Steps**:
1. Create `cmd/migrate-legacy/report.go`.
2. Model the report structs so they can serialize to:
   - `report.json`
   - `report.txt`
   - `mappings.tsv`
   - `unmatched.tsv`
3. Generate a stable `run_id` and output directory naming scheme that matches the planning artifacts.
4. Ensure the report directory is created before mutating phases begin.
5. Ensure partial/failure runs still persist the report before exit.
6. Record per-phase duration/status/count metadata in a way later WPs can extend without changing the format.

**Implementation Sketch**:
- Define internal structs first.
- Add serializer helpers second.
- Wire the report object into the phase runner third.

### T004: Extend SQLite initialization with the nullable `memories.project` column and project index, idempotently
**Purpose**: Land FR-001/C-006 in the storage layer so search and migrator phases can rely on the column.

**Steps**:
1. Update `src/storage/sqlite/db.go` to add a backwards-compatible schema migration path for `memories.project`.
2. Check for existing column/index presence before applying changes.
3. Ensure repeated initialization remains safe and does not recreate indexes or fail on rerun.
4. Preserve all existing startup behavior for the main HSME server and worker.
5. Add comments only where needed to document the migration rationale.

**Guardrails**:
- Do not redesign the entire schema system.
- Do not break older queries that ignore `project`.
- Keep the migration local to the existing SQLite initialization flow.

### T005: Add migrator foundation tests for preflight, schema migration, and report persistence
**Purpose**: Prove the safety-critical foundation works before higher-risk data mutation code lands.

**Steps**:
1. Add `tests/modules/migrate_legacy_foundation_test.go`.
2. Cover at least:
   - preflight success against temp DBs
   - backup refusal / backup skip behavior in the intended modes
   - idempotent schema creation for `project`
   - report directory/file generation with predictable contents
3. Use temp directories and temp SQLite DBs; do not touch production paths.
4. Keep tests table-driven where practical.

## Implementation Sketch
- CLI/config skeleton first.
- Safety and backup flow second.
- Report persistence third.
- Schema migration fourth.
- Tests last, but validate the safety behavior rather than just happy-path wiring.

## Risks
- Making the CLI contract drift from `contracts/migrator-cli.md`.
- Building report structures that later phases cannot extend cleanly.
- Introducing a schema migration that only works for the migrator but breaks normal HSME startup.

## Definition of Done
- `cmd/migrate-legacy` exists with usable mode parsing and phase orchestration primitives.
- Backup/preflight behavior enforces fail-fast semantics.
- Run reports persist in the documented formats.
- `memories.project` is added idempotently through the SQLite layer.
- Foundation tests cover these behaviors with temp fixtures.

## Reviewer Guidance
Verify that:
- backup is mandatory for real runs
- `dry-run` remains read-only
- legacy DB is opened read-only
- schema migration is additive/idempotent
- report files are written even on failure paths

## Activity Log

- 2026-04-26T03:22:48Z – codex – shell_pid=1573443 – Foundation implemented: CLI skeleton, preflight, backup script, and SQLite project column migration.
- 2026-04-26T03:22:58Z – gemini:1.5-pro:architect:reviewer – shell_pid=1579176 – Started review via action command
- 2026-04-26T03:23:09Z – gemini:1.5-pro:architect:reviewer – shell_pid=1579176 – Review passed: Foundation implementation is complete, follows the contract, and includes robust preflight and backup safety checks.
