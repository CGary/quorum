# Implementation Plan: HSME Unified CLI

**Mission ID**: `01KQ59MVCJZ7EPETGSGG04VR54`
**Mission Slug**: `hsme-unified-cli-01KQ59MV`
**Branch**: `main` | **Date**: 2026-04-26 | **Spec**: [spec.md](spec.md)
**Input**: [Feature specification](spec.md)

## Summary

Build a single `hsme-cli` binary that exposes the four MCP tools (`store_context`, `search_fuzzy`, `search_exact`, `explore_knowledge_graph`), the status dashboard (`status` with `--watch`), three admin operations (`retry-failed`, `backup`, `restore`) under an `admin` namespace, and a `help` subcommand — all powered by a new shared `bootstrap` package consumed by the four binaries in the repo (`hsme`, `hsme-worker`, `hsme-ops`, `hsme-cli`).

Engineering approach: stdlib `flag` for parsing with manual subcommand dispatch, lazy embedder initialization for subcommands that don't need it, ANSI-aware text/JSON output, SQLite Online Backup API for `admin backup`, atomic `rename`-based restore with WAL/SHM cleanup. No new third-party dependencies.

## Technical Context

| Field | Value |
|-------|-------|
| **Language/Version** | Go 1.26.2 (per `go.mod`) |
| **Primary Dependencies** | `github.com/mattn/go-sqlite3 v1.14.42`, `github.com/asg017/sqlite-vec-go-bindings v0.1.6`, `golang.org/x/text v0.36.0` (no new modules added) |
| **Storage** | SQLite with WAL mode, FTS5, sqlite-vec extensions (build tags `sqlite_fts5 sqlite_vec`) |
| **Testing** | Go `testing` package with table-driven tests; integration tests against ephemeral DBs in `tests/modules/`; build with `-tags "sqlite_fts5 sqlite_vec"` |
| **Target Platform** | Linux/macOS terminal; Windows out of scope |
| **Project Type** | Single binary CLI + reusable Go package (no client/server split) |
| **Performance Goals** | `status` p95 <500 ms on 1k-10k corpus; `admin backup` zero-corruption under concurrent reads |
| **Constraints** | No new third-party deps; binary ≤30 MB stripped; existing test suite must remain green |
| **Scale/Scope** | 9 subcommands, 1 new package, 4 binaries touched, ~1500 LoC estimated |
| **Module Path** | `github.com/hsme/core` |

## Charter Check

**Status**: SKIPPED — no charter file exists at `.kittify/charter/charter.md`.

No charter gates to evaluate. Architectural alignment is governed by:

- Existing code conventions (stdlib-first, build tags, package layout under `src/`)
- The mission's own constraints (`C-001` through `C-006`)
- The Engineering Alignment captured in `research.md`

## Engineering Alignment (decisions cristalizadas)

| Decision area | Resolution | Source |
|--------------|-----------|--------|
| **Restore safety** | Just-do-it. SQLite Backup API for atomic write or `rename`-based replacement; cleanup of `-wal`/`-shm` sidecars; no `pgrep` / lock probing; operator responsibility documented | User directive 2026-04-26 |
| **Backup implementation** | SQLite Online Backup API via `*sqlite3.SQLiteConn.Backup()` from `mattn/go-sqlite3 v1.14.42` (verified available) | Driver inspection 2026-04-26 |
| **JSON output shape** | Mirrors the data shape of MCP handler results without the `{"content":[{"type":"text",…}]}` wrapper | `cmd/hsme/main.go:20-44` reference |
| **Status `--watch`** | ANSI clear-screen + redraw on each tick; if stdout is not a TTY, fail with clear error (watching a non-TTY is meaningless) | Standard CLI pattern |
| **Bootstrap signatures** | `bootstrap.OpenDB(cfg) (*sql.DB, error)` and `bootstrap.OpenWithEmbedder(cfg) (*sql.DB, *ollama.Embedder, error)`. `Config` struct with DB path, Ollama host, embedding model, plus a method `LoadFromEnv()` that consumes `SQLITE_DB_PATH`/`OLLAMA_HOST`/`EMBEDDING_MODEL` | Spec FR-060/FR-061/FR-062 |
| **CLI parser** | Stdlib `flag` with manual subcommand dispatch by `os.Args[1]`. No `cobra`/`urfave-cli` | Spec NFR-001 + idea file recommendation |
| **Test boundary** | Table-driven tests for parsing/dispatch; integration tests against temp DB seeded by `tests/testutil` patterns; one tag-gated suite for `admin backup`/`admin restore` round-trip | Repo convention |
| **Error contract for `--format=json`** | `{"error":"<msg>","code":<exit_code>}` written to stderr; preserved exit code per FR-080..FR-082 | Spec FR-043 |

## Project Structure

### Documentation (this feature)

```
kitty-specs/hsme-unified-cli-01KQ59MV/
├── plan.md              # This file (/spec-kitty.plan output)
├── spec.md              # /spec-kitty.specify output
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output (input/output structs for subcommands)
├── quickstart.md        # Phase 1 output (operator-facing usage walkthrough)
├── contracts/
│   ├── exit-codes.md    # FR-080..082 codified
│   ├── output-shapes.md # JSON shape per subcommand
│   └── flags.md         # Flags per subcommand
├── checklists/
│   └── requirements.md  # /spec-kitty.specify quality gate (already passing)
└── tasks/               # /spec-kitty.tasks output (NOT created here)
```

### Source Code (repository root)

**New paths added by this mission:**

```
cmd/cli/
├── main.go              # Entry point, subcommand dispatcher
├── store.go             # `hsme-cli store` handler
├── search.go            # `hsme-cli search-fuzzy` and `search-exact`
├── explore.go           # `hsme-cli explore`
├── status.go            # `hsme-cli status [--watch]`
├── admin.go             # `hsme-cli admin <retry-failed|backup|restore>`
├── help.go              # `hsme-cli help [subcommand]`
├── output.go            # Text/JSON formatting + TTY detection
├── flags.go             # Shared flag definitions
└── *_test.go            # Subcommand and parsing tests

src/bootstrap/
├── bootstrap.go         # OpenDB, OpenWithEmbedder, Config struct
├── config.go            # LoadFromEnv, defaults
└── bootstrap_test.go    # Init verification tests

src/core/admin/          # Optional new package for admin operations logic
├── retry.go             # Re-queue failed tasks (ported from justfile inline SQL)
├── backup.go            # SQLite Online Backup wrapper
├── restore.go           # Atomic restore with integrity check + WAL/SHM cleanup
└── admin_test.go        # Integration tests against temp DB

tests/modules/
└── cli_test.go          # End-to-end CLI integration tests
```

**Existing paths modified by this mission:**

```
cmd/hsme/main.go                    # Replace inline init with bootstrap.OpenWithEmbedder
cmd/worker/main.go                  # Replace inline init with bootstrap.OpenWithEmbedder
cmd/ops/main.go                     # Replace inline init with bootstrap.OpenWithEmbedder (or OpenDB if no embedder needed — TBD per WP)
justfile                            # Add cli-build, cli-install; reduce status/backup/restore/retry-failed to wrappers (or remove)
scripts/status.sh                   # Deleted after parity verification
ideas/cli-tool.md                   # Deleted after spec/plan supersede
```

**Structure Decision**: Single Go module, single repo. Two new packages (`cmd/cli`, `src/bootstrap`) and an optional third (`src/core/admin`) for the admin operations logic. Existing binaries are touched only in their entrypoint init code; their domain logic is unaffected.

The `src/core/admin/` package separation keeps `cmd/cli/admin.go` thin (parsing + invocation) and lets the admin logic be tested in isolation against a temporary DB. If during implementation the admin logic turns out to be small enough that a separate package adds noise, it can be inlined into `cmd/cli/admin.go` as a tactical choice without changing the spec.

## Phase 0 Output

See [research.md](research.md) for:

- Subcommand-to-core-function mapping confirmation
- SQLite Online Backup API verification
- TTY/ANSI detection approach
- Lazy embedder initialization pattern
- Atomic restore implementation strategy
- Watch-loop signal handling

## Phase 1 Output

- [data-model.md](data-model.md) — input parameter structs and result struct schemas for each subcommand
- [contracts/exit-codes.md](contracts/exit-codes.md) — exit code matrix (success / usage error / runtime error)
- [contracts/output-shapes.md](contracts/output-shapes.md) — JSON output shape per subcommand
- [contracts/flags.md](contracts/flags.md) — flag/arg list per subcommand
- [quickstart.md](quickstart.md) — operator-facing walkthrough of all 9 subcommands

## Complexity Tracking

| Topic | Decision | Rationale |
|-------|----------|-----------|
| New `src/core/admin/` package | Allowed | Keeps admin logic testable independently from CLI parsing; mirrors the pattern of other core packages (`indexer`, `search`, `worker`). If during WP execution the package proves too thin, it can be folded into `cmd/cli/admin.go` — that's an implementation tactical decision, not a plan deviation. |
| `bootstrap` package separation | Required | FR-060 mandates shared init; without a package the duplication just relocates. The package is small (≤2 functions + Config struct) and pays for itself across 4 binaries. |
| No new external deps | Hard constraint | NFR-001. `cobra`/`urfave-cli` would be ergonomic but 9 subcommands don't justify ~50KB of binary or new module dependency. |

## Re-evaluation of Charter Check (post-design)

**Status**: SKIPPED — no charter exists.

No new gate violations introduced by Phase 1 design. Plan ready for `/spec-kitty.tasks`.

## Branch Contract (final)

- **Current branch**: `main`
- **Planning/base branch**: `main`
- **Final merge target**: `main`
- **branch_matches_target**: `true`
- **Branch strategy summary**: Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main.

The mission is ready to be broken down into Work Packages via `/spec-kitty.tasks`.
