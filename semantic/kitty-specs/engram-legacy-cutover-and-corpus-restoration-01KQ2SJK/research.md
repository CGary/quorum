# Phase 0 Research: Engram Legacy Cutover & Corpus Restoration

**Mission**: `engram-legacy-cutover-and-corpus-restoration-01KQ2SJK`
**Date**: 2026-04-25

This document captures decisions made during planning, the rationale for each, and the alternatives considered. Every decision below was driven either by measurements taken during scoping or by explicit constraints in the spec.

---

## R-001: Match strategy — content equality, not hash equality

**Decision**: Match HSME's migrated memories to legacy observations by extracting the wrapped `raw_content` payload and comparing it byte-for-byte to the legacy `content` column.

**Rationale**:
During scoping we measured 0/20 matches by hash (HSME `content_hash` ↔ legacy `normalized_hash`). Both are SHA-256 hex of 64 chars but the inputs differ: HSME hashes the **wrapped** form `"Title: ...\nProject: ...\nType: ...\n\n{content}"`, while legacy hashes the bare `content`. Recomputing one to align with the other was tested but a simpler insight emerged: parsing the HSME wrapper and matching `parsed.content == legacy.content` directly produces a 93% (842/905) exact match. The remaining 62 are session summaries fabricated by the LLM that ran the original migration; they have no legacy equivalent by design.

**Alternatives considered**:
- *Recompute legacy hashes with HSME's algorithm.* Possible, but it requires reading every legacy row, building the wrapper string the same way the migration did, and computing SHA-256. More work for the same result. Rejected.
- *Fuzzy match (FTS-based, similarity scoring).* Useful only when content was rewritten. We measured: rewriting did not happen for 842/905 rows. The 62 fabricated rows are not findable by fuzzy match either (FTS phrase lookup returned 0/62 hits). Rejected as unnecessary.

---

## R-002: Schema migration via `ALTER TABLE ADD COLUMN`

**Decision**: Add the `project` column with a single `ALTER TABLE memories ADD COLUMN project TEXT;` followed by `CREATE INDEX idx_memories_project ON memories(project);`. Wrap both statements in a single transaction. Make the migration idempotent by checking `pragma_table_info('memories')` for the column before applying.

**Rationale**:
SQLite's `ADD COLUMN` is O(1) — it does not rewrite the table. Existing queries that don't reference `project` continue to work because the column is nullable. The index is created once; subsequent runs detect the column already exists and skip. This is the boring, correct path.

**Alternatives considered**:
- *Recreate the table with the new column.* Required for breaking changes, but unnecessary here and dangerous (cascade DELETE triggers the FTS table). Rejected.
- *Versioned migrations directory.* Overkill for a single one-shot column addition. The repo has no migrations framework today; introducing one here is scope creep.

---

## R-003: One transaction per phase, not one big transaction

**Decision**: Each of phases 2–5 runs in its own SQLite transaction. Phase 7 runs without a wrapping transaction because `StoreContext` already manages its own transactions.

**Rationale**:
- **Atomicity per logical unit**: Phase 3 is "all 842 backfills succeed or none", phase 4 is "all 62 retags succeed or none", etc. A single mega-transaction would either all-succeed or all-roll-back, including the schema change, which is harder to reason about on resume.
- **Simpler idempotency**: Each phase queries for its remaining work at start. Re-running after phase 3 succeeded but phase 4 failed: phase 3 finds 0 rows to update (all already restored), phase 4 retries.
- **Faster recovery**: If phase 7 (orphan ingestion, slowest) hits an Ollama timeout on row 30 of 59, the prior 29 are already committed. Restart resumes at row 30.

**Alternatives considered**:
- *Single big transaction.* Atomic, but slow recovery and ambiguous resume semantics. Rejected.
- *Per-row transactions.* Maximum granularity but pathological performance (842 separate fsyncs). Rejected.

---

## R-004: Never rewrite `raw_content` of matched rows

**Decision**: Phase 3 only updates metadata columns (`created_at`, `source_type`, `project`). The `raw_content` column remains unchanged. The wrapper format stays as it is.

**Rationale**:
- `content_hash` is computed from `raw_content`. Changing `raw_content` would invalidate the hash and risk dedup collisions.
- `memory_chunks` rows reference the chunked text of `raw_content`. The FTS index `memory_chunks_fts` is populated from those chunks. The vector embeddings (768-dim) are computed on those same chunks. Rewriting `raw_content` would force re-chunking, re-FTS-indexing, and re-embedding for 842 memories — minutes of Ollama work and a lot of risk for zero functional gain.
- Search results already include the wrapper text. Users see "Title: ...\nProject: ...\nType: ...\n\n..." today and that's actually useful context. No reason to strip it.

**Alternatives considered**:
- *Strip the wrapper from `raw_content` and store fields properly.* Cleaner long-term but a heavyweight change. Out of scope. If we ever do it, it goes in a separate "schema normalization" mission.

---

## R-005: Orphan ingestion via `indexer.StoreContext`, not raw INSERT

**Decision**: Phase 7 calls `indexer.StoreContext(db, content, sourceType, supersedesID, forceReingest)` for each of the 59 orphans, with the legacy `created_at` injected post-call via UPDATE.

**Rationale**:
`StoreContext` does the work that the original prompt-driven migration skipped: chunking, FTS5 trigger firing (memories show up in lexical search immediately), and async task enqueue (graph extraction runs eventually). A raw INSERT into `memories` would leave the row invisible to `search_fuzzy` and `search_exact` until the user runs a manual reindex. We do NOT want to repeat the original migration's mistake.

The `created_at` is overridden via a follow-up UPDATE inside the same transaction. `StoreContext` doesn't support a `created_at` parameter today, and adding one is a wider refactor than this mission needs.

**Alternatives considered**:
- *Add a `createdAt` parameter to `StoreContext`.* Cleaner, but expands the surface of an existing function used elsewhere. Deferred.
- *Direct INSERT + manual chunk/FTS population.* Recreates the original migration bug. Rejected.

---

## R-006: Cutover is fail-loud (Option A), not zero-downtime

**Decision**: The cutover sequence is:
1. Phase 6 captures the pre-cutover snapshot (`MAX(created_at)`, `COUNT(*)`, filesize).
2. Phase 7 ingests orphans up to the snapshot.
3. **Operator manually runs `claude mcp remove engram` and reloads Claude Code.**
4. Operator runs `cmd/migrate-legacy --mode=delta` to ingest any race-window writes.
5. Operator runs `scripts/verify_cutover.sh` at T0 and again at T+24h.

If the user attempts a memory write between step 3 and step 4, Claude Code's MCP fails (engram is gone). The user sees a loud error instead of a silent loss.

**Rationale**:
The user explicitly chose this option during planning interrogation. The justification holds: Option B (delta first, cutover after) creates a window where writes can land in legacy and never make it to HSME if the user doesn't re-run the migrator. Silent data loss is worse than a few seconds of "memory unavailable". Option C (dual-write proxy) is more code than the rest of this mission combined for a one-shot single-user migration.

**Alternatives considered**: Spelled out in the planning interrogation. The user picked A.

---

## R-007: MCP reconfiguration stays manual

**Decision**: The migrator does NOT call `claude mcp remove engram`. The runbook in `quickstart.md` instructs the operator to run it.

**Rationale**:
Modifying the user's Claude Code configuration from a Go binary owned by HSME is the wrong layering. `claude mcp` is a tool of Claude Code, not of HSME. Automating it requires assumptions about:
- Where the user's Claude Code config lives (varies by OS, install method).
- Whether the user has multiple MCP profiles.
- Whether the user actually wants engram removed at this exact moment (might be running other work).

Manual execution with confirmation matches the "executing actions with care" principle in the user's global `CLAUDE.md`. The operator runs one short command; the boundary stays clean.

**Alternatives considered**:
- *Migrator detects Claude Code, edits config, restarts.* Higher blast radius, OS-specific. Rejected.
- *Migrator prints the exact command to run.* Already covered by `quickstart.md` — the runbook spells out `claude mcp remove engram`. Acceptable as a complement, not a replacement.

---

## R-008: Telemetry for NFR-005 — filesize + rowcount + max(created_at) snapshot

**Decision**: `scripts/verify_cutover.sh` snapshots three values from `/home/gary/.engram/engram.db`:
1. File size of `engram.db` (bytes).
2. `COUNT(*) FROM observations`.
3. `MAX(created_at) FROM observations`.

Output is one TSV line: `<timestamp>\t<filesize>\t<rowcount>\t<max_created_at>`. The operator runs it twice — at cutover (T0) and 24 hours later (T+24h) — and diffs the two lines manually.

**Rationale**:
Three signals catch all realistic write patterns:
- Filesize delta catches WAL activity even if rows were later compacted.
- Rowcount delta catches actual row inserts.
- Max `created_at` delta catches row updates that bumped a timestamp.

If any of the three changed in the 24h window, NFR-005 is violated and we know a writer wasn't fully cut off. Implementation is ~10 lines of bash. No daemon, no cron, no extra dependency.

**Alternatives considered**:
- *Cron job that polls every 5 minutes.* Overkill — a single 24h check is enough for this mission, and continuous monitoring is a feature for a later "ops dashboard" mission, not this one.
- *Filesystem `inotify` watcher.* Detects every write but requires running a process for 24h. Fragile on a developer machine that gets rebooted.

---

## R-009: Run report format — JSON + text + TSV mappings

**Decision**: Each migrator run produces four files under `data/migrations/<run_id>/`:
- `report.json`: machine-readable summary with phase counts, durations, errors.
- `report.txt`: human-readable summary for terminal review.
- `mappings.tsv`: one row per `{hsme_id, legacy_id, action}` triple — the audit trail.
- `unmatched.tsv`: HSME rows that didn't match a legacy observation, with reasons.

`<run_id>` is `<timestamp>-<mode>` (e.g., `20260425T180000Z-full`).

**Rationale**:
- JSON for any downstream automation (e.g., the verify script can parse it).
- TXT for terminal review by the operator without `jq`.
- TSV for mappings because they're tabular by nature and grep/awk-friendly.
- One run produces one directory: easy to archive, easy to delete, never overwrites a previous run.

`data/migrations/` goes into `.gitignore` — these are operational artifacts, not source.

**Alternatives considered**:
- *Single JSON file.* Less convenient for scanning mappings without tooling. Rejected.
- *SQLite table for run history.* Overkill for what is fundamentally a one-shot operation.

---

## R-010: `project` filter behavior on `search_fuzzy` / `search_exact`

**Decision**: Both tools accept an optional `project` parameter (string, default empty/missing). When supplied, the SQL `WHERE` clause adds `AND m.project = ?`. When omitted or empty, behavior is byte-identical to today.

**Rationale**:
- Backwards compatibility: existing callers (other agents, scripts, MCP clients) keep working without changes.
- Filter is applied at the `memories` table level, before chunk-level RRF, so the filter cannot accidentally let a chunk from a different project leak in via aggregation.
- For `search_exact` (which uses FTS5 MATCH today), the filter is applied via JOIN on `memories.id` and `WHERE memories.project = ?`. Cheap, indexed.

**Alternatives considered**:
- *Make `project` mandatory.* Would force every existing caller to pass an empty string or a value. Breaks backwards compatibility for no benefit.
- *Multi-project filter (`projects: ["a", "b"]`).* Unnecessary today — the agent calls one project at a time. Add later if a use case appears.

---

## R-011: Idempotency strategy

**Decision**: Each phase queries for its remaining work at the start. If the query returns 0 rows, the phase no-ops and reports `0` in the run report. No locks, no checkpoints, no separate state table.

Concretely:
- Phase 2 (schema): `pragma_table_info('memories')` — does `project` column exist? If yes, skip.
- Phase 3 (backfill): `SELECT COUNT(*) FROM memories WHERE source_type IN ('engram_migration','engram_session_migration')` — if 0, skip.
- Phase 4 (retag): same query restricted to `engram_session_migration` after phase 3.
- Phase 5 (delete): `SELECT COUNT(*) FROM memories WHERE id = 722 OR <garbage rule>` — if 0, skip.
- Phase 7 (orphans): `SELECT * FROM legacy.observations WHERE content NOT IN (...)` minus already-ingested. The negative anti-join is the source of truth.

**Rationale**:
The state of the work is the data itself. A separate state table would just be a denormalized copy. Re-running the migrator is always safe.

**Alternatives considered**:
- *State table `migration_runs`.* More machinery. Doesn't add value for a single-machine, single-user migration. Rejected.

---

## Open Items — None

All planning questions resolved. No `[NEEDS CLARIFICATION]` markers remain. Phase 1 (data-model + contracts + quickstart) can proceed.
