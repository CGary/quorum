---
work_package_id: WP01
title: Shared Bootstrap Package
dependencies: []
requirement_refs:
- FR-050
- FR-051
- FR-052
- FR-053
- FR-060
- FR-061
- FR-062
- FR-063
- FR-070
- FR-071
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-hsme-unified-cli-01KQ59MV
base_commit: 228508ffe78f1a7d869e98d27ccbfd4294d6fdfe
created_at: '2026-04-26T17:32:50.641751+00:00'
subtasks:
- T001
- T002
- T003
- T004
- T005
- T006
- T007
- T008
agent: "gemini:o3:reviewer:reviewer"
shell_pid: "2086800"
history:
- date: '2026-04-26T16:47:42Z'
  action: tasks generated
  actor: tasks skill
agent_profile: ''
authoritative_surface: src/bootstrap/
execution_mode: code_change
model: ''
owned_files:
- src/bootstrap/bootstrap.go
- src/bootstrap/config.go
- src/bootstrap/bootstrap_test.go
- cmd/hsme/main.go
- cmd/worker/main.go
- cmd/ops/main.go
role: ''
tags: []
---

## ⚡ Do This First: Load Agent Profile

Before reading anything else, load the implementer agent profile:

```
/ad-hoc-profile-load implementer
```

This injects your role identity, skill directives, and execution context. All other instructions in this prompt are subordinate to the profile load.

---

## Objective

Create the `src/bootstrap/` shared initialization package consumed by all four binaries (`hsme`, `hsme-worker`, `hsme-ops`, `hsme-cli`), and refactor the existing three binaries to use it. Also add `cli-build` and `cli-install` justfile targets.

---

## Context

### Feature directory

`/home/gary/dev/hsme/kitty-specs/hsme-unified-cli-01KQ59MV`

### Key design documents (read before implementing)

- **plan.md** — architecture overview, decisions, project structure
- **data-model.md** — Config struct signature, bootstrap function signatures
- **research.md** — R5 (lazy embedder), R10 (decay config loading), all decisions codified

### What this WP produces

```
src/bootstrap/
├── bootstrap.go       # OpenDB, OpenWithEmbedder
├── config.go          # Config struct + LoadFromEnv
└── bootstrap_test.go  # Init verification tests

cmd/hsme/main.go       # refactored to use bootstrap
cmd/worker/main.go     # refactored to use bootstrap
cmd/ops/main.go        # refactored to use bootstrap

justfile               # cli-build + cli-install targets added
```

### What this WP does NOT produce

- `cmd/cli/` directory (WP02)
- Any admin logic (WP04)
- Tests beyond bootstrap package tests

---

## Guidance per Subtask

### T001 — Create `src/bootstrap/bootstrap.go`

**Purpose**: Implement two initialization functions — one for DB-only, one for DB+embedder.

**File**: `src/bootstrap/bootstrap.go` (new file)

**Signature** (from data-model.md):

```go
func OpenDB(cfg Config) (*sql.DB, error)
func OpenWithEmbedder(cfg Config) (*sql.DB, *ollama.Embedder, error)
```

**Behavior — OpenDB**:
1. Open SQLite DB at `cfg.DBPath` with build tags `sqlite_fts5 sqlite_vec` (same as existing binaries)
2. Apply `search.LoadDecayConfig()` — this is the only place decay config is loaded (R10)
3. Return `*sql.DB`

**Behavior — OpenWithEmbedder**:
1. Call `OpenDB(cfg)` to get the DB
2. Create embedder: `ollama.NewEmbedder(cfg.OllamaHost, cfg.EmbeddingModel, cfg.EmbeddingDim)`
3. Validate: `indexer.ValidateEmbeddingConfig(db, embedder)` — must pass or return error (C-005)
4. Return DB + embedder

**Error handling**: All errors wrapped with context (e.g., `fmt.Errorf("bootstrap open: %w", err)`).

**Import paths**: Use `github.com/hsme/core/src/core/search` and `github.com/hsme/core/src/core/indexer` as appropriate. Verify exact import paths from existing code in `cmd/hsme/main.go`.

**Build tags**: This file needs `//go:build sqlite_fts5 sqlite_vec` at the top since it opens the DB.

---

### T002 — Create `src/bootstrap/config.go`

**Purpose**: Define the `Config` struct and `LoadFromEnv()` method (from data-model.md section 1).

**File**: `src/bootstrap/config.go` (new file)

**Signature** (from data-model.md):

```go
type Config struct {
    DBPath         string  // SQLITE_DB_PATH or --db; default "data/engram.db"
    OllamaHost     string  // OLLAMA_HOST or --ollama-host; default "" → driver picks default
    EmbeddingModel string  // EMBEDDING_MODEL or --embedding-model; default "nomic-embed-text"
    EmbeddingDim   int     // hard-coded to 768 (matches existing code)
}

func LoadFromEnv() Config  // reads env, applies defaults, no flag awareness
func (c *Config) ApplyFlagOverrides(flags *flag.FlagSet)  // overlays --db, --ollama-host, --embedding-model
```

**Defaults** (apply in `LoadFromEnv()`):
- `DBPath = "data/engram.db"` if `SQLITE_DB_PATH` not set
- `OllamaHost = ""` (empty string) if `OLLAMA_HOST` not set
- `EmbeddingModel = "nomic-embed-text"` if `EMBEDDING_MODEL` not set
- `EmbeddingDim = 768` (constant, never changed)

**Flag overlay** (`ApplyFlagOverrides`): Called after env is loaded. Takes a `*flag.FlagSet` and overlays fields if flags were set on the command line. Use `flag.Lookup("db")`, `flag.Lookup("ollama-host")`, etc. Only override if the flag value is not the zero value (empty string for these).

**No build tags needed** — this file is pure config, no DB access.

---

### T003 — Move decay config loading into `OpenDB`

**Purpose**: Ensure decay config is loaded exactly once, in the bootstrap package (R10 from research.md).

**Impact**: Currently `cmd/hsme/main.go:101-106` loads decay config inline. After this, `OpenDB` loads it.

**Verify**: Search for `LoadDecayConfig` across all `cmd/*/main.go` files and confirm current usages. There should be 3-4 usages across `hsme`, `worker`, `ops`. Each will be replaced by a single `OpenDB` call that handles decay automatically.

**Constraint**: Do NOT change behavior — only relocate where decay config is loaded. The global `search.GlobalDecayConfig` must be set before any search operation runs.

---

### T004 — Create `src/bootstrap/bootstrap_test.go`

**Purpose**: Test that `OpenDB` and `OpenWithEmbedder` work correctly.

**File**: `src/bootstrap/bootstrap_test.go` (new file)

**Tests to write**:
1. `LoadFromEnv()` returns a valid Config with defaults applied
2. `LoadFromEnv()` reads env vars correctly
3. `OpenDB` with a temp DB returns a valid `*sql.DB`
4. `OpenDB` fails with clear error when DB path is invalid
5. `OpenWithEmbedder` fails with clear error when Ollama is unreachable (skip if no Ollama available in test env — use a build tag `//go:build ignore` pattern or check `OLLAMA_HOST` before running embedder test)

**Build tags**: `//go:build sqlite_fts5 sqlite_vec` since `OpenDB` opens a DB.

**Use `os.MkdirTemp`**: Create a temp directory for the DB path in tests. Clean up with `defer os.RemoveAll`.

**Import existing test util**: Check if `tests/testutil/` has DB seeding helpers; if so, reuse them.

---

### T005 — Wire `cmd/hsme/main.go`

**Purpose**: Replace the inline init in `cmd/hsme/main.go` with a call to `bootstrap.OpenWithEmbedder`.

**File**: `cmd/hsme/main.go` (modify)

**Current pattern** (grep for this):
```go
db, embedder, err := initFn(...)  // where initFn varies by binary
```

**Replace with**:
```go
cfg := bootstrap.LoadFromEnv()
cfg.ApplyFlagOverrides(flag.CommandLine)
db, embedder, err := bootstrap.OpenWithEmbedder(cfg)
if err != nil {
    // existing error handling
}
```

**What to keep**: All MCP handler registration, the HTTP server startup, signal handling — everything after init stays the same.

**What to remove**: Inline DB init code, inline embedder creation, inline decay config loading. These all move into `bootstrap.OpenDB` / `OpenWithEmbedder`.

**Check**: Read the current `cmd/hsme/main.go` to understand what init code exists before modifying.

---

### T006 — Wire `cmd/worker/main.go`

**Purpose**: Same as T005, for `cmd/worker/main.go`.

**File**: `cmd/worker/main.go` (modify)

**Same pattern**: Replace inline init with `bootstrap.OpenWithEmbedder`.

**Verify**: Check if `worker` uses embedder (it likely does for processing tasks). If in doubt, check what `worker` does with the embedder — if it just passes it to search/indexer, `OpenWithEmbedder` is correct.

---

### T007 — Wire `cmd/ops/main.go`

**Purpose**: Same as T005, for `cmd/ops/main.go`.

**File**: `cmd/ops/main.go` (modify)

**DB-only vs with embedder**: Verify what `ops` actually uses. If it only runs `admin` operations (backup/restore/retry) and status queries, it may not need the embedder. In that case, use `OpenDB` instead of `OpenWithEmbedder`.

**Decision process**:
1. Read `cmd/ops/main.go` to find embedder usage
2. If embedder is used (passed to any function), use `OpenWithEmbedder`
3. If embedder is NOT used, use `OpenDB` (saves startup time and removes Ollama dependency for `ops`)

---

### T008 — Add `cli-build` and `cli-install` justfile targets

**File**: `justfile` (modify)

**Add these targets**:

```just
# Build hsme-cli binary with required build tags
cli-build:
    go build -tags "sqlite_fts5 sqlite_vec" -o bin/hsme-cli ./cmd/cli

# Install hsme-cli to $INSTALL_PATH (default ~/go/bin)
cli-install: cli-build
    mkdir -p ${INSTALL_PATH:-$HOME/go/bin}
    cp bin/hsme-cli ${INSTALL_PATH:-$HOME/go/bin}/hsme-cli

# Update default install to also include CLI
install: cli-install
```

**Check existing `install` target**: Verify the current `install` target structure. The `install` target should call `cli-install` as part of its flow so that `hsme`, `hsme-worker`, `hsme-ops`, AND `hsme-cli` are all installed together.

**Build output**: `bin/hsme-cli` (mkdir -p bin/ if needed).

**Build tags**: `sqlite_fts5 sqlite_vec` are required (C-001).

---

## Branch Strategy

- **Planning branch**: `main`
- **Final merge target**: `main`
- **Execution worktrees**: Allocated per computed lane from `lanes.json` (resolved by `finalize-tasks`). After `finalize-tasks` runs, each WP belongs to a lane with a specific workspace path and branch. Execute in the workspace for this lane.

---

## Definition of Done

- [ ] `src/bootstrap/bootstrap.go` — `OpenDB` and `OpenWithEmbedder` both implemented and exportable
- [ ] `src/bootstrap/config.go` — `Config` struct, `LoadFromEnv()`, `ApplyFlagOverrides()` all working
- [ ] T003: Decay config loaded in `OpenDB`, not in individual binaries
- [ ] `src/bootstrap/bootstrap_test.go` — tests pass
- [ ] `cmd/hsme/main.go` — compiles and runs identically after refactor
- [ ] `cmd/worker/main.go` — compiles and runs identically after refactor
- [ ] `cmd/ops/main.go` — compiles and runs identically after refactor
- [ ] `justfile` — `cli-build` and `cli-install` targets work; `install` includes CLI
- [ ] `go test ./src/bootstrap/...` passes with tags
- [ ] No regression in existing binary behavior (quick smoke test after each refactor)

---

## Risks & Reviewer Guidance

**Risk — Embedder validation timing**: The `ValidateEmbeddingConfig` check (C-005) must run at startup whenever embedder is initialized. If `OpenWithEmbedder` skips this, the constraint is violated. Verify the call is present in `OpenWithEmbedder`.

**Risk — Decay config regression**: If T003 is not implemented carefully, decay config could be loaded twice (once in `OpenDB`, once in existing binary init), causing confusing behavior. Ensure each binary's inline decay loading is REMOVED when refactored.

**Risk — `ops` binary embedder usage**: If `ops` does need the embedder and we use `OpenDB`, it may fail at runtime when embedder is nil but a function expects it. Verify by reading `cmd/ops/main.go` before deciding which bootstrap function to use.

**Reviewer**: After WP01 lands, run `hsme-cli --help` (once WP02 exists) and confirm the binary uses the shared bootstrap. Also confirm `go test ./src/bootstrap/...` passes.

---

## Implementation Notes

- Use `fmt.Errorf("bootstrap: %w", err)` pattern for error wrapping
- Keep the package small — only initialization, no business logic
- The `Config.ApplyFlagOverrides` method is used by CLI (WP02) and the refactored binaries alike
- Build tags `sqlite_fts5 sqlite_vec` are required on any file that calls `OpenDB` since it opens a SQLite connection

## Activity Log

- 2026-04-26T17:32:51Z – gemini:o3:implementer:implementer – shell_pid=2050031 – Assigned agent via action command
- 2026-04-26T17:38:48Z – gemini:o3:implementer:implementer – shell_pid=2050031 – Implemented shared bootstrap and refactored binaries
- 2026-04-26T17:38:53Z – gemini:o3:reviewer:reviewer – shell_pid=2058459 – Started review via action command
- 2026-04-26T17:41:25Z – gemini:o3:reviewer:reviewer – shell_pid=2058459 – Rejected: Issues with flag overrides, repository pollution, and build failures.
- 2026-04-26T17:41:34Z – gemini:o3:implementer:implementer – shell_pid=2062360 – Started implementation via action command
- 2026-04-26T17:44:20Z – gemini:o3:implementer:implementer – shell_pid=2062360 – Addressed all review feedback points.
- 2026-04-26T17:44:25Z – gemini:o3:reviewer:reviewer – shell_pid=2066529 – Started review via action command
- 2026-04-26T18:00:49Z – gemini:o3:reviewer:reviewer – shell_pid=2066529 – Rejected: Binaries still tracked.
- 2026-04-26T18:01:05Z – gemini:o3:reviewer:reviewer – shell_pid=2066529 – Removed all untracked files and moving to review.
- 2026-04-26T18:01:10Z – gemini:o3:reviewer:reviewer – shell_pid=2086800 – Started review via action command
- 2026-04-26T18:01:29Z – gemini:o3:reviewer:reviewer – shell_pid=2086800 – Review passed: All previous issues addressed. Bootstrap package and binary refactoring look solid. No binaries tracked in git.
