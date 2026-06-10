# Data Model: Engram Legacy Cutover & Corpus Restoration

**Mission**: `engram-legacy-cutover-and-corpus-restoration-01KQ2SJK`
**Date**: 2026-04-25

---

## 1. HSME `memories` table — schema after migration

Path: `/home/gary/dev/hsme/data/engram.db`. Schema is mutated by phase 2 of the migrator.

```sql
CREATE TABLE memories (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    raw_content     TEXT,
    content_hash    TEXT NOT NULL UNIQUE,             -- SHA-256(NFC(raw_content)) hex
    source_type     TEXT,                              -- restored values, no longer 'engram_migration'
    project         TEXT,                              -- NEW (phase 2). Indexed. Nullable for memories without a project.
    created_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME DEFAULT CURRENT_TIMESTAMP,
    superseded_by   INTEGER REFERENCES memories(id),
    status          TEXT
);

CREATE INDEX        idx_memories_project       ON memories(project);                   -- NEW
CREATE INDEX        idx_memories_status        ON memories(status);
CREATE INDEX        idx_memories_superseded_by ON memories(superseded_by);
CREATE UNIQUE INDEX idx_memories_active_hash   ON memories(content_hash) WHERE status = 'active';
```

### Field-level migration rules

| Field | Behavior under migration |
|-------|-------------------------|
| `id` | Never modified. Identity preserved. |
| `raw_content` | **Never modified for matched rows.** Wrapper format preserved to keep `content_hash`, chunks, FTS, and embeddings consistent. New ingestions (phase 7) build the wrapper from legacy `(title, project, type, content)`. |
| `content_hash` | Never modified. Recomputing would risk collisions with the active-hash unique index. |
| `source_type` | Phase 3: replaced with `legacy.type`. Phase 4: replaced with literal `'session_summary'`. Phase 7: copied from `legacy.type`. |
| `project` | Phase 3: copied from `legacy.project`. Phase 4: parsed from the wrapper line `Project: ...`. Phase 7: copied from `legacy.project`. |
| `created_at` | Phase 3: replaced with `legacy.created_at`. Phase 4: untouched (born in HSME, the 2026-04-23 18:02:10 timestamp is the truth for these). Phase 7: replaced with `legacy.created_at` via a follow-up UPDATE inside the same transaction. |
| `updated_at` | Bumped to migration time on any modified row (default trigger or explicit SET). |
| `status` | Untouched. |
| `superseded_by` | Untouched. |

### Index integrity

- `idx_memories_active_hash` is a partial unique index on `(content_hash) WHERE status='active'`. Phase 3 does not change `content_hash` or `status`, so the index is unaffected.
- `idx_memories_project` is added in phase 2 and gets populated naturally as phases 3, 4, and 7 set `project`.

---

## 2. Legacy Engram `observations` table — read-only source

Path: `/home/gary/.engram/engram.db`. Opened with `?mode=ro&immutable=1`. NEVER mutated by this mission.

```sql
CREATE TABLE observations (
    id                INTEGER PRIMARY KEY AUTOINCREMENT,
    sync_id           TEXT,
    session_id        TEXT NOT NULL,
    type              TEXT NOT NULL,                   -- maps to HSME source_type
    title             TEXT NOT NULL,
    content           TEXT NOT NULL,                   -- the source of truth for byte-equality match
    tool_name         TEXT,
    project           TEXT,                            -- maps to HSME memories.project
    scope             TEXT NOT NULL DEFAULT 'project',
    topic_key         TEXT,
    normalized_hash   TEXT,                            -- NOT used for matching (algorithm differs)
    revision_count    INTEGER NOT NULL DEFAULT 1,
    duplicate_count   INTEGER NOT NULL DEFAULT 1,
    last_seen_at      TEXT,
    created_at        TEXT NOT NULL,                   -- the source of truth for chronology
    updated_at        TEXT NOT NULL,
    deleted_at        TEXT,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);
```

### Fields used by the migrator

| Field | Used as |
|-------|---------|
| `content` | Match key (byte equality against parsed HSME wrapper payload). |
| `created_at` | Restored to HSME `memories.created_at` for matched rows; assigned to newly-ingested orphans. |
| `type` | Restored to HSME `memories.source_type`. |
| `project` | Restored to HSME `memories.project`. |
| `title` | Used to rebuild the wrapper format for orphan ingestion (phase 7). NOT stored as a separate column in HSME — title lives inside `raw_content`. |
| `id` | Recorded in `mappings.tsv` for audit. |
| `deleted_at` | If non-null, the observation is excluded from match candidates (deleted in legacy). |

### Fields ignored by the migrator

`sync_id`, `session_id`, `tool_name`, `scope`, `topic_key`, `normalized_hash`, `revision_count`, `duplicate_count`, `last_seen_at`, `updated_at`. These are legacy-system bookkeeping with no equivalent in HSME and no impact on this migration.

---

## 3. Wrapper format grammar

The HSME `raw_content` for every migrated memory follows this exact shape:

```
Title: <title>\n
Project: <project>\n
Type: <type>\n
\n
<content>
```

In Go regex form:
```
^Title: (.+?)\nProject: (.+?)\nType: (.+?)\n\n(.*)$    (with DOTALL)
```

### Parser contract

```go
type WrappedMemory struct {
    Title   string
    Project string
    Type    string
    Content string
}

func ParseWrapper(raw string) (*WrappedMemory, error)
```

Returns:
- `(*WrappedMemory, nil)` on success.
- `(nil, ErrUnparseable)` when the format does not match. The migrator uses this to flag malformed rows for the unmatched bucket.

### Verified edge cases (from scoping)

- Empty title with empty content (HSME id 722, `raw_content = "Title: \nProject: Unknown\nType: manual\n\n"`) **fails** the regex because of the trailing `\n\n` followed by nothing — actually it matches with empty groups. Verified: phase 5 deletes by id-or-rule, so this row is handled regardless.
- Multi-line content with embedded blank lines: matched correctly by the DOTALL regex on the final `(.*)`.
- Content with leading whitespace after the blank line: preserved (no trim).

### Wrapper construction (phase 7)

For orphan ingestion, the migrator builds the wrapper from legacy fields:

```go
func BuildWrapper(o *Observation) string {
    return fmt.Sprintf("Title: %s\nProject: %s\nType: %s\n\n%s",
        o.Title, o.Project, o.Type, o.Content)
}
```

This is then passed to `indexer.StoreContext(db, wrapped, o.Type, 0, false)`.

---

## 4. Migration Run Report

Written to `data/migrations/<run_id>/`. Four files, one run.

### `report.json`

```json
{
  "run_id": "20260425T180000Z-full",
  "mode": "full",
  "started_at": "2026-04-25T18:00:00Z",
  "finished_at": "2026-04-25T18:03:42Z",
  "duration_seconds": 222,
  "status": "success",
  "hsme_db": "/home/gary/dev/hsme/data/engram.db",
  "legacy_db": "/home/gary/.engram/engram.db",
  "backup_path": "/home/gary/hsme/backups/engram-20260425T180001Z.db",
  "phases": [
    { "phase": 0, "name": "preflight",         "status": "ok", "duration_ms": 12 },
    { "phase": 1, "name": "backup",            "status": "ok", "duration_ms": 240 },
    { "phase": 2, "name": "schema",            "status": "ok", "applied": true,  "duration_ms": 8 },
    { "phase": 3, "name": "backfill_matched", "status": "ok", "matched": 842, "unmatched": 62, "errored": 0, "duration_ms": 4100 },
    { "phase": 4, "name": "retag_born_in_hsme","status": "ok", "retagged": 62, "duration_ms": 90 },
    { "phase": 5, "name": "delete_garbage",    "status": "ok", "deleted": 1,   "duration_ms": 12 },
    { "phase": 6, "name": "snapshot_legacy",   "status": "ok", "rowcount": 902, "filesize": 7503872, "max_created_at": "2026-04-25 07:09:52", "duration_ms": 5 },
    { "phase": 7, "name": "ingest_orphans",   "status": "ok", "ingested": 59,  "errored": 0, "duration_ms": 132000 }
  ],
  "totals": {
    "matched_and_restored": 842,
    "retagged": 62,
    "deleted": 1,
    "ingested": 59,
    "unmatched": 0,
    "errored": 0
  }
}
```

### `report.txt`

Plain-text mirror of `report.json` for terminal review. Layout: title, table, totals.

### `mappings.tsv`

```
hsme_id<TAB>legacy_id<TAB>action<TAB>created_at_before<TAB>created_at_after<TAB>source_type_before<TAB>source_type_after<TAB>project_before<TAB>project_after
```

One row per affected memory. `legacy_id` is empty for phase-4 retags and phase-5 deletes (no legacy counterpart).

### `unmatched.tsv`

```
hsme_id<TAB>parsed_title<TAB>parsed_project<TAB>parsed_type<TAB>content_length<TAB>reason
```

Reasons: `no_legacy_content_match`, `unparseable_wrapper`, `legacy_row_deleted` (legacy `deleted_at IS NOT NULL`).

---

## 5. Configuration & environment

| Variable | Purpose | Default |
|----------|---------|---------|
| `HSME_DB_PATH` | HSME database path | `/home/gary/dev/hsme/data/engram.db` |
| `LEGACY_DB_PATH` | Legacy engram database path | `/home/gary/.engram/engram.db` |
| `MIGRATIONS_DIR` | Where run reports are written | `<repo>/data/migrations` |
| `MIGRATION_UNMATCHED_THRESHOLD` | Refuse phase 4 if `unmatched/total > threshold` | `0.10` (10%) |
| `BACKUP_HOT_SCRIPT` | Path to backup script | `<repo>/scripts/backup_hot.sh` |
| `OLLAMA_HOST` | Inherited from HSME's existing config | (existing default) |
| `EMBEDDING_MODEL` | Inherited from HSME's existing config | `nomic-embed-text` (existing) |

The migrator validates `EMBEDDING_MODEL` matches the HSME schema's `system_config` row before running phase 7, mirroring the check in `cmd/hsme/main.go`.

---

## 6. State machine — what the migrator can be in

```
[start] --(preflight ok)--> [backed-up]
[backed-up] --(schema applied)--> [schema-current]
[schema-current] --(phase 3)--> [matched-restored]
[matched-restored] --(phase 4)--> [retagged]
[retagged] --(phase 5)--> [garbage-cleared]
[garbage-cleared] --(phase 6)--> [snapshotted]
[snapshotted] --(phase 7)--> [orphans-ingested]
[orphans-ingested] --(report written)--> [done]

Any transition can fail; failure writes the report and exits non-zero.
A re-run starts from [start] but each phase no-ops if its work is already done.
```

The state is implicit in the data, not stored separately. See R-011 for rationale.

---

## 7. Backwards compatibility

- Adding `project` column: backwards-compatible (nullable, no rewrite).
- Restoring `created_at`: backwards-compatible (existing queries that order by `created_at` simply get more accurate ordering).
- Adding optional `project` parameter to `search_fuzzy` / `search_exact`: backwards-compatible (default behavior unchanged).
- Replacing `source_type='engram_migration'` with real types: any code that explicitly filters on `engram_migration` would break. **Searchable check**: `grep -rn "engram_migration\|engram_session_migration" src/ cmd/ tests/` — at planning time no code references these strings; they exist only as DB values.
