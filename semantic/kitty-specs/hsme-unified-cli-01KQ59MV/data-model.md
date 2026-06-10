# Data Model: HSME Unified CLI

**Mission**: `hsme-unified-cli-01KQ59MV`
**Phase**: 1 (Design)

## Scope

The CLI does not introduce new persistent entities. All on-disk schema is owned by the existing `src/storage/sqlite` migrations. This document captures the **runtime data structures** the CLI passes between layers (parsing → bootstrap → core function → output formatter) and the **schema of values exposed to the operator**, so contract tests can assert against stable shapes.

## 1. Configuration entity

**Owner**: `src/bootstrap/config.go`

```go
type Config struct {
    DBPath         string  // SQLITE_DB_PATH or --db; default "data/engram.db"
    OllamaHost     string  // OLLAMA_HOST or --ollama-host; default "" → driver picks default
    EmbeddingModel string  // EMBEDDING_MODEL or --embedding-model; default "nomic-embed-text"
    EmbeddingDim   int     // hard-coded to 768 (matches existing code)
}

func LoadFromEnv() Config             // reads env, applies defaults, no flag awareness
func (c *Config) ApplyFlagOverrides(flags *flag.FlagSet)  // overlays --db, --ollama-host, --embedding-model
```

**Invariants:**
- `DBPath` is never empty after `LoadFromEnv()` (default applied).
- `EmbeddingModel` is never empty after `LoadFromEnv()` (default applied).
- `EmbeddingDim` is never mutated externally (constant).

## 2. Subcommand input parameter structs

Each subcommand parses CLI args into a typed struct before invoking the core function. These are internal types — not part of the JSON output contract.

### 2.1 `store`

```go
type StoreArgs struct {
    SourceType         string  // --source-type (required)
    Project            string  // --project (optional)
    SupersedesMemoryID *int64  // --supersedes (optional, nil if not provided)
    ForceReingest      bool    // --force-reingest (optional)
    Content            string  // read from stdin
}
```

### 2.2 `search-fuzzy`

```go
type SearchFuzzyArgs struct {
    Query   string  // positional arg (required)
    Limit   int     // --limit (default 10)
    Project string  // --project (optional)
}
```

### 2.3 `search-exact`

```go
type SearchExactArgs struct {
    Keyword string  // positional arg (required)
    Limit   int     // --limit (default 10)
    Project string  // --project (optional)
}
```

### 2.4 `explore`

```go
type ExploreArgs struct {
    EntityName string  // positional arg (required)
    Direction  string  // --direction in {"upstream","downstream","both"} (default "both")
    MaxDepth   int     // --max-depth (default 5)
    MaxNodes   int     // --max-nodes (default 100)
}
```

### 2.5 `status`

```go
type StatusArgs struct {
    Watch    bool           // --watch
    Interval time.Duration  // --interval (default 2s)
    Format   string         // --format in {"text","json"} (default "text")
    NoColor  bool           // --no-color or NO_COLOR env
}
```

### 2.6 `admin retry-failed`

```go
type RetryFailedArgs struct {
    Format string  // --format
}
```

### 2.7 `admin backup`

```go
type BackupArgs struct {
    Out    string  // --out (default: "backups/engram-<UTC timestamp>.db")
    Format string  // --format
}
```

### 2.8 `admin restore`

```go
type RestoreArgs struct {
    From   string  // --from <path>; mutually exclusive with --latest
    Latest bool    // --latest; mutually exclusive with --from
    Format string  // --format
}
```

**Validation rule**: exactly one of `From`/`Latest` must be set; both unset or both set → usage error (exit 1).

### 2.9 `help`

```go
type HelpArgs struct {
    Subcommand string  // optional positional; if empty, top-level help
}
```

## 3. Result value types (CLI output)

These are the **stable shapes** that JSON output must conform to. They reuse the core types where possible to avoid drift.

### 3.1 `store` result

```go
type StoreResult struct {
    MemoryID int64  `json:"memory_id"`
    Status   string `json:"status"`  // mirrors MCP: "stored, pending processing"
}
```

### 3.2 Search results

`search-fuzzy` returns `search.MemorySearchResult` (defined in `src/core/search/`); `search-exact` returns `search.ExactMatchResult`. These are wrapped in:

```go
// search-fuzzy
type FuzzyResultEnvelope struct {
    Results []search.MemorySearchResult `json:"results"`
}

// search-exact
type ExactResultEnvelope struct {
    Results []search.ExactMatchResult `json:"results"`
}
```

The envelope mirrors the MCP wrapper functions in `cmd/hsme/main.go:20-44`.

### 3.3 `explore` result

`search.TraceDependencies` already returns a structured payload. The CLI emits it verbatim; no envelope.

### 3.4 `status` result

```go
type StatusResult struct {
    WorkerOnline bool         `json:"worker_online"`
    Queue        QueueStats   `json:"queue"`
    Graph        GraphStats   `json:"graph"`
    LastPending  *PendingTask `json:"last_pending,omitempty"`
}

type QueueStats struct {
    Total      int64 `json:"total"`
    Completed  int64 `json:"completed"`
    Pending    int64 `json:"pending"`
    Processing int64 `json:"processing"`
    Failed     int64 `json:"failed"`
}

type GraphStats struct {
    Nodes int64 `json:"nodes"`
    Edges int64 `json:"edges"`
}

type PendingTask struct {
    ID           int64  `json:"id"`
    TaskType     string `json:"task_type"`
    MemoryID     int64  `json:"memory_id"`
    AttemptCount int    `json:"attempt_count"`
}
```

**Source of truth**: `scripts/status.sh:22-29` (queue stats SQL) and `:44-45` (graph counts). The CLI port replicates the same queries via `database/sql`.

### 3.5 `admin retry-failed` result

```go
type RetryFailedResult struct {
    Requeued int64 `json:"requeued"`     // rows affected
    Pending  int64 `json:"pending"`      // post-update count
    Failed   int64 `json:"failed"`       // post-update count
}
```

### 3.6 `admin backup` result

```go
type BackupResult struct {
    BackupPath string `json:"backup_path"`
    SizeBytes  int64  `json:"size_bytes"`
}
```

### 3.7 `admin restore` result

```go
type RestoreResult struct {
    From         string `json:"from"`           // resolved source path
    IntegrityOK  bool   `json:"integrity_ok"`
    DBPath       string `json:"db_path"`        // restore destination
}
```

### 3.8 Error result (`--format=json` only)

```go
type ErrorResult struct {
    Error string `json:"error"`
    Code  int    `json:"code"`
}
```

Written to stderr. `Code` matches the OS exit code (1 or 2).

## 4. Worker detection contract

`status` must determine if the `hsme-worker` binary is currently running.

**Approach**: scan `/proc/*/comm` on Linux. If `/proc` is unavailable (macOS), fall back to `os/exec` invocation of `pgrep -x hsme-worker`. If neither is available, set `worker_online: false` and add a debug log to stderr (text mode) or a `worker_detection: "unsupported"` field (JSON mode — TBD during implementation).

**Note**: this is the only place the CLI shells out. It's a probe, not a write, so failure modes are bounded. Documented as a known limitation for future cross-platform work.

## 5. Backup file naming convention

**Pattern**: `engram-<UTC timestamp>.db` where timestamp is `YYYYMMDDTHHMMSSZ`.

**Examples**:
- `backups/engram-20260426T163000Z.db`
- `backups/engram-20260427T020000Z.db`

**Compatible** with the existing pattern from `scripts/backup_hot.sh:9-11` (`%Y%m%dT%H%M%SZ`).

**Selection by `--latest`**: glob `backups/engram-*.db`, sort by mtime descending, pick first. The justfile previously sorted by name pattern (`engram_*.db`); the underscore vs. dash difference is intentional — the new CLI standardizes on dash to match `backup_hot.sh`. Existing backups with underscores remain readable via `--from <path>` but are not picked up by `--latest`. This is a one-time, low-impact migration that the operator controls.

## 6. Lifecycle and state transitions

### 6.1 Backup lifecycle

```
[Initial: live DB exists]
        ↓
[Operator runs backup]
        ↓
[Open backup target file (O_EXCL)]
        ↓
[Online Backup API: copy pages]
        ↓
[On completion: file synced to disk]
        ↓
[Result emitted with backup_path + size]
```

**Failure modes**:
- Target path exists → operation aborts with error (file not overwritten).
- Backup interrupted (signal/IO error) → partial file removed, error returned.

### 6.2 Restore lifecycle

```
[Initial: live DB possibly in use]
        ↓
[Resolve source: --from or --latest]
        ↓
[Verify integrity: PRAGMA integrity_check]
        ├─ FAIL → abort, exit 2, live DB untouched
        └─ OK ↓
[Copy source → temp file in same dir as target]
        ↓
[Remove target -wal and -shm sidecars]
        ↓
[os.Rename(temp, target) — atomic]
        ↓
[Result emitted with from + db_path + integrity_ok=true]
```

**Failure modes**:
- No backups found (`--latest`) → exit 2, "no backups in <dir>".
- Integrity check fails → exit 2, "backup at <path> failed integrity check".
- Rename fails (cross-device) → exit 2 with cause; this should never happen because temp is created in target's dir.

## 7. Build artifact

The CLI compiles to a single binary at the repo root: `hsme-cli`. Build tags `sqlite_fts5 sqlite_vec` required (C-001). Consistent with existing binaries.
