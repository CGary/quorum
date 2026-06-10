# Operator Runbook: Engram Legacy Cutover & Corpus Restoration

**Mission**: `engram-legacy-cutover-and-corpus-restoration-01KQ2SJK`
**Audience**: the operator (the user) running this once on their machine.

This runbook walks the cutover from start to finish, including the 24-hour verification window. Every step is idempotent — if you stop halfway, you can resume.

---

## Pre-conditions to verify

Before starting, confirm:

- [ ] `git status` is clean (no uncommitted changes).
- [ ] HSME's MCP server is running OR is not actively writing (the migrator can run with the MCP up; just no concurrent writes please).
- [ ] Worker (`just work-bg`) is running so async tasks (graph extraction) drain after orphan ingestion.
- [ ] `ollama ps` shows GPU active (per memory: `100% GPU`, ~1.8s/task).
- [ ] Disk has at least 50 MB free for the hot backup.
- [ ] You're logged into Claude Code in a state where you can run `claude mcp` from your shell.

## Step 1 — Build the migrator

```bash
just migrator-build      # NEW just target added by this mission
# or directly:
go build -tags "sqlite_fts5 sqlite_vec" -o migrate-legacy ./cmd/migrate-legacy
```

Expected: the `migrate-legacy` binary appears at the repo root or in `$GOPATH/bin` per the `just` target's install rule.

## Step 2 — Dry run

```bash
./migrate-legacy --mode=dry-run
```

Expected stdout:
```
[preflight] ok
[backup] skipped reason=dry-run
[schema] would-apply applied=false
[backfill_matched] preview matched=842 unmatched=62 errored=0
[retag_born_in_hsme] preview retagged=62
[delete_garbage] preview deleted=1
[snapshot_legacy] ok rowcount=902 max_created_at='2026-04-25 07:09:52'
[ingest_orphans] preview ingested=59 errored=0
[done] report=data/migrations/<run_id>-dryrun/
```

If any count differs significantly from what was measured during scoping (842 / 62 / 1 / 59), STOP and investigate. The numbers may have drifted because the legacy DB is still receiving writes.

Review `data/migrations/<run_id>-dryrun/report.txt` and `unmatched.tsv`.

## Step 3 — Real run

```bash
./migrate-legacy --mode=full
```

Expected: same stdout shape but with real counts and `applied=true`. Total runtime ~3–5 minutes (most of it is phase 7 calling Ollama for orphan embeddings).

If `--unmatched-threshold` is exceeded (>10%), the binary exits 5 and writes the report. Re-run with `--unmatched-threshold` adjusted, or investigate why the threshold drifted.

After success, sanity-check the corpus:

```bash
sqlite3 data/engram.db "SELECT source_type, COUNT(*) FROM memories GROUP BY source_type;"
# Expected: 0 rows for engram_migration, 0 rows for engram_session_migration.
# session_summary count = 62 + (existing session_summary count from legacy).

sqlite3 data/engram.db "SELECT MIN(created_at), MAX(created_at) FROM memories;"
# Expected: MIN goes back to 2026-04-03 (or earlier if legacy has it), MAX is recent.

sqlite3 data/engram.db "SELECT COUNT(*) FROM memories WHERE project IS NULL;"
# Expected: very low (only memories that genuinely have no project — should be near zero after migration).
```

## Step 4 — Cutover (manual, fail-loud)

This is the irreversible step. Do it deliberately.

```bash
# 4a. Confirm engram is currently registered
claude mcp list | grep engram

# 4b. Remove engram from Claude Code's MCP config
claude mcp remove engram

# 4c. Reload Claude Code so the change takes effect
# (close and reopen the Claude Code window/session, or run `claude` fresh)
```

**From this moment on, any write to legacy Engram from Claude Code will fail loudly** (the engram MCP no longer exists in this client). That's expected.

If you need to verify the MCP config:
```bash
claude mcp list
# engram should be absent. hsme should be present.
```

## Step 5 — Delta-ingest

Capture any race-window writes that landed in legacy between Step 3 and Step 4.

```bash
./migrate-legacy --mode=delta
```

Expected stdout:
```
[preflight] ok
[snapshot_legacy] ok rowcount=<N> max_created_at='<latest>'
[ingest_orphans] ok ingested=<N> errored=0
[done] report=data/migrations/<run_id>-delta/
```

Most likely `ingested=0` if you moved through Steps 3–4 quickly. Anything ≤5 is normal. Anything >20 means there was a slow operator or a very busy session — still fine, the data is now in HSME.

## Step 6 — T0 telemetry

```bash
scripts/verify_cutover.sh > data/migrations/cutover-T0.tsv
cat data/migrations/cutover-T0.tsv
# Expected: <iso_timestamp>  <filesize>  <rowcount>  <max_created_at>
```

This is your baseline for NFR-005.

## Step 7 — Wait 24 hours

During this window, USE Claude Code normally. Save memories, do work. Every write should land in HSME (`/home/gary/dev/hsme/data/engram.db`), not in legacy.

If you're paranoid about silent writes, run `scripts/verify_cutover.sh` periodically and watch for any of the three signals to change.

## Step 8 — T+24h telemetry and verification

```bash
scripts/verify_cutover.sh > data/migrations/cutover-T24h.tsv
diff data/migrations/cutover-T0.tsv data/migrations/cutover-T24h.tsv
```

**Expected**: empty diff (filesize, rowcount, max_created_at unchanged).

If non-empty:
1. NFR-005 is violated. A writer to legacy still exists.
2. Find it: `lsof /home/gary/.engram/engram.db` while a fresh Claude Code session is active. Identify the process. Reconfigure that client to point at HSME.
3. Rerun `migrate-legacy --mode=delta` to capture the new writes.
4. Reset T0 (`scripts/verify_cutover.sh > cutover-T0-v2.tsv`) and try the 24h window again.

## Step 9 — Mark mission complete

When NFR-005 holds, append the diff result to the run report's `report.txt`:

```bash
echo "## NFR-005 verification ($(date -u +%Y-%m-%dT%H:%M:%SZ))" >> data/migrations/<run_id>-full/report.txt
echo "T0:    $(cat data/migrations/cutover-T0.tsv)"            >> data/migrations/<run_id>-full/report.txt
echo "T24h:  $(cat data/migrations/cutover-T24h.tsv)"          >> data/migrations/<run_id>-full/report.txt
echo "Diff:  empty (passed)"                                    >> data/migrations/<run_id>-full/report.txt
```

Mission complete. Mission 2 (recency fast path) can now be opened against a corpus with real timestamps.

---

## Rollback

If anything goes wrong at any point, restore the hot backup:

```bash
# The backup path is in [phase 1] of the run report.
ls -lh /home/gary/hsme/backups/

# Restore (the existing script handles this):
scripts/restore.sh /home/gary/hsme/backups/engram-<timestamp>.db
```

The restore script atomically swaps the DB file. Restart the MCP server after restore.

You will lose:
- Any restorations done by phases 3–5 (842 + 62 + 1 = 905 modified rows, plus the `project` column drop).
- Any orphan ingestions from phase 7.

You will NOT lose:
- Any data in legacy Engram (mission never modifies legacy).
- Any memory written to HSME BEFORE the migrator's hot backup was taken (phase 1).

Re-run the migrator after the restore to retry. The operation is idempotent — every phase no-ops if its work is already done.

---

## Schedule the 24h verification (optional)

Spec-kitty / Claude Code can schedule a background agent to run Step 8 for you:

```
/schedule "in 24 hours, run scripts/verify_cutover.sh, diff against data/migrations/cutover-T0.tsv, and report the result"
```

This is optional but recommended — it removes the "did I forget to verify?" failure mode.
