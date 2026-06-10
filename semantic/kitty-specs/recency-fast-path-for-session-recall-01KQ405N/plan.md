# Implementation Plan: Recency Fast Path for Session Recall

**Branch**: `main` | **Date**: 2026-04-26 | **Spec**: `kitty-specs/recency-fast-path-for-session-recall-01KQ405N/spec.md`
**Mission**: `recency-fast-path-for-session-recall-01KQ405N`
**Mission ID**: `01KQ405N8N65EAS0WAWFXPS0AY`

## Summary

Add a dedicated MCP tool, `recall_recent_session`, that retrieves the most recent active `session_summary` memories by chronological order instead of semantic relevance. The tool performs a pure SQL lookup over `memories`, supports an optional `project` filter, caps `limit` at 50 server-side, and never invokes the embedder. To support this feature cleanly, the plan also includes an additive composite index on `(source_type, created_at DESC)`, a minimal MCP registration surface, repo protocol guidance in `CLAUDE.md`, and mechanical fixes for currently broken Go tests caused by prior search/indexer signature changes.

## Technical Context

| Item | Value |
|------|-------|
| **Language/Version** | Go, same module/toolchain as the existing HSME binaries. |
| **Primary Dependencies** | Standard library plus the repo's current SQLite stack (`mattn/go-sqlite3`) and internal packages under `src/core/search`, `src/storage/sqlite`, and `cmd/hsme`. No new third-party dependencies required. |
| **Storage** | SQLite at `data/engram.db`; additive index on `memories(source_type, created_at DESC)`; read-only query path for the new tool. |
| **Testing** | Go tests under `tests/modules/` using build tags `sqlite_fts5 sqlite_vec`; one new recall test file plus mechanical compilation fixes in several existing test files. |
| **Target Platform** | Linux developer workstation running the current HSME + Ollama environment. |
| **Project Type** | Single Go module with multiple `cmd/` binaries and shared `src/` packages. |
| **Performance Goals** | `recall_recent_session` P50 ≤ 100ms on the current production corpus; no meaningful regression in `search_fuzzy` / `search_exact` after the new index. |
| **Constraints** | No embedder calls; server-side limit cap of 50; `CLAUDE.md` changes restricted to the HSME protocol section; all existing search tools remain behaviorally unchanged. |
| **Scale/Scope** | Small additive feature: one new search file, one DB index, one MCP tool registration, one protocol doc update, and mechanical test repairs. |

## Engineering Alignment

Mission 1 restored real `created_at` values and `project` metadata across the corpus. This mission intentionally takes the smallest safe next step: solve recency queries with a dedicated chronological tool rather than modifying hybrid search ranking. That keeps correctness and quality separate:
- **Mission 2 (this mission)** ships an exact, cheap fast path for "last session" queries.
- **Mission 3** can later tune RRF time-decay using real baselines, without blocking this low-risk gain.

This separation matches the future-mission drafts and avoids mixing a simple retrieval feature with a ranking-core change.

## Charter Check

**SKIPPED** — charter bootstrap reported no `.kittify/charter/charter.md` in this repository. Standard engineering safeguards apply: additive API change only, preserve backward compatibility, keep tests green, and avoid touching non-HSME protocol sections in `CLAUDE.md`.

## Project Structure

### Documentation (this mission)

```
kitty-specs/recency-fast-path-for-session-recall-01KQ405N/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── recall-tool.md
├── checklists/
│   └── requirements.md
└── tasks/                    # produced later by /spec-kitty.tasks
```

### Source Code (repository root)

```
cmd/
└── hsme/
    └── main.go               # register recall_recent_session

src/
├── core/
│   └── search/
│       ├── fuzzy.go          # existing fuzzy/exact search remains behaviorally unchanged
│       ├── graph.go          # unchanged
│       └── recency.go        # NEW chronological lookup function
└── storage/
    └── sqlite/
        └── db.go             # add composite index if missing

tests/
└── modules/
    ├── recall_test.go                # NEW
    ├── search_test.go                # mechanical signature fixes + regression coverage
    ├── indexer_test.go               # mechanical signature fixes
    ├── search_project_filter_test.go # mechanical cleanup/fixes
    ├── migrate_legacy_orphans_test.go
    └── migrate_legacy_restore_test.go

CLAUDE.md                              # HSME protocol section only
```

**Structure Decision**: Keep the feature inside the existing HSME surfaces. The new retrieval primitive lives in `src/core/search/recency.go`, the durable schema support remains in `src/storage/sqlite/db.go`, and the MCP exposure happens at `cmd/hsme/main.go`. No new binary is needed because this is a read-only tool, not a new operational workflow.

## Phase 0: Outline & Research

Research conclusions are already captured in `kitty-specs/recency-fast-path-for-session-recall-01KQ405N/research.md`. The key decisions are:
- use a dedicated `recency.go` file instead of growing `fuzzy.go` further;
- implement `recall_recent_session` as pure SQL with deterministic ordering (`created_at DESC, id DESC`);
- enforce the `limit <= 50` cap server-side, not only in schema docs;
- create the supporting composite index in `src/storage/sqlite/db.go` via `CREATE INDEX IF NOT EXISTS`;
- fold the currently broken test-file fixes into this mission because they are mechanical prerequisites for a green suite;
- update only the HSME protocol subsection of `CLAUDE.md`.

No unresolved clarification markers remain.

## Phase 1: Design & Contracts

The design artifacts for this mission are:
- `kitty-specs/recency-fast-path-for-session-recall-01KQ405N/data-model.md`
- `kitty-specs/recency-fast-path-for-session-recall-01KQ405N/contracts/recall-tool.md`
- `kitty-specs/recency-fast-path-for-session-recall-01KQ405N/quickstart.md`

Together they define:
- the `RecallRecentSession` function signature and SQL contract;
- the MCP tool schema, defaulting, cap semantics, and output shape;
- the lightweight operator/developer validation flow for local verification after implementation.

## Implementation Phases

| Phase | Goal | Key outputs |
|------|------|-------------|
| **Phase 1 — Index & Core Query** | Add the composite index and implement `RecallRecentSession` as a pure SQL path. | `src/storage/sqlite/db.go`, `src/core/search/recency.go` |
| **Phase 2 — MCP Surface** | Register `recall_recent_session` and expose optional `project`/`limit` inputs. | `cmd/hsme/main.go` |
| **Phase 3 — Test Repairs & New Coverage** | Repair broken tests and add explicit recall coverage. | `tests/modules/*.go` |
| **Phase 4 — Protocol Guidance** | Update agent retrieval guidance so recency queries use the new tool first. | `CLAUDE.md` HSME protocol section |

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| New tool accidentally calls embedder | Medium | Keep implementation isolated in `recency.go` with no embedder dependency in the signature or imports. |
| Index introduces startup issues | Low | Use `CREATE INDEX IF NOT EXISTS` inside the existing DB init flow. |
| Existing callers break | Medium | Make the tool additive; do not modify existing MCP tool signatures or response shapes. |
| Test suite remains red due to unrelated signature drift | Medium | Treat the mechanical test-file fixes as part of the mission scope, not out-of-band cleanup. |
| Protocol docs drift from actual tool behavior | Low | Keep `CLAUDE.md` update minimal and mirror the exact MCP tool name and parameters from the contract. |

## Definition of Done

1. `plan.md` is fully materialized with no template placeholders.
2. `research.md`, `data-model.md`, `contracts/recall-tool.md`, and `quickstart.md` consistently describe the same feature.
3. The mission remains strictly additive: new tool + index + docs + test repairs only.
4. The plan is ready for `/spec-kitty.tasks` to decompose into work packages.

## Out of Scope

- Time-decay or any scoring changes inside `search_fuzzy`.
- Pagination beyond the hard cap of 50.
- Enforcing a specific session-summary content format or session ID convention.
- Any changes to `search_exact` / `search_fuzzy` behavior beyond keeping them green under current signatures.

## Branch Contract Reaffirmation

- **Current branch at plan completion:** `main`
- **Planning/base branch:** `main`
- **Final merge target:** `main`
- **Current branch matches intended target:** `true`

## Next Step

Run `/spec-kitty.tasks` for `kitty-specs/recency-fast-path-for-session-recall-01KQ405N/` to generate work packages. Do **not** proceed directly to implementation from `/spec-kitty.plan`.
