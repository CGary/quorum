# Quickstart: HSME Unified CLI

**Mission**: `hsme-unified-cli-01KQ59MV`
**Audience**: HSME operators and engineers who want to query memory or run admin operations from the terminal.

This walkthrough mirrors the operator's everyday loop — install, inspect, query, maintain, recover.

## 1. Install

```bash
just install     # builds and installs hsme, hsme-worker, hsme-ops, hsme-cli
# or
just cli-install # CLI binary only
```

Both flows install to `$INSTALL_PATH` (default `~/go/bin`) and copy the binary to the repo root for `just` wrappers.

Verify:

```bash
hsme-cli --help
```

Expected output (text mode):

```
hsme-cli — HSME unified CLI

USAGE:
  hsme-cli <subcommand> [flags] [args]

SUBCOMMANDS:
  store           Store content as a memory (reads stdin)
  search-fuzzy    Hybrid lexical + semantic search
  search-exact    Exact FTS5 keyword search
  explore         Trace knowledge-graph dependencies
  status          Show worker, queue, and graph status
  admin           Maintenance operations (retry-failed, backup, restore)
  help            Show help for a subcommand

GLOBAL FLAGS:
  --db PATH                  override SQLITE_DB_PATH
  --ollama-host URL          override OLLAMA_HOST
  --embedding-model NAME     override EMBEDDING_MODEL
  --format text|json         output format (default: text)
  --no-color                 disable ANSI colors
  -h, --help                 show this help

Run `hsme-cli help <subcommand>` for details.
```

## 2. Inspect health

```bash
hsme-cli status
```

Returns the same dashboard `scripts/status.sh` produced, but typed and faster.

```bash
hsme-cli status --watch --interval 1s
```

Live refresh until Ctrl-C. Requires TTY.

```bash
hsme-cli status --format=json | jq '.queue.failed'
```

Pipe-friendly variant for scripts and dashboards.

## 3. Store and query

### Store from a file

```bash
hsme-cli store --source-type note < notes/decision-log.md
```

Output:

```json
{ "memory_id": 12345, "status": "stored, pending processing" }
```

### Store from a heredoc

```bash
hsme-cli store --source-type code <<'EOF'
The fuzzy search uses RRF fusion of FTS5 + sqlite-vec. See src/core/search/fuzzy.go.
EOF
```

### Semantic search

```bash
hsme-cli search-fuzzy "decay ranking adversarial" --limit 5
```

Requires Ollama to be reachable (embedder needed for query embedding).

### Lexical search

```bash
hsme-cli search-exact "RRF_HALF_LIFE_DAYS" --limit 10
```

No Ollama dependency.

### Graph exploration

```bash
hsme-cli explore "Redis" --direction downstream --max-depth 3
```

## 4. Maintenance loop

### Re-queue stuck tasks

```bash
hsme-cli admin retry-failed
```

Output:

```json
{ "requeued": 7, "pending": 11, "failed": 0 }
```

### Backup

```bash
hsme-cli admin backup
# default location: backups/engram-<UTC timestamp>.db
```

```bash
hsme-cli admin backup --out /mnt/external/hsme-snapshot.db
```

The backup is **WAL-safe** and runs concurrently with a live MCP server. No service interruption required.

### Restore

```bash
hsme-cli admin restore --latest
```

Picks the most recent `backups/engram-*.db`, runs `PRAGMA integrity_check`, atomically replaces the live DB, cleans up `-wal`/`-shm` sidecars.

```bash
hsme-cli admin restore --from backups/engram-20260420T020000Z.db
```

**⚠️ Operator responsibility**: stop running services (`hsme`, `hsme-worker`, `hsme-ops`) before invoking restore. The CLI does not check for running processes by design (recovery has priority over running services).

## 5. JSON consumption examples

### Pipe to `jq`

```bash
hsme-cli search-exact "ollama" --format=json | jq '.results | length'
```

### Daily ops cron

```bash
#!/bin/bash
# /etc/cron.daily/hsme-housekeeping
set -e
hsme-cli admin retry-failed --format=json > /var/log/hsme/retry.log
hsme-cli admin backup --format=json > /var/log/hsme/backup.log
```

### Alert on failed tasks

```bash
FAILED=$(hsme-cli status --format=json | jq '.queue.failed')
if [ "$FAILED" -gt 10 ]; then
  echo "alert: $FAILED HSME tasks blocked" | wall
fi
```

## 6. Help and discovery

```bash
hsme-cli help              # top-level help
hsme-cli help store        # detailed help for `store`
hsme-cli store --help      # equivalent
```

## 7. Coexistence with the MCP server

The CLI and MCP server can run simultaneously. SQLite WAL mode coordinates concurrent readers and writers. There is no extra locking introduced.

**Practical guidance**:
- Read subcommands (`status`, `search-*`, `explore`) are 100% safe alongside the server.
- Write subcommands (`store`, `admin retry-failed`, `admin backup`) are safe — backups especially are designed for live operation.
- `admin restore` is the one operation where you should stop services first (operator responsibility, not enforced by the binary).

## 8. Exit codes

| Exit code | Meaning | Example |
|-----------|---------|---------|
| `0` | Success | Any subcommand completing normally |
| `1` | Usage error | `hsme-cli unknown-thing`, missing required flag |
| `2` | Runtime error | DB unreachable, integrity check failure |

Scripts can branch on exit code as usual.
