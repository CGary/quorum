# Contract: `scripts/verify_cutover.sh`

**Mission**: `engram-legacy-cutover-and-corpus-restoration-01KQ2SJK`

A small bash utility that captures three signals from the legacy Engram database and prints them as a single TSV line. Used by the operator to verify NFR-005 (zero writes to legacy over a 24-hour window after cutover).

---

## Synopsis

```
scripts/verify_cutover.sh [--legacy-db <path>]
```

## Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `--legacy-db` | `${LEGACY_DB_PATH:-/home/gary/.engram/engram.db}` | Path to legacy Engram database |

## Output

Exactly one line on stdout, TSV (tab-separated):

```
<iso8601_timestamp>\t<filesize_bytes>\t<rowcount>\t<max_created_at>
```

Example:
```
2026-04-25T18:05:00Z	7503872	902	2026-04-25 07:09:52
```

## Exit codes

| Code | Meaning |
|------|---------|
| `0` | Snapshot taken successfully |
| `1` | Legacy DB file not found |
| `2` | SQLite query failed (db locked, schema mismatch, etc.) |

## Behavior

1. `stat` the legacy DB file; capture filesize.
2. Open the legacy DB read-only.
3. Run `SELECT COUNT(*), MAX(created_at) FROM observations` — captures rowcount and max timestamp.
4. Print one TSV line. Exit 0.

The script does **NOT**:
- Modify the legacy DB.
- Modify HSME.
- Diff against a previous snapshot. Diffing is the operator's responsibility (e.g., `diff` two output files).

## Usage pattern (24h verification)

```bash
# At cutover (T0):
scripts/verify_cutover.sh > data/migrations/cutover-T0.tsv

# 24 hours later (T+24h):
scripts/verify_cutover.sh > data/migrations/cutover-T24h.tsv

# Compare:
diff data/migrations/cutover-T0.tsv data/migrations/cutover-T24h.tsv
# If empty: NFR-005 holds (zero writes).
# If non-empty: a writer to legacy still exists. Investigate.
```

## Verification of NFR-005

The TSV format is intentionally machine-trivial so an operator can pipe it through `awk` or check by hand:

```bash
T0=$(scripts/verify_cutover.sh)
sleep 86400
T24=$(scripts/verify_cutover.sh)
if [ "$(echo "$T0" | cut -f2-4)" = "$(echo "$T24" | cut -f2-4)" ]; then
  echo "NFR-005 OK: zero writes over 24h"
else
  echo "NFR-005 VIOLATED: writer still active"
  echo "T0:    $T0"
  echo "T24h:  $T24"
fi
```

## Implementation note

Roughly 15 lines of bash. No new dependencies. Uses the `sqlite3` CLI tool (already available on the developer machine — confirmed during scoping). The script must NOT load any Go binary or require the migrator; it stands alone so the operator can run it from a recovery shell if needed.
