---
name: hsme-cli-usage
description: Guide for agents calling hsme-cli (store, search-fuzzy, search-exact, explore, status, admin backup/restore/retry-failed, import-quorum) with the mk-cli JSON contract. Use whenever a skill or agent needs to query the HSME semantic memory engine via CLI instead of read-querying its SQLite DB directly. Do not use for the Quorum core module's own memory; that is quorum memory save/search.
---

# hsme-cli-usage

`hsme-cli` is HSME's unified CLI (`semantic/cmd/cli/*.go`, built via
`cd semantic && just install`). It exposes the `--json` / `--schema` /
`--dry-run` mk-cli agent contract. Skills call it for the advisory HSME read
hook (search-fuzzy / search-exact) and agents use it directly for store,
explore, status, admin, and import-quorum.

> HSME is **subordinate** (ADR 0008): results are advisory suggestions
> requiring human confirmation before influencing any persisted artifact.
> See `AGENTS.md` § "hsme-cli for agents".

## When to use

- An advisory read hook in a `/q-*` skill needs past tasks, failures, or
  capsules — call `hsme-cli search-fuzzy` or `search-exact` with `--json`.
- An agent needs to store a context note (`store`), trace knowledge-graph
  dependencies (`explore`), check system health (`status`), manage
  snapshots (`admin backup` / `admin restore`), retry failed tasks
  (`admin retry-failed`), or import Quorum memory into HSME (`import-quorum`).
- You need the exact JSON I/O contract for a subcommand without running it —
  call `--schema`.

## When NOT to use

- Quorum's own centralized SQLite memory (`quorum memory save` /
  `quorum memory search`) — that is the root module's curated memory, not
  HSME. Do not call `hsme-cli` to read or write Quorum memory.
- HSME memory when the binary is not installed or Ollama is unavailable —
  the skill's graceful-degradation path emits
  `[ADVISOR] No disponible — se procede sin contexto semántico.` and
  continues without blocking.
- Pasting full `--help` text into a prompt — use `--schema` instead for the
  machine-readable contract.

## Required agent flags

- `--json` — required for programmatic parsing. Under `--json`, stdout
  carries exactly one JSON object (success or error); all logs go to stderr.
- `--no-input` — required for non-interactive runs. Accepted by every
  subcommand for contract symmetry with `quorum fleet run`; no hsme-cli
  subcommand prompts today.
- `--project <name>` — required by search/store/import-quorum to scope
  results (HSME does not isolate projects by itself).
- `--dry-run` — use before `store`, `admin restore`, `import-quorum` to
  preview without side effects.
- `--schema` — print the input/output JSON contract for a subcommand and
  exit 0 (no DB open, no network).
- `--output <file>` — for large `search-fuzzy` / `search-exact` /
  `explore` / `import-quorum` results; the stdout envelope's `data` becomes
  `{"result_file": "<path>"}`.
- `--timeout <seconds>` — bounds Ollama/network operations (0 = default).

## Common commands

The database path defaults to `data/engram.db` **relative to the current working
directory** — from any cwd other than the semantic module root, set
`SQLITE_DB_PATH=<path-to-engram.db>` (or pass `--db <path>`) or every command
except `--schema` fails with `INTERNAL_ERROR: failed to open database`.

```bash
# Introspection
hsme-cli --schema
hsme-cli search-fuzzy --schema
hsme-cli store --schema

# Advisory read hook (skills call these)
hsme-cli search-fuzzy "<query text>" --project quorum --limit 10 --json --no-input
hsme-cli search-exact "<keyword>" --project quorum --limit 10 --json --no-input

# Knowledge-graph traversal
hsme-cli explore "<entity_name>" --direction both --json --no-input

# System health
hsme-cli status --json --no-input

# Store a context note (preview first)
hsme-cli store --source-type note --project quorum --dry-run --json --no-input < notes.md
hsme-cli store --source-type note --project quorum --json --no-input < notes.md

# Admin operations
hsme-cli admin retry-failed --json --no-input
hsme-cli admin backup --json --no-input
hsme-cli admin restore --from path/to/backup.db --dry-run --json --no-input
hsme-cli admin restore --latest --json --no-input

# Import Quorum memory into HSME
hsme-cli import-quorum --project <proj> --source all --json --no-input
```

## JSON contract

### Success envelope (stdout, one object)

```json
{
  "ok": true,
  "command": "<see table below>",
  "summary": "one sentence",
  "data": { ... },
  "next_actions": []
}
```

`data` shape per subcommand:

| `command` | `data` shape |
|---|---|
| `store` | `{"memory_id": <int>, "status": "stored"}` |
| `search-fuzzy` | `{"results": [{"memory_id": <int>, "score": <float>, "is_superseded": <bool>, "vector_coverage": "<complete\|partial\|none>", "highlights": [{"chunk_id": <int>, "chunk_index": <int>, "text": "<str>"}]}]}` |
| `search-exact` | `{"results": [{"memory_id": <int>, "chunk_id": <int>, "chunk_index": <int>, "text": "<str>", "score": <float>}]}` |
| `explore` | `{"entity": "<str>", "nodes": [...], "edges": [...], "truncated": <bool>}` |
| `status` | `{"status": {"active": <int>, "superseded": <int>, ...}, "graph_stats": {...}, "worker_running": <bool>}` |
| `admin.backup` | `{"backup_path": "<str>", "file_count": <int>}` |
| `admin.restore` | `{"restored_from": "<str>", "memories": <int>}` |
| `admin.retry-failed` | `{"retried": <int>}` |
| `import-quorum` | `{"curated": {"inserted": <int>, "skipped": <int>, "errored": <int>}, "capsules": {...}, "summary": "<str>"}` |
| `version` | `{"version": "<str>"}` |

### Error envelope (stdout under `--json`, stderr in human mode — one object)

```json
{
  "ok": false,
  "command": "<subcommand>",
  "error": {
    "code": "<see table below>",
    "message": "<human-readable>",
    "field": "<flag or arg name, if applicable>",
    "received": "<invalid value, if applicable>"
  },
  "retryable": <bool>,
  "suggested_fix": {"command": "<shell command to retry>"}
}
```

Exit codes: `exitUsage = 1` (usage/validation errors), `exitRuntime = 2`
(runtime/IO/network errors).

## Error handling

Stable `error.code` values — exactly 10, defined in `semantic/cmd/cli/envelope.go`:

| Code | Exit | Retryable | Meaning |
|---|---|---|---|
| `MISSING_REQUIRED_FLAG` | 1 | true | A required flag or positional arg is absent. |
| `INVALID_ENUM` | 1 | false | `--source` (import-quorum), `--direction` (explore), or `--source` value is not in its enum. |
| `INVALID_ARGUMENT` | 1 | false | Argument is structurally wrong (e.g. unknown subcommand). |
| `VALIDATION_FAILED` | 1 | true | Input fails a check (e.g. `store` stdin is a TTY or empty; `--from`/`--latest` mutually exclusive on `admin restore`). |
| `FILE_NOT_FOUND` | 2 | false | `--from <path>`, `--quorum-db <path>`, or backup file does not exist. |
| `CONFLICT` | 2 | true | `store` with `--force-reingest` but no matching `--supersedes`; `suggested_fix` is `hsme-cli store ... --supersedes <existing_memory_id>`. |
| `TIMEOUT` | 2 | true | Operation exceeded `--timeout`. |
| `NETWORK_ERROR` | 2 | true | HTTP/Ollama/OpenRouter round-trip failure. |
| `PERMISSION_DENIED` | 2 | false | Filesystem permission error (e.g. `admin backup` cannot create `backups/`). |
| `INTERNAL_ERROR` | 2 | false | Any other runtime error (DB open, indexer, query, marshalling). |

Rules:

- **Retryable = true** means the same command with a trivial fix is expected
  to succeed (flag-shaped or transient failures).
- **Retryable = false** means structural or permission failures — do not
  retry the identical command.
- On `INVALID_ENUM`, the `error.message` lists the valid values.
- On `CONFLICT`, `suggested_fix.command` gives the exact retry command.
- Under `--json`, parse the single stdout JSON object; never parse stderr for
  data.
- Without `--json` (human mode), errors print one concise line to stderr:
  `error: <message>` and optionally `try: <suggested_fix.command>`.
