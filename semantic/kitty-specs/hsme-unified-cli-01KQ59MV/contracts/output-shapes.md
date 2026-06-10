# Contract: Output Shapes (`--format=json`)

**Mission**: `hsme-unified-cli-01KQ59MV`

All shapes below are stable contracts that downstream automation may depend on. Any change requires a major version bump of the CLI binary.

## Convention

- Successful results go to **stdout**, terminated with a newline.
- Errors (when `--format=json`) go to **stderr** as a structured object, terminated with a newline.
- Numeric IDs are integers (not strings) — consistent with existing MCP responses.
- Timestamps are ISO 8601 UTC strings unless explicitly noted.
- Optional fields use `omitempty` semantics; absent means the value was not applicable, not null.

## Success shapes per subcommand

### `store`

```json
{
  "memory_id": 12345,
  "status": "stored, pending processing"
}
```

### `search-fuzzy`

```json
{
  "results": [
    {
      "memory_id": 123,
      "chunk_id": 456,
      "content": "...",
      "source_type": "note",
      "score": 0.87,
      "created_at": "2026-04-26T10:00:00Z",
      "...": "additional fields per search.MemorySearchResult"
    }
  ]
}
```

The `results` array shape is **inherited** from `search.MemorySearchResult`. The CLI does not project, rename, or omit fields — verbatim passthrough.

### `search-exact`

```json
{
  "results": [
    {
      "memory_id": 123,
      "chunk_id": 456,
      "content": "...",
      "source_type": "code",
      "...": "additional fields per search.ExactMatchResult"
    }
  ]
}
```

Same passthrough rule as `search-fuzzy`.

### `explore`

The verbatim return value of `search.TraceDependencies(...)`. No envelope. The structure is:

```json
{
  "nodes": [...],
  "edges": [...],
  "...": "per existing TraceDependencies return shape"
}
```

### `status`

```json
{
  "worker_online": true,
  "queue": {
    "total": 1004,
    "completed": 998,
    "pending": 4,
    "processing": 1,
    "failed": 1
  },
  "graph": {
    "nodes": 412,
    "edges": 1187
  },
  "last_pending": {
    "id": 1004,
    "task_type": "extract_entities",
    "memory_id": 998,
    "attempt_count": 1
  }
}
```

`last_pending` is omitted when the queue is empty.

### `admin retry-failed`

```json
{
  "requeued": 7,
  "pending": 11,
  "failed": 0
}
```

### `admin backup`

```json
{
  "backup_path": "backups/engram-20260426T163000Z.db",
  "size_bytes": 12582912
}
```

### `admin restore`

```json
{
  "from": "backups/engram-20260426T163000Z.db",
  "integrity_ok": true,
  "db_path": "data/engram.db"
}
```

### `help`

`help` does not emit JSON — it always prints text usage to stdout regardless of `--format`. This is intentional: help text is for humans, and a JSON shape for it would have no semantic value.

## Error shape (any subcommand with `--format=json`)

```json
{
  "error": "no backups found in 'backups/'",
  "code": 2
}
```

Written to **stderr**. The `code` field matches the OS exit code (1 for usage, 2 for runtime).

## Stability test surface

For each shape above, a test asserts:

1. The **fields present** match the documented schema.
2. The **field types** are correct (int vs. string, etc.).
3. The shape is **stable across runs** with the same input.

These are codified in `cmd/cli/output_test.go` (table-driven) and `tests/modules/cli_test.go` (integration).
