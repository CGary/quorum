# Implementation Plan: Engram Legacy Cutover & Corpus Restoration

**Branch**: `main` | **Date**: 2026-04-26 | **Spec**: `kitty-specs/engram-legacy-cutover-and-corpus-restoration-01KQ2SJK/spec.md`
**Mission**: `engram-legacy-cutover-and-corpus-restoration-01KQ2SJK`
**Mission ID**: `01KQ2SJK44AP2YDKXSBKPZCB8Q`

## Summary

Restore lost chronology and project metadata for the migrated Engram corpus by matching wrapped HSME memories against the legacy Engram observations database, backfilling `created_at` / `source_type` / `project` for matched rows, retagging born-in-HSME summaries, deleting the malformed garbage row, ingesting legacy-only post-migration observations through the normal HSME ingestion path, and cutting Claude Code over so HSME becomes the only active write target. The migration is delivered as a dedicated Go binary under `cmd/migrate-legacy/`, performs a mandatory hot backup before mutation, persists an auditable run report under `data/migrations/`, and leaves downstream recency missions unblocked by restoring real timestamps first.

## Technical Context

| Item | Value |
|------|-------|
| **Language/Version** | Go (same module/toolchain as the rest of HSME; build tags `sqlite_fts5 sqlite_vec` remain mandatory where SQLite features are exercised). |
| **Primary Dependencies** | Standard library plus the repo's existing `mattn/go-sqlite3`; internal reuse of `src/storage/sqlite`, `src/core/indexer`, and existing MCP/search packages. No new third-party runtime dependencies required. |
| **Storage** | Primary HSME DB at `data/engram.db` (read-write); legacy Engram DB at `/home/gary/.engram/engram.db` (strictly read-only); migration reports written under `data/migrations/`. |
| **Testing** | Standard Go tests under `tests/modules/` with temp SQLite fixtures; build/test commands continue to use `-tags "sqlite_fts5 sqlite_vec"`. |
| **Target Platform** | Linux developer workstation running the existing HSME / Ollama stack. |
| **Project Type** | Single Go module with multiple `cmd/` binaries and a shared `src/` tree. |
| **Performance Goals** | Backfill/migration complete within ~10 minutes for the current ~965-row scope; orphan ingest stays bounded by existing Ollama throughput; search latency after `project` filter remains within the spec budget. |
| **Constraints** | Mandatory hot backup before mutation; legacy DB never modified; matched memories keep `raw_content` unchanged; migration must be idempotent and resumable by rerun; cutover remains manual/fail-loud at the Claude MCP layer. |
| **Scale/Scope** | 905 migrated HSME memories + 59 orphan legacy observations + 1 malformed garbage row, plus additive `project` filter support for `search_fuzzy` / `search_exact`. |

## Engineering Alignment

The design is intentionally split into a correctness-first Mission 1 and two future recency missions:
- Mission 1 (this mission) repairs the corpus so chronology and project scoping become trustworthy.
- `docs/future-missions/mission-2-recency-fast-path.md` depends on this mission's restored timestamps and populated `project` column.
- `docs/future-missions/mission-3-rrf-time-decay.md` depends on Mission 1 and Mission 2 baselines before touching ranking quality.

This keeps the current mission focused on data correctness, auditability, and cutover safety, while deferring ranking-quality changes to later missions with real baselines.

## Charter Check

**SKIPPED** — Charter context bootstrap reports that `.kittify/charter/charter.md` does not exist for this repository. No charter gates are available, so standard engineering safeguards apply instead: backup-before-mutation, explicit read-only handling of legacy state, idempotent reruns, and auditable outputs.

## Project Structure

### Documentation (this mission)

```
kitty-specs/engram-legacy-cutover-and-corpus-restoration-01KQ2SJK/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── migrator-cli.md
│   ├── search-tools.md
│   └── verify-cutover.md
├── checklists/
│   └── requirements.md
└── tasks/                    # produced later by /spec-kitty.tasks
```

### Source Code (repository root)

```
cmd/
├── hsme/                     # MCP server entrypoint
├── worker/                   # async task worker
├── ops/                      # observability / maintenance runner
└── migrate-legacy/           # NEW: one-shot migration binary
    ├── main.go
    ├── phases.go
    ├── matcher.go
    ├── report.go
    ├── backup.go
    └── ...

src/
├── core/
│   ├── indexer/              # reused for orphan ingestion
│   └── search/
│       └── fuzzy.go          # add optional project filter + exact search update surface
├── mcp/
│   └── handler.go            # expose optional project filter to tools
└── storage/
    └── sqlite/
        └── db.go             # additive schema migration for memories.project

scripts/
├── backup_hot.sh             # existing hot-backup path
├── restore.sh                # existing restore path
└── verify_cutover.sh         # NEW: post-cutover telemetry snapshot helper

tests/
└── modules/
    ├── migrate_legacy_*.go   # NEW: migration tests
    ├── search*_test.go       # updated search coverage for project filter
    └── ...

data/
└── migrations/               # NEW operational output, gitignored
```

**Structure Decision**: Keep this as a monolithic Go-module change. The migrator is a separate `cmd/` binary because it is operationally distinct, testable, and intentionally one-shot, while storage/search/MCP changes remain in the existing shared packages they extend.

## Phase Definition

The implementation is organized into the following logical phases:

| Phase | Goal | Key Outputs |
|------|------|-------------|
| **Phase 0 — Preflight & Backup** | Verify both DBs, required schema assumptions, report directory, and backup safety before mutation. | Preflight checks, backup metadata in report |
| **Phase 1 — Schema** | Add `memories.project` and its supporting index, idempotently. | Additive schema migration in `src/storage/sqlite/db.go` |
| **Phase 2 — Match & Restore** | Parse wrapped migrated memories and restore metadata from legacy observations without rewriting `raw_content`. | Matcher + matched-row backfill |
| **Phase 3 — Retag & Cleanup** | Retag born-in-HSME summaries and delete the malformed garbage row. | Explicit phase logic + audit counts |
| **Phase 4 — Snapshot & Orphans** | Capture pre-cutover snapshot and ingest legacy-only observations through `indexer.StoreContext`. | Orphan ingestion + persisted run report |
| **Phase 5 — Search Surface** | Add optional `project` filter to `search_fuzzy` and `search_exact`. | Core search + MCP handler/schema updates |
| **Phase 6 — Operator Cutover Support** | Provide delta-mode replay and `scripts/verify_cutover.sh` for manual cutover verification. | Delta replay + verification script + docs |

## Phase 0: Outline & Research

Research conclusions are already captured in `kitty-specs/engram-legacy-cutover-and-corpus-restoration-01KQ2SJK/research.md`. The key planning decisions established there are:
- exact content equality is the only trustworthy match strategy for migrated rows;
- `ALTER TABLE ... ADD COLUMN project` is the correct low-risk schema path;
- one transaction per logical mutation phase is simpler and safer than one giant transaction;
- matched memories must not have `raw_content` rewritten;
- orphan observations must go through `indexer.StoreContext` instead of raw SQL inserts;
- Claude MCP cutover remains manual/fail-loud, with a delta replay immediately after removal of legacy Engram from Claude Code.

No `[NEEDS CLARIFICATION]` markers remain in the research artifact.

## Phase 1: Design & Contracts

Design artifacts are already present and remain authoritative:
- `kitty-specs/engram-legacy-cutover-and-corpus-restoration-01KQ2SJK/data-model.md`
- `kitty-specs/engram-legacy-cutover-and-corpus-restoration-01KQ2SJK/contracts/migrator-cli.md`
- `kitty-specs/engram-legacy-cutover-and-corpus-restoration-01KQ2SJK/contracts/search-tools.md`
- `kitty-specs/engram-legacy-cutover-and-corpus-restoration-01KQ2SJK/contracts/verify-cutover.md`
- `kitty-specs/engram-legacy-cutover-and-corpus-restoration-01KQ2SJK/quickstart.md`

They define:
- the `memories.project` data model extension;
- the wrapper grammar and exact matching contract;
- the `migrate-legacy` CLI modes (`full`, `delta`, `dry-run`);
- the additive `project` filter on `search_fuzzy` and `search_exact`;
- the T0 / T+24h telemetry contract for verifying that legacy Engram stops receiving writes.

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Matching less than the expected ~93% baseline | Medium | Enforce unmatched-threshold reporting, keep unmatched rows explicit, and refuse silent metadata invention. |
| Data loss during migration | High | Mandatory hot backup before any mutation, per-phase transaction boundaries, idempotent reruns, and read-only legacy access. |
| Repeating the original migration bug by bypassing indexing | High | Ingest orphans through `src/core/indexer.StoreContext` only. |
| Search filter regressions | Medium | Keep `project` optional and additive; test both filtered and unfiltered paths. |
| Manual cutover race window | Medium | Persist snapshot baseline before cutover and require an immediate `--mode=delta` replay after `claude mcp remove engram`. |
| Future recency missions being specified against fake chronology | High | Explicitly keep Mission 2 and Mission 3 out of scope until this mission lands and produces real baselines. |

## Definition of Done

1. `kitty-specs/engram-legacy-cutover-and-corpus-restoration-01KQ2SJK/research.md`, `data-model.md`, `contracts/`, and `quickstart.md` remain aligned with the implementation plan.
2. `plan.md` is fully materialized — no template placeholders, no unresolved clarification markers.
3. The mission remains scoped to correctness/cutover, not future ranking enhancements.
4. The plan is ready for `/spec-kitty.tasks` to decompose into execution work packages.

## Out of Scope

Explicitly deferred to later mission creation in `docs/future-missions/`:
- `docs/future-missions/mission-2-recency-fast-path.md` — new `recall_recent_session` MCP tool and protocol guidance.
- `docs/future-missions/mission-3-rrf-time-decay.md` — recency-aware scoring changes inside hybrid search.
- `ideas/cli-tool.md` and `ideas/graph-cleanup-maintenance.md` remain separate future work items.

## Branch Contract Reaffirmation

- **Current branch at plan completion:** `main`
- **Planning/base branch:** `main`
- **Final merge target:** `main`
- **Current branch matches intended target:** `true`

## Next Step

Run `/spec-kitty.tasks` for `kitty-specs/engram-legacy-cutover-and-corpus-restoration-01KQ2SJK/` to regenerate or validate work-package decomposition against this completed plan. Do **not** proceed directly to implementation from `/spec-kitty.plan`.
