---
work_package_id: WP02
title: CLI Dispatcher and Subcommand Handlers
dependencies: []
requirement_refs:
- FR-001
- FR-002
- FR-003
- FR-004
- FR-005
- FR-010
- FR-011
- FR-012
- FR-013
- FR-014
- FR-020
- FR-021
- FR-022
- FR-023
- FR-030
- FR-031
- FR-032
- FR-033
- FR-034
- FR-035
- FR-036
- FR-040
- FR-041
- FR-042
- FR-043
- FR-080
- FR-081
- FR-082
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
subtasks:
- T009
- T010
- T011
- T012
- T013
- T014
- T015
- T016
- T017
agent: "gemini:o3:reviewer:reviewer"
shell_pid: "2100044"
history:
- date: '2026-04-26T16:47:42Z'
  action: tasks generated
  actor: tasks skill
agent_profile: ''
authoritative_surface: cmd/cli/
execution_mode: code_change
model: ''
owned_files:
- cmd/cli/main.go
- cmd/cli/output.go
- cmd/cli/flags.go
- cmd/cli/help.go
- cmd/cli/store.go
- cmd/cli/search.go
- cmd/cli/explore.go
- cmd/cli/status.go
- cmd/cli/admin.go
role: ''
---

## ⚡ Do This First: Load Agent Profile

Before reading anything else, load the implementer agent profile:

```
/ad-hoc-profile-load implementer
```

This injects your role identity, skill directives, and execution context. All other instructions in this prompt are subordinate to the profile load.

---

## Objective

Create `cmd/cli/` with the entry point, subcommand dispatcher, all nine subcommand handlers, shared output formatting, and shared flag definitions. Every handler invokes the corresponding core function with no transformation. This is the operator-facing interface of the entire mission.

---

## Context

### Feature directory

`/home/gary/dev/hsme/kitty-specs/hsme-unified-cli-01KQ59MV`

### Key design documents (read before implementing)

- **spec.md** — all FR requirements, exit code contract (FR-080..FR-082), NFR constraints
- **data-model.md** — input parameter structs (sections 2.1–2.9), result types (sections 3.1–3.8), worker detection contract (section 4), backup naming (section 5)
- **research.md** — R4 (TTY/ANSI detection), R6 (watch signal handling), R7 (error output contract), R8 (justfile cleanup pattern)
- **contracts/** — exit-codes.md, output-shapes.md, flags.md (read these too)

### What this WP produces

```
cmd/cli/
├── main.go        # entry point + dispatcher
├── output.go      # TTY detection, text/JSON formatting, ANSI colors
├── flags.go       # shared flag defs (--db, --ollama-host, --embedding-model, --format)
├── help.go        # top-level + per-subcommand help
├── store.go       # hsme-cli store
├── search.go      # hsme-cli search-fuzzy + search-exact
├── explore.go     # hsme-cli explore
├── status.go      # hsme-cli status [--watch] [--interval]
└── admin.go       # hsme-cli admin retry-failed/backup/restore
```

### Bootstrap dependency

This WP uses `src/bootstrap/` from WP01. Import it as `github.com/hsme/core/src/bootstrap`. Subcommands that need embedder call `bootstrap.OpenWithEmbedder`; those that don't call `bootstrap.OpenDB`.

### Embedder requirement matrix

| Subcommand | Bootstrap call | Why |
|------------|----------------|-----|
| `help` | none | pure function, no DB needed |
| `store` | `OpenWithEmbedder` | indexer.StoreContext needs embedder |
| `search-fuzzy` | `OpenWithEmbedder` | FuzzySearch needs embedder for query embedding |
| `search-exact` | `OpenDB` | lexical search, no embedder needed |
| `explore` | `OpenDB` | TraceDependencies, no embedder needed |
| `status` | `OpenDB` | SQL queries only, no embedder needed |
| `admin retry-failed` | `OpenDB` | admin SQL, no embedder needed |
| `admin backup` | `OpenDB` | backup API, no embedder needed |
| `admin restore` | `OpenDB` | restore logic, no embedder needed |

---

## Guidance per Subtask

### T009 — Create `cmd/cli/output.go`

**Purpose**: Shared output formatting — TTY detection, text output with ANSI colors, JSON output, error formatting.

**File**: `cmd/cli/output.go` (new file)

**Required exports**:

```go
func IsTTY() bool                           // true if stdout is a TTY
func ShouldColor() bool                     // true if TTY + no NO_COLOR + no --no-color
func FormatJSON(v interface{}) (string, error)  // marshal to indent 2
func FormatText(v interface{}) string       // human-readable (for status/search results)
func WriteResult(w io.Writer, v interface{}, format string) error
func WriteError(w io.Writer, err error, code int, format string)
```

**TTY detection** (R4):
```go
func IsTTY() bool {
    fi, err := os.Stdout.Stat()
    if err != nil {
        return false
    }
    return (fi.Mode() & os.ModeCharDevice) != 0
}
```

**Color decision**:
- `ShouldColor()` returns `IsTTY() && !os.Getenv("NO_COLOR") != "" && !noColorFlag`
- ANSI color codes: green for "online"/"ok", red for "offline"/"failed", yellow for "pending"
- Example: `fmt.Sprintf("\033[32m%s\033[0m", "online")`

**JSON formatting** (R7):
- `FormatJSON`: `json.MarshalIndent(v, "", "  ")` — 2-space indent
- Error in JSON mode: `{"error": "<msg>", "code": <exit_code>}` written to stderr
- Text mode errors: plain text to stderr prefixed with `error: `

**Build tags**: No build tags needed (pure I/O).

---

### T010 — Create `cmd/cli/flags.go`

**Purpose**: Shared flag definitions used across all subcommands.

**File**: `cmd/cli/flags.go` (new file)

**Required flags** (FR-050, FR-051):

```go
func RegisterDBFlags(fs *flag.FlagSet, cfg *bootstrap.Config)
func GetDBPath(fs *flag.FlagSet) string
func GetOllamaHost(fs *flag.FlagSet) string
func GetEmbeddingModel(fs *flag.FlagSet) string
```

**RegisterDBFlags** registers `--db`, `--ollama-host`, `--embedding-model` on the provided `*flag.FlagSet`. It also registers `--format` (text|json, default text), `--no-color` (bool).

**Pattern**: Each subcommand's `setup` function calls `RegisterDBFlags` and then parses subcommand-specific flags.

**Usage**: In `main.go` before subcommand dispatch, call `bootstrap.LoadFromEnv()` and overlay with command-line flags via `cfg.ApplyFlagOverrides(flag.CommandLine)`.

---

### T011 — Create `cmd/cli/help.go`

**Purpose**: Top-level help and per-subcommand help text.

**File**: `cmd/cli/help.go` (new file)

**Top-level help** (FR-002): Print available subcommands:

```
HSME CLI — Unified command-line interface for HSME

Usage: hsme-cli <subcommand> [flags]

Subcommands:
  store          Ingest content from stdin
  search-fuzzy   Semantic search
  search-exact   Keyword search
  explore        Trace graph dependencies
  status         Show system health
  admin          Admin operations (backup, restore, retry-failed)
  help           Show this help or help for a specific subcommand

Use "hsme-cli help <subcommand>" for detailed usage.
```

**Per-subcommand help** (FR-003, FR-004): When `help.go` receives a subcommand name, print its specific flags and usage.

**Exit codes** (FR-081): Help for unknown subcommand → print top-level help + exit 1.

**No build tags needed**.

---

### T012 — Create `cmd/cli/main.go`

**Purpose**: Entry point and subcommand dispatcher.

**File**: `cmd/cli/main.go` (new file)

**Entry point** (`main()`):
1. Parse global flags ( `--db`, `--ollama-host`, `--embedding-model`, `--format`, `--no-color` ) using `flag.CommandLine`.
2. Load env via `bootstrap.LoadFromEnv()`.
3. Apply flag overrides via `cfg.ApplyFlagOverrides(flag.CommandLine)`.
4. If no args, print top-level help + exit 1 (FR-002).
5. Dispatch on `os.Args[1]` to subcommand handler.

**Dispatcher pattern** (stdlib `flag`, manual dispatch — NFR-001):

```go
switch os.Args[1] {
case "store":
    runStore(os.Args[2:])
case "search-fuzzy":
    runSearchFuzzy(os.Args[2:])
// ... etc
case "help":
    runHelp(os.Args[2:])
default:
    fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", os.Args[1])
    printTopLevelHelp()
    os.Exit(1)
}
```

**`--help` handling** (FR-004): If any subcommand receives `--help` or `-h`, delegate to `help.go` before attempting to parse subcommand-specific flags.

**Exit codes** (FR-080..FR-082):
- Success: `0`
- Usage error (unknown subcommand, missing required arg): `1`
- Runtime error (DB failure, embedder failure, query error): `2`

**Build tags**: `//go:build sqlite_fts5 sqlite_vec` — CLI opens DB.

---

### T013 — Create `cmd/cli/store.go`

**Purpose**: Implement `hsme-cli store`.

**File**: `cmd/cli/store.go` (new file)

**FR**: FR-010, FR-040–FR-043, FR-050–FR-054

**Subcommand name**: `store`

**Usage**: `hsme-cli store --source-type <type> [--project <proj>] [--supersedes <id>] [--force-reingest]`

**Flag parsing** (use `flag.NewFlagSet("store", flag.ExitOnError)`):
- `--source-type` (required, string)
- `--project` (optional, string)
- `--supersedes` (optional, int64 — pointer, nil if not provided)
- `--force-reingest` (optional, bool)

**Stdin read** (FR-010): Read all of stdin to get content. If stdin is empty and TTY is detected, print hint: "store: no input on stdin. Usage: hsme-cli store --source-type <type> < notes.md" and exit 1.

**Bootstrap call**: `bootstrap.OpenWithEmbedder(cfg)` (embedder required — C-005)

**Core function**: `indexer.StoreContext(db, content, sourceType, project, supersedesID, forceReingest)` from `src/core/indexer/ingest.go`

**Output**: Format result via `output.go` helpers.

**Exit on error**: Wrap errors with context, pass to `output.WriteError`, exit 2.

**Tests**: Add `store_test.go` in WP03.

---

### T014 — Create `cmd/cli/search.go`

**Purpose**: Implement `hsme-cli search-fuzzy` and `hsme-cli search-exact`.

**File**: `cmd/cli/search.go` (new file)

**FR**: FR-011, FR-012, FR-040–FR-043, FR-050–FR-054

**`search-fuzzy`**: Usage `hsme-cli search-fuzzy <query> [--limit <n>] [--project <proj>]`
- Positional arg `query` (required)
- `--limit` default 10
- Bootstrap: `OpenWithEmbedder` (embedder needed for query embedding)
- Core function: `search.FuzzySearch(ctx, db, embedder, query, limit, project)` from `src/core/search/fuzzy.go`
- Output: envelope `{"results": [...]}`

**`search-exact`**: Usage `hsme-cli search-exact <keyword> [--limit <n>] [--project <proj>]`
- Positional arg `keyword` (required)
- `--limit` default 10
- Bootstrap: `OpenDB` (no embedder needed)
- Core function: `search.ExactSearch(ctx, db, keyword, limit, project)` from `src/core/search/fuzzy.go`
- Output: envelope `{"results": [...]}`

**Parse positional args**: Use `flag.NewFlagSet` with `flag.Args()` to get remaining non-flag arguments.

**Output format**: JSON envelope with `results` array. For text mode, print each result on its own line with memory_id + snippet.

---

### T015 — Create `cmd/cli/explore.go`

**Purpose**: Implement `hsme-cli explore`.

**File**: `cmd/cli/explore.go` (new file)

**FR**: FR-013, FR-040–FR-043, FR-050–FR-054

**Usage**: `hsme-cli explore <entity-name> [--direction upstream|downstream|both] [--max-depth <n>] [--max-nodes <n>]`

**Flags**:
- `direction` default "both"
- `max-depth` default 5
- `max-nodes` default 100

**Bootstrap**: `OpenDB` (no embedder needed — explore uses existing graph data, not embeddings)

**Core function**: `indexer.CanonicalizeName(name)` then `search.TraceDependencies(ctx, db, canonical, direction, maxDepth, maxNodes)` from `src/core/search/graph.go` and `src/core/indexer/normalize.go`

**Output**: `search.TraceDependencies` returns structured payload — emit it verbatim (no envelope).

---

### T016 — Create `cmd/cli/status.go`

**Purpose**: Implement `hsme-cli status` with `--watch` and `--interval`.

**File**: `cmd/cli/status.go` (new file)

**FR**: FR-020, FR-021, FR-022, FR-023, FR-040–FR-043

**Usage**: `hsme-cli status [--watch] [--interval <duration>]`

**Flags**:
- `--watch` (bool) — default false
- `--interval` (duration) — default 2s

**Output**: Structured JSON or text (TTY-aware colors).

**Data to report** (per data-model.md section 3.4):
- `worker_online`: bool — detect via `/proc/*/comm` scan (Linux) or `pgrep` fallback (macOS)
- `queue`: `{total, completed, pending, processing, failed}`
- `graph`: `{nodes, edges}`
- `last_pending` (optional): `{id, task_type, memory_id, attempt_count}`

**Status queries** (from `scripts/status.sh:22-29` and `:44-45`):
```sql
SELECT
  COUNT(*) as total,
  SUM(state='completed') as completed,
  SUM(state='pending') as pending,
  SUM(state='processing') as processing,
  SUM(state='failed') as failed
FROM async_tasks
```
Graph: `SELECT COUNT(*) FROM memories; SELECT COUNT(*) FROM memory_dependencies;`

**Worker detection** (data-model.md section 4):
- On Linux: scan `/proc/*/comm` for `hsme-worker` pattern
- On macOS: `pgrep -x hsme-worker` via `os/exec`
- If undetectable: `worker_online: false` (don't error)

**Watch mode** (R6):
- Use `time.Ticker` with the interval
- Use `signal.NotifyContext` for SIGINT/SIGTERM
- Restore cursor on exit: `fmt.Printf("\033[?25h")`
- **Require TTY**: if `--watch` and not `IsTTY()`, print error "status --watch requires a terminal" and exit 1.
- Clear screen between ticks: `fmt.Print("\033[2J\033[H")`
- Exit: code 0 on clean interrupt

---

### T017 — Create `cmd/cli/admin.go`

**Purpose**: Implement `hsme-cli admin` subcommand dispatch and handlers.

**File**: `cmd/cli/admin.go` (new file)

**FR**: FR-030, FR-031, FR-032, FR-033, FR-034, FR-035, FR-036, FR-040–FR-043

**Dispatcher**: `hsme-cli admin <retry-failed|backup|restore>`

**Subcommand `retry-failed`**:
- Usage: `hsme-cli admin retry-failed [--format text|json]`
- Bootstrap: `OpenDB`
- Handler: call `admin.RetryFailed(ctx, db)` from `src/core/admin/retry.go`
- Output: `{"requeued": N, "pending": N, "failed": N}`

**Subcommand `backup`**:
- Usage: `hsme-cli admin backup [--out <path>] [--format text|json]`
- Bootstrap: `OpenDB`
- Default `--out`: `backups/engram-<UTC timestamp>.db` (data-model.md section 5)
- If `--out` not provided, create `backups/` directory if it doesn't exist
- Handler: call `admin.Backup(ctx, db, destPath)` from `src/core/admin/backup.go`
- Output: `{"backup_path": "...", "size_bytes": N}`

**Subcommand `restore`**:
- Usage: `hsme-cli admin restore (--from <path> | --latest) [--format text|json]`
- Bootstrap: `OpenDB`
- Validation: exactly one of `--from` and `--latest` must be set (data-model.md section 2.8)
- If `--latest`: glob `backups/engram-*.db`, sort by mtime descending, pick first
- Handler: call `admin.Restore(ctx, db, srcPath)` from `src/core/admin/restore.go`
- Output: `{"from": "...", "integrity_ok": true/false, "db_path": "..."}`
- Error paths (FR-036): no backup found → exit 2; integrity check fails → exit 2

**Exit codes** (FR-081, FR-082):
- Usage error (both --from and --latest set, or neither set): exit 1
- Runtime error (backup fails, restore fails): exit 2

---

## Branch Strategy

- **Planning branch**: `main`
- **Final merge target**: `main`
- **Execution worktrees**: Allocated per computed lane from `lanes.json` (resolved by `finalize-tasks`). Execute in the workspace for this lane.

---

## Definition of Done

- [ ] `hsme-cli` binary compiles with `go build -tags "sqlite_fts5 sqlite_vec" ./cmd/cli`
- [ ] `hsme-cli` with no args prints top-level help + exits 1
- [ ] `hsme-cli help` works
- [ ] `hsme-cli help store` prints store-specific usage
- [ ] `hsme-cli store --help` works
- [ ] Unknown subcommand prints help + exits 1
- [ ] `store` reads stdin, calls core, outputs result
- [ ] `search-fuzzy` calls `FuzzySearch`, outputs results
- [ ] `search-exact` calls `ExactSearch`, outputs results
- [ ] `explore` calls `TraceDependencies`, outputs result
- [ ] `status` shows worker/queue/graph info
- [ ] `status --watch` works (TTY required enforced)
- [ ] `admin retry-failed` works
- [ ] `admin backup` creates backup file
- [ ] `admin restore --latest` selects and restores latest backup
- [ ] `admin restore --from <path>` restores specified backup
- [ ] JSON format on all subcommands outputs valid JSON
- [ ] Runtime errors in JSON mode emit `{"error":"...", "code":2}` to stderr

---

## Risks & Reviewer Guidance

**Risk — stdin hang**: `store` without pipe (TTY) must detect the missing input and print a hint, not hang forever waiting. Implement the TTY check before reading stdin.

**Risk — watch without TTY**: `status --watch` must error clearly if stdout is not a TTY. Don't just silently skip — print a clear error and exit 1.

**Risk — admin restore validation**: The mutually exclusive flag check (--from XOR --latest) must happen before any bootstrap call so the usage error is clean.

**Risk — exit codes**: Every code path must set the correct exit code (0/1/2). Verify by reviewing each handler's error handling. Exit 1 = usage, Exit 2 = runtime, Exit 0 = success.

**Reviewer**: After WP02, verify each subcommand by running `hsme-cli <subcommand> --help` and checking the output format matches `contracts/output-shapes.md`. Also verify JSON errors on runtime failures.

---

## Implementation Notes

- Use `flag.NewFlagSet` per subcommand for clean flag parsing
- Positional args via `flagSet.Args()` after `flagSet.Parse()`
- `os.Stdin.ReadAll()` in Go 1.21+: `io.ReadAll(os.Stdin)`
- Time format for backup: `time.Now().UTC().Format("20060102T150405Z")` → `YYYYMMDDTHHMMSSZ`
- Create `backups/` directory with `os.MkdirAll` before backup
- Constants: exit codes as package-level vars (`const exitUsage = 1`, `const exitRuntime = 2`) for clarity

## Activity Log

- 2026-04-26T18:01:39Z – gemini:o3:implementer:implementer – shell_pid=2087465 – Started implementation via action command
- 2026-04-26T18:06:13Z – gemini:o3:implementer:implementer – shell_pid=2087465 – Implemented all CLI subcommands and dispatcher.
- 2026-04-26T18:06:18Z – gemini:o3:reviewer:reviewer – shell_pid=2093927 – Started review via action command
- 2026-04-26T18:08:16Z – gemini:o3:reviewer:reviewer – shell_pid=2093927 – Rejected: Global flag parsing bug, incomplete admin subcommands, and missing worker detection.
- 2026-04-26T18:08:22Z – gemini:o3:implementer:implementer – shell_pid=2096714 – Started implementation via action command
- 2026-04-26T18:10:33Z – gemini:o3:implementer:implementer – shell_pid=2096714 – Addressed all review feedback for WP02.
- 2026-04-26T18:10:38Z – gemini:o3:reviewer:reviewer – shell_pid=2100044 – Started review via action command
- 2026-04-26T18:10:48Z – gemini:o3:reviewer:reviewer – shell_pid=2100044 – Review passed: Flag parsing, worker detection, and admin stubs/core functions correctly implemented.
