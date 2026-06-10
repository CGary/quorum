# Contract: `cmd/migrate-legacy` CLI

**Mission**: `engram-legacy-cutover-and-corpus-restoration-01KQ2SJK`

The `migrate-legacy` binary is the operational entry point for the migration. It is invoked by the operator (the user) from the repository root.

---

## Synopsis

```
migrate-legacy [--mode <full|delta|dry-run>] [--unmatched-threshold <float>] [flags]
```

## Flags

| Flag | Type | Default | Purpose |
|------|------|---------|---------|
| `--mode` | enum | `full` | Execution mode (see below) |
| `--hsme-db` | path | `$HSME_DB_PATH` or `/home/gary/dev/hsme/data/engram.db` | HSME SQLite database (read-write) |
| `--legacy-db` | path | `$LEGACY_DB_PATH` or `/home/gary/.engram/engram.db` | Legacy Engram SQLite database (read-only) |
| `--migrations-dir` | path | `<repo>/data/migrations` | Where run reports are written |
| `--unmatched-threshold` | float | `0.10` | Refuse phase 4 if unmatched ratio > threshold |
| `--skip-backup` | bool | `false` | DANGEROUS — skip phase 1 (only for testing with temp DBs) |
| `--ollama-host` | string | `$OLLAMA_HOST` or default | Ollama endpoint (used by phase 7) |
| `--embedding-model` | string | `$EMBEDDING_MODEL` or `nomic-embed-text` | Must match HSME's `system_config` |
| `--quiet` | bool | `false` | Suppress per-row progress output |
| `--help` | bool | — | Print usage and exit 0 |

## Modes

### `--mode=full` (default)

Runs phases 0 through 7 in strict order. The expected one-shot mode for the initial cutover.

Pre-condition: HSME is in its post-prompt-migration state (905 rows with `source_type='engram_migration'`).
Post-condition: 842 rows restored, 62 retagged, 1 deleted, 59 ingested. Run report written.

### `--mode=delta`

Runs phases 0 (preflight), 6 (snapshot legacy), and 7 (ingest orphans WHERE `created_at > pre_snapshot_max`). Used for the post-cutover delta-ingest in the runbook.

Pre-condition: a previous `full` run completed. Phase-7's negative anti-join key is the previous run's `max_created_at` (read from the prior run's `report.json`).
Post-condition: any race-window writes are now in HSME.

### `--mode=dry-run`

Runs phases 0–7 in shadow mode: matcher computes match counts, schema checks are read-only, no writes to HSME, no backup taken. Run report is written under `data/migrations/<run_id>-dryrun/` for review. Used for pre-flight validation before the real run.

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Success — all phases completed and report written |
| `1` | Usage error (bad flag, missing flag, conflicting flags) |
| `2` | Preflight failure (DB unreachable, schema mismatch, embedding model mismatch) |
| `3` | Backup failure |
| `4` | Phase failure during mutation (rolled back, report written, partial state possible only across-phase) |
| `5` | Unmatched threshold exceeded — operator must review and re-run |
| `6` | Embedding/Ollama error during phase 7 (some orphans may be ingested, others not — re-run picks them up) |
| `7` | Idempotency conflict (e.g., `project` column exists with different definition than expected) |

## Stdout / stderr conventions

- Stdout: one line per phase, `[<phase-name>] <status> <key=value...>`. Example:
  ```
  [preflight] ok
  [backup] ok path=/home/gary/hsme/backups/engram-20260425T180001Z.db size=11448320
  [schema] ok applied=true
  [backfill_matched] ok matched=842 unmatched=62 errored=0
  [retag_born_in_hsme] ok retagged=62
  [delete_garbage] ok deleted=1
  [snapshot_legacy] ok rowcount=902 max_created_at='2026-04-25 07:09:52'
  [ingest_orphans] ok ingested=59 errored=0
  [done] report=/home/gary/dev/hsme/data/migrations/20260425T180000Z-full/
  ```
- Stderr: only on errors and warnings. Format: `[<level>] <phase> <message>`.
- `--quiet` suppresses stdout phase headers but still emits the final `[done]` line.

## Idempotency contract

Re-running `--mode=full` after a successful run MUST:
- Produce a new `<run_id>` directory.
- Have all phase counts at zero except `phase 0` and `phase 1` and `phase 6` (which always run).
- Exit 0.

Re-running `--mode=full` after a partial failure MUST:
- Resume from the failed phase.
- Not double-apply any change.
- Either complete successfully or fail with the same actionable error.

## Concurrency contract

- The migrator opens HSME with `journal_mode=WAL` (already the default per `src/storage/sqlite/db.go`).
- The MCP server may be running concurrently. The migrator's writes use SQLite's BEGIN IMMEDIATE to grab a write lock per phase; if the MCP holds the write lock, the migrator retries with exponential backoff up to 3 times before failing with exit 4.
- The legacy DB is opened read-only and `immutable=1`. Concurrent writes by other processes don't affect the migrator (the snapshot semantics of `immutable=1`).

## Examples

### First-time cutover (the expected use)

```bash
# 1. Dry run to validate
migrate-legacy --mode=dry-run

# 2. Real run
migrate-legacy --mode=full

# 3. Operator runs:    claude mcp remove engram
# 4. Operator reloads Claude Code

# 5. Catch race window
migrate-legacy --mode=delta

# 6. Telemetry baseline
scripts/verify_cutover.sh > data/migrations/cutover-T0.tsv

# (24 hours later)
scripts/verify_cutover.sh > data/migrations/cutover-T24h.tsv
diff data/migrations/cutover-T0.tsv data/migrations/cutover-T24h.tsv
```

### Resume after a failure

```bash
# Initial run failed at phase 7 due to Ollama being down
migrate-legacy --mode=full
# ... [ingest_orphans] failed errored=12 ...

# Fix Ollama, re-run. Phases 2-6 no-op; phase 7 picks up the remaining 12.
migrate-legacy --mode=full
```

### Custom DB paths (testing)

```bash
migrate-legacy \
  --hsme-db /tmp/test-hsme.db \
  --legacy-db /tmp/test-legacy.db \
  --migrations-dir /tmp/migration-out \
  --skip-backup \
  --mode=full
```
