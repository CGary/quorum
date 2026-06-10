# Contract: Flags per Subcommand

**Mission**: `hsme-unified-cli-01KQ59MV`

This is the authoritative reference for every flag and positional argument the CLI accepts. Implementation and tests must match this table.

## Global flags

These are accepted by every subcommand (parsed before subcommand-specific flags):

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--db <path>` | string | env `SQLITE_DB_PATH` or `data/engram.db` | Override DB file path |
| `--ollama-host <url>` | string | env `OLLAMA_HOST` or driver default | Override Ollama host (only used when subcommand needs embedder) |
| `--embedding-model <name>` | string | env `EMBEDDING_MODEL` or `nomic-embed-text` | Override embedding model |
| `--format text\|json` | string | `text` | Output format |
| `--no-color` | bool | `false` | Disable ANSI codes even on TTY (also honors `NO_COLOR` env) |
| `--help`, `-h` | bool | — | Print subcommand-specific help and exit 0 |

## Per-subcommand contract

### `hsme-cli store`

**Synopsis**: `hsme-cli store --source-type <type> [--project <name>] [--supersedes <id>] [--force-reingest] < content`

| Flag/arg | Required | Default | Notes |
|----------|----------|---------|-------|
| `--source-type <type>` | yes | — | One of `code`, `note`, `log` (per existing convention) |
| `--project <name>` | no | "" | Project tag |
| `--supersedes <id>` | no | unset | Marks the new memory as superseding the given memory ID |
| `--force-reingest` | no | `false` | Force re-processing even if content matches existing |
| Stdin | yes | — | Content; CLI fails with exit 1 if stdin is a TTY |

### `hsme-cli search-fuzzy`

**Synopsis**: `hsme-cli search-fuzzy "<query>" [--limit N] [--project <name>]`

| Flag/arg | Required | Default | Notes |
|----------|----------|---------|-------|
| Positional `<query>` | yes | — | Free-text query |
| `--limit N` | no | `10` | Max results |
| `--project <name>` | no | "" | Filter by project |

Requires embedder.

### `hsme-cli search-exact`

**Synopsis**: `hsme-cli search-exact "<keyword>" [--limit N] [--project <name>]`

| Flag/arg | Required | Default | Notes |
|----------|----------|---------|-------|
| Positional `<keyword>` | yes | — | FTS5 keyword/phrase |
| `--limit N` | no | `10` | Max results |
| `--project <name>` | no | "" | Filter by project |

No embedder.

### `hsme-cli explore`

**Synopsis**: `hsme-cli explore "<entity>" [--direction both|upstream|downstream] [--max-depth N] [--max-nodes N]`

| Flag/arg | Required | Default | Notes |
|----------|----------|---------|-------|
| Positional `<entity>` | yes | — | Entity name; canonicalized via `indexer.CanonicalizeName` before lookup |
| `--direction <both\|upstream\|downstream>` | no | `both` | Traversal direction |
| `--max-depth N` | no | `5` | Max graph depth |
| `--max-nodes N` | no | `100` | Max nodes returned |

No embedder.

### `hsme-cli status`

**Synopsis**: `hsme-cli status [--watch] [--interval <duration>]`

| Flag/arg | Required | Default | Notes |
|----------|----------|---------|-------|
| `--watch` | no | `false` | Refresh on interval until interrupted; requires TTY |
| `--interval <duration>` | no | `2s` | Go duration string (e.g. `500ms`, `5s`, `1m`) |

No embedder.

### `hsme-cli admin retry-failed`

**Synopsis**: `hsme-cli admin retry-failed`

No subcommand-specific flags (only globals).

No embedder.

### `hsme-cli admin backup`

**Synopsis**: `hsme-cli admin backup [--out <path>]`

| Flag/arg | Required | Default | Notes |
|----------|----------|---------|-------|
| `--out <path>` | no | `backups/engram-<UTC ts>.db` | Destination file; parent dir auto-created |

No embedder.

### `hsme-cli admin restore`

**Synopsis**: `hsme-cli admin restore (--from <path> | --latest)`

| Flag/arg | Required | Default | Notes |
|----------|----------|---------|-------|
| `--from <path>` | one-of | — | Restore from explicit backup file |
| `--latest` | one-of | — | Restore from most recent `engram-*.db` in `backups/` (by mtime) |

Exactly one of `--from` / `--latest` must be set.

No embedder.

### `hsme-cli help`

**Synopsis**: `hsme-cli help [<subcommand>]`

| Flag/arg | Required | Default | Notes |
|----------|----------|---------|-------|
| Positional `<subcommand>` | no | — | If given, prints detailed help for that subcommand; otherwise prints top-level help |

No DB. No embedder.

## Test surface for flag contracts

For each subcommand:

1. **Required flag/arg missing** → exit 1, error message names the missing flag.
2. **Unknown flag** → exit 1, error message names the unknown flag.
3. **Flag with wrong type** (e.g. `--limit foo`) → exit 1.
4. **Mutually exclusive flags both set** (only `admin restore`) → exit 1.
5. **Default values applied** when flag absent → integration test verifies behavior.

These are table-driven in `cmd/cli/flags_test.go` and exercised end-to-end in `tests/modules/cli_test.go`.
