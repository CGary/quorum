# Specification: Engram Legacy Cutover & Corpus Restoration

**Mission ID**: 01KQ2SJK44AP2YDKXSBKPZCB8Q
**Mission slug**: engram-legacy-cutover-and-corpus-restoration-01KQ2SJK
**Mission type**: software-dev
**Target branch**: main
**Created**: 2026-04-25

---

## Purpose (Stakeholder Summary)

**TL;DR**: Restore lost chronology and project metadata for migrated memories, ingest the 59 post-migration writes still living in legacy Engram, normalize the corpus, and cut Claude Code's dual-write so HSME becomes the single source of truth.

**Context**: When legacy Engram memories were migrated to HSME via an LLM-driven prompt, the wrapper format preserved content but flattened every `created_at` to the migration moment and dropped `project`, `title`, and `type` metadata into a generic `source_type='engram_migration'`. Verification confirmed 842 of the 905 migrated rows still match the legacy source byte-for-byte, 62 are fabricated session summaries born during the migration session, and 1 is a malformed empty row. Meanwhile the legacy Engram database keeps receiving writes from Claude Code (30 new observations on the day this mission was scoped), so the gap widens by the hour. Without this restoration, every recency-aware feature planned downstream is built on broken timestamps.

---

## User Scenarios & Testing

### Primary scenario — Recency-aware recall by project

**Actor**: A coding agent operating against HSME on behalf of the user.
**Trigger**: The agent asks "what was the last session for the `aibbe` project?".
**Today (broken)**: Search returns migrated memories that all share `created_at = 2026-04-23 ~18:02` regardless of their real age, mixed with results from other projects. The agent cannot tell which session is actually the latest, and irrelevant projects pollute the result.
**After this mission (success)**: Search returns the most recent real session summary for `aibbe`, ordered by the legacy `created_at`. Memories from other projects do not appear when the project filter is applied.

### Primary scenario — Single source of truth for new writes

**Actor**: The user, working through Claude Code.
**Trigger**: The user asks the agent to remember a decision.
**Today (broken)**: The agent calls `mem_save`, which writes to `/home/gary/.engram/engram.db` — a database HSME does not own. HSME never sees the write. The corpus drifts further from the source.
**After this mission (success)**: The MCP configuration of Claude Code lists only HSME. New writes land in `/home/gary/dev/hsme/data/engram.db` directly. The legacy file receives zero new writes from this client.

### Exception scenario — Migration row without a legacy match

**Trigger**: The backfill scans an HSME row whose parsed content does not byte-equal any legacy observation.
**Outcome**: The row is left unchanged and recorded in a per-run report under `Unmatched`. The mission does not silently invent metadata for it. The 62 known fabricated session summaries are pre-classified and re-tagged separately, not lumped into this exception path.

### Exception scenario — Re-running the backfill

**Trigger**: An operator re-runs the migration script after a partial run, a backup restore, or a transient failure.
**Outcome**: The script detects rows already restored (e.g., `source_type` no longer matches `engram_migration`) and skips them. No row is ever updated twice with conflicting values.

### Edge cases

- A legacy observation that exists in HSME and has been mutated in Engram after the original migration (revision_count > 1). The freshest legacy version wins; the older HSME content is superseded, not overwritten in place.
- A write reaches legacy Engram during the cutover window between MCP reconfig and the first delta-ingest. A final delta-ingest pass after cutover catches it.
- The user reopens Claude Code mid-mission. The MCP reconfiguration is the LAST step in the cutover phase, so an interrupted session never sees a half-broken config.
- The HSME database is being read by the MCP server while the script runs. SQLite WAL mode allows concurrent reads, so search continues to function during the migration.

---

## Domain Language

| Term | Canonical meaning | Synonyms to avoid |
|------|-------------------|-------------------|
| Legacy Engram | The predecessor SQLite database at `/home/gary/.engram/engram.db`, holding 902 observations as of 2026-04-25. | "Engram", "old DB", "source DB" — ambiguous |
| HSME | The Hybrid Semantic Memory Engine MCP server and its DB at `/home/gary/dev/hsme/data/engram.db`. | "the engine", "memory server" — ambiguous |
| Migrated memory | An HSME memory whose `source_type ∈ {engram_migration, engram_session_migration}`. | "imported memory", "migrated row" — accept |
| Wrapper format | The textual layout `"Title: {t}\nProject: {p}\nType: {ty}\n\n{content}"` used by the original migration prompt to wrap each legacy observation into HSME's `raw_content`. | "preamble" — accept |
| Matched memory | A migrated memory whose parsed content (after stripping the wrapper) is byte-identical to the `content` of some legacy observation. | — |
| Born-in-HSME memory | A migrated memory with no equivalent in legacy Engram. The 62 known cases are session summaries fabricated by the migration agent. | — |
| Orphan observation | A legacy observation that has no equivalent in HSME (typically a write to legacy Engram that occurred after the original migration). | — |
| Cutover | The act of removing legacy Engram from Claude Code's MCP configuration so HSME becomes the only memory backend that client writes to. | — |

---

## Functional Requirements

| ID | Requirement | Status |
|----|-------------|--------|
| FR-001 | The `memories` table SHALL gain a nullable `project` column of type TEXT, indexed for query performance. Existing rows without a known project remain NULL until backfilled. | Drafted |
| FR-002 | For each of the 842 matched migrated memories, the system SHALL update `created_at`, `source_type`, and `project` in place using the legacy observation's values. The `raw_content` SHALL remain unchanged so existing chunks, FTS index, and vector embeddings stay valid. | Drafted |
| FR-003 | The 62 born-in-HSME session summaries SHALL have their `source_type` updated from `engram_session_migration` to `session_summary`. Their `project` SHALL be backfilled from the wrapper format already present in `raw_content`. Their `created_at` SHALL remain at its current value (2026-04-23 18:02:10). | Drafted |
| FR-004 | The malformed empty memory at HSME id 722 (title empty, content empty, length 39) SHALL be deleted, including its dependent chunks and FTS rows via existing cascade triggers. | Drafted |
| FR-005 | The 59 legacy observations that have no equivalent in HSME SHALL be ingested through the existing `indexer.StoreContext` path so they receive proper chunking, FTS indexing, and graph extraction tasks, with `created_at`, `source_type`, and `project` from the legacy row. | Drafted |
| FR-006 | The `search_fuzzy` and `search_exact` MCP tools SHALL accept an optional `project` parameter. When supplied, results are restricted to memories with that project. When omitted, behavior is unchanged. | Drafted |
| FR-007 | Claude Code's MCP configuration SHALL be reconfigured so the legacy `engram` server is removed and `hsme` is the only memory backend registered for that client. | Drafted |
| FR-008 | The system SHALL produce an automated hot backup of HSME's database before any mutating step, using the existing `scripts/backup_hot.sh` mechanism. | Drafted |
| FR-009 | The migration SHALL be idempotent: re-running the script after a successful or partial run SHALL leave the database in the same final state without conflicts or double-updates. | Drafted |
| FR-010 | The migration SHALL produce a structured run report listing counts by phase (matched-and-restored, retagged, deleted, ingested, unmatched, errored) and the legacy ↔ HSME ID mappings for the matched set. | Drafted |
| FR-011 | A final delta-ingest pass SHALL run AFTER the MCP cutover to catch any race-window writes that landed in legacy Engram during the cutover transition. | Drafted |
| FR-012 | The legacy database file at `/home/gary/.engram/engram.db` SHALL NEVER be modified by this mission. The mission only reads from it. | Drafted |

---

## Non-Functional Requirements

| ID | Requirement | Threshold | Status |
|----|-------------|-----------|--------|
| NFR-001 | Match rate of migrated memories against legacy observations | ≥ 93% (matches the measured baseline of 842/905; lower triggers human review) | Drafted |
| NFR-002 | Backfill end-to-end runtime on production-sized corpus | ≤ 10 minutes for ≤ 1500 memories on the developer's machine | Drafted |
| NFR-003 | Ingestion of the 59 orphan observations | All 59 reach `status='active'` and are recoverable via search within 10 minutes of script completion | Drafted |
| NFR-004 | Search latency (P50) for `search_fuzzy` and `search_exact` after the schema change | Within ±10% of the pre-mission baseline measured against the same query set | Drafted |
| NFR-005 | Post-cutover write isolation: legacy Engram receives zero writes from any user-controlled client | Zero new rows in `/home/gary/.engram/engram.db:observations` over a 24-hour observation window after cutover | Drafted |
| NFR-006 | Backfill script run report | Machine-readable (JSON) and human-readable (text summary) outputs, both written to disk under `data/migrations/` for audit | Drafted |

---

## Constraints

| ID | Constraint | Status |
|----|------------|--------|
| C-001 | The mission MUST take a fresh hot backup of HSME's database before any mutating action and SHALL refuse to proceed if the backup step fails. | Drafted |
| C-002 | Mutations on the matched 842 memories SHALL run inside a single SQLite transaction OR with a per-row checkpoint, so a partial run can be resumed without leaving the corpus in a half-restored state. | Drafted |
| C-003 | The 62 born-in-HSME session summaries SHALL be classified explicitly by their known IDs (or by the verified rule: `source_type='engram_session_migration'` AND no legacy match), never lumped into the unmatched bucket. | Drafted |
| C-004 | The legacy database at `/home/gary/.engram/engram.db` SHALL be opened in read-only mode by every script in this mission. | Drafted |
| C-005 | The `raw_content` column of any matched memory SHALL NOT be rewritten during backfill — only the metadata columns (`created_at`, `source_type`, `project`). This keeps the existing `content_hash`, `memory_chunks`, and `memory_chunks_fts` rows valid without re-indexing. | Drafted |
| C-006 | The schema migration adding the `project` column SHALL be backwards-compatible: existing queries that do not reference `project` continue to work unchanged. | Drafted |
| C-007 | The migration report SHALL be persisted on disk before the script exits, even on failure, so the operator can diagnose and resume. | Drafted |

---

## Success Criteria

1. After mission completion, **100% of the 842 matched memories** carry their legacy `created_at`, `source_type`, and `project` values, replacing the migration-flattened metadata.
2. **0 memories** retain `source_type='engram_migration'` once their legacy match has been resolved.
3. The 59 orphan legacy observations are searchable in HSME within 10 minutes of script completion.
4. The malformed empty memory (HSME id 722) no longer exists in any HSME table.
5. The 62 born-in-HSME session summaries carry `source_type='session_summary'` and a non-NULL `project`.
6. Searching `search_fuzzy("session summary", project="aibbe")` returns only `aibbe` results, ordered such that the most recent legacy session summary appears within the top 5.
7. The Claude Code MCP configuration shows `hsme` registered and `engram` absent.
8. Over a 24-hour observation window starting at cutover, the legacy file `/home/gary/.engram/engram.db` receives **0 new rows** from any user-controlled client.
9. The migration report (JSON + text) is present under `data/migrations/` and accounts for every one of the 905 migrated rows + 59 orphans + 1 garbage row.
10. Re-running the migration script ten minutes after a successful run produces a report with `matched-and-restored: 0`, `ingested: 0`, `errored: 0` — confirming idempotency.

---

## Key Entities

### Memory (HSME)
- `id` (INT, PK)
- `raw_content` (TEXT) — wrapped form for migrated rows; bare content for new ingestions
- `content_hash` (TEXT, unique active) — SHA-256 of NFC-normalized `raw_content`
- `source_type` (TEXT) — replaces the temporary `engram_migration` / `engram_session_migration` values
- `project` (TEXT, **NEW**, indexed)
- `created_at` (DATETIME) — restored from legacy for matched rows; preserved for born-in-HSME and new orphan ingestions
- `updated_at`, `status`, `superseded_by` — unchanged

### Observation (Legacy Engram, read-only)
- `id`, `type`, `project`, `scope`, `title`, `content`
- `normalized_hash` — not used for matching (algorithm differs from HSME's `content_hash`)
- `created_at`, `updated_at`, `revision_count`
- Used as the authoritative source for `created_at`, `source_type` (= `type`), and `project` of matched HSME memories.

### Migration Run Report (new, written to disk)
- `run_id`, `started_at`, `finished_at`, `status`
- `counts`: `matched-and-restored`, `retagged`, `deleted`, `ingested`, `unmatched`, `errored`
- `mappings`: list of `{hsme_id, legacy_id, action}` for audit
- `unmatched`: list of `{hsme_id, reason}` for manual review

---

## Assumptions

1. The 93% match rate measured on the 2026-04-25 snapshot will hold or improve at execution time. Fewer dual-writes after cutover means the match base only grows in HSME's favor.
2. The Claude Code instance is the sole remaining writer to legacy Engram. If telemetry during the 24-hour observation window contradicts this, a follow-up mission is required to identify and reconfigure the other writer.
3. The legacy Engram database schema does not change during this mission. The mission reads `observations` with the schema verified at scoping time.
4. The wrapper format `"Title: ...\nProject: ...\nType: ...\n\n{content}"` is consistent across all migrated rows, with the only known exception being HSME id 722 (empty title, empty content). This was verified against the full 905-row corpus during scoping.
5. SQLite WAL mode allows the MCP server to keep serving reads while the migration runs. Writes from the MCP server during the migration window are unlikely (the MCP is in read-mostly use during agent search), but the script holds its mutations to the smallest possible windows to minimize contention.

---

## Out of Scope (Deferred to Follow-up Missions)

- **Recency feature surface** (`recall_recent_session` MCP tool, session-tag convention, RRF time-decay): planned as Mission 2 once this mission ships and the corpus has real timestamps.
- **Knowledge graph janitor** (entity merging, dirty node pruning, conflict detection): tracked separately in `ideas/graph-cleanup-maintenance.md`.
- **Decommissioning the legacy Engram database file**: this mission cuts off WRITES to it, but the file itself remains on disk as a historical artifact. Deletion is a separate manual operator decision.
- **Re-ingesting legacy data with fresh embeddings under a newer embedding model**: the 842 matched rows keep their existing chunks and embeddings to preserve search continuity. A re-embed mission can run later if the model is upgraded.
- **CLI tool `hsme-cli`**: separate idea, separate mission.
