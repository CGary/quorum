# Research: HSME Unified CLI

**Mission**: `hsme-unified-cli-01KQ59MV`
**Phase**: 0 (Outline & Research)
**Date**: 2026-04-26

## Scope

This document resolves every technical unknown left open by `spec.md`'s Assumptions section, plus the architectural questions that surfaced during planning. Each entry follows the format **Decision · Rationale · Alternatives**.

## R1 — Subcommand-to-core-function mapping

**Decision**: The four MCP-tool subcommands invoke the existing core functions directly without any wrapping or transformation:

| Subcommand | Core function | Source file |
|-----------|--------------|-------------|
| `store` | `indexer.StoreContext(db, content, sourceType, project, supersedesID, forceReingest)` | `src/core/indexer/ingest.go` |
| `search-fuzzy` | `search.FuzzySearch(ctx, db, embedder, query, limit, project)` | `src/core/search/fuzzy.go` |
| `search-exact` | `search.ExactSearch(ctx, db, keyword, limit, project)` | `src/core/search/fuzzy.go` |
| `explore` | `indexer.CanonicalizeName(name)` then `search.TraceDependencies(ctx, db, canonical, direction, maxDepth, maxNodes)` | `src/core/search/graph.go`, `src/core/indexer/normalize.go` |

**Rationale**: Verified at `cmd/hsme/main.go:122-256` — the MCP server already invokes these signatures with the exact same parameter shapes. The CLI is a parallel transport, not a different consumer; using identical call sites guarantees behavior parity.

**Note**: The `project` parameter is present on all MCP-tool calls. The CLI must expose it as a flag (`--project`) on every relevant subcommand to preserve full feature parity. This is captured in the data-model and contract artifacts.

**Alternatives**:
- Adding a thin facade per subcommand was considered but rejected — it would just duplicate the MCP wrapping pattern and create drift risk.

## R2 — SQLite Online Backup API availability

**Decision**: Use `*sqlite3.SQLiteConn.Backup(destName string, srcDB *SQLiteConn, srcName string) (*SQLiteBackup, error)` from `mattn/go-sqlite3`. Workflow:

1. Open a target backup connection on the destination file path.
2. Acquire `*sqlite3.SQLiteConn` from both via `db.Conn(ctx).Raw(func(driverConn) error { ... })`.
3. Call `srcConn.Backup("main", destConn, "main")` to get a `*SQLiteBackup` handle.
4. Loop `Step(N)` until `Done`, then `Finish()`.
5. Close both connections.

**Rationale**:
- `mattn/go-sqlite3 v1.14.42` (`go.mod` line 7) exposes this API publicly. It's the WAL-safe canonical path documented by SQLite upstream.
- The justfile currently uses `sqlite3 .backup` (CLI shell-out). Going through the driver eliminates the need for the `sqlite3` CLI to be installed and gives us typed error handling.

**Alternatives**:
- File copy under `BEGIN IMMEDIATE` — was the spec's documented fallback. Rejected because the API is available; the fallback adds complexity without benefit.
- `sqlite3` shell-out — rejected; defeats the purpose of the migration (NFR-001 stdlib-first, portability gain).

## R3 — Atomic restore strategy

**Decision**: Multi-step restore with safety:

1. Resolve source: either `--from <path>` (explicit) or scan `BACKUP_DIR` (default `backups/`) and pick the most recent `engram*.db` by mtime when `--latest`.
2. Open source DB read-only, run `PRAGMA integrity_check`. Abort if not `ok`.
3. Compute target path (`SQLITE_DB_PATH` or `--db <path>`).
4. Remove WAL/SHM sidecars: `os.Remove(target+"-wal")`, `os.Remove(target+"-shm")` (ignore `os.ErrNotExist`).
5. Atomic replace via `os.Rename(tmpCopy, target)`. The temp copy is created in the same directory as `target` (same filesystem) so `rename(2)` is atomic.
6. Use `io.Copy` with `os.O_EXCL|O_CREATE|O_WRONLY` for the temp file to avoid clobbering.

**Rationale**:
- `os.Rename` is atomic on POSIX when source and target are on the same filesystem. Putting the temp file alongside the target guarantees this.
- Cleaning sidecars ensures a previous instance's stale WAL doesn't get applied on top of the restored DB and corrupt it.
- Integrity check before commit means a corrupt backup never overwrites the live DB.

**Alternatives**:
- Backup API for restore (i.e. open target, restore from source) — rejected because target may currently be open by other processes (per the user's "just-do-it" directive we don't pre-validate); whole-file replacement is simpler and more bulletproof.
- Mutex/lockfile coordination — rejected by user directive (`research.md` Engineering Alignment).

## R4 — TTY and ANSI detection

**Decision**: Use stdlib only. Detect TTY via `os.File.Stat()` checking `Mode()&os.ModeCharDevice != 0` on `os.Stdout`. Apply ANSI codes only when:

- Output is a TTY, **and**
- `--no-color` flag is not set, **and**
- `NO_COLOR` env var is not set (de facto standard, https://no-color.org/).

For the `status --watch` subcommand, also require TTY; emit a clear error if the user pipes `--watch` somewhere.

**Rationale**:
- The existing `scripts/status.sh` already uses raw ANSI codes (`\033[32m` etc.) without TTY detection, which produces garbled output when piped. The new CLI improves on this.
- Stdlib-only honors NFR-001.

**Alternatives**:
- `golang.org/x/term` (already in indirect tree?) — verified NOT in `go.mod`; introducing it would violate NFR-001 for a one-call-site need.
- `mattn/go-isatty` — same reason rejected.

## R5 — Lazy embedder initialization

**Decision**: The `bootstrap` package exposes two distinct functions instead of a single function with options:

```go
// Returns DB only. Cheap; no Ollama dependency.
func OpenDB(cfg Config) (*sql.DB, error)

// Returns DB + validated embedder. Calls ValidateEmbeddingConfig.
func OpenWithEmbedder(cfg Config) (*sql.DB, *ollama.Embedder, error)
```

The CLI dispatcher decides which to call based on the subcommand:

| Subcommand | Bootstrap call |
|-----------|----------------|
| `help` | none (pure function) |
| `status`, `search-exact`, `explore`, `admin/*` | `OpenDB` |
| `store`, `search-fuzzy` | `OpenWithEmbedder` |

**Rationale**:
- Two functions are clearer than one with optional behavior — the contract is read straight from the call site.
- `search-fuzzy` requires the embedder for the query embedding; nothing else does.
- `store` requires the embedder because `indexer.StoreContext` enqueues processing that depends on the active embedding model being valid (the `ValidateEmbeddingConfig` check protects against schema drift).

**Alternatives**:
- Single `Open(cfg, opts...)` with functional options — rejected; not idiomatic for a package this small.
- Interface with `EmbedderProvider` lazy hook — rejected; over-engineered for two binaries' need.

## R6 — Watch-loop signal handling

**Decision**: `status --watch` runs a `time.Ticker` in the main goroutine. SIGINT / SIGTERM are wired through `signal.NotifyContext`; on signal, the loop exits cleanly, restores the cursor (`\033[?25h`), and returns exit code `0`.

**Rationale**:
- Operators expect Ctrl-C to stop a watch and not leave the terminal in a broken state.
- `signal.NotifyContext` (Go 1.16+) is the idiomatic stdlib path.

**Alternatives**:
- Raw `signal.Notify` channel — works but uglier than `NotifyContext`.

## R7 — Output format contract for errors

**Decision**:

- **Text format**: errors written to stderr as plain text, prefixed with `error: `.
- **JSON format**: errors written to stderr as `{"error": "<message>", "code": <exit_code>}`, where `code` is the same exit code returned to the OS.
- Exit code preserved per FR-080..082 in both formats.

**Rationale**:
- Scripting consumers parse JSON from stderr only when they explicitly opted into `--format=json`. They get structured errors there for free.
- Plain text consumers get human-readable errors.

**Alternatives**:
- All errors to stdout in JSON mode — rejected; muddles result vs. error streams for downstream `jq | grep` pipelines.

## R8 — Justfile cleanup pattern

**Decision**: Replace each affected target with a one-line wrapper that invokes the new CLI:

```just
status:        ; @./hsme-cli status
backup:        ; @./hsme-cli admin backup
restore:       ; @./hsme-cli admin restore --latest
retry-failed:  ; @./hsme-cli admin retry-failed
```

Then delete `scripts/status.sh` (parity verified during the WP that lands `status`). The `cli-build` and `cli-install` targets are added; `install` is updated to also install the CLI binary.

**Rationale**:
- Wrapping (not removing) preserves muscle memory for operators who type `just status`.
- The wrappers are pure delegation, so there's no risk of drift.

**Alternatives**:
- Remove targets entirely — rejected; users typing `just status` would just hit "target not found" with no signpost to the new CLI.

## R9 — Test boundary

**Decision**: Three test layers, all using build tags `sqlite_fts5 sqlite_vec`:

1. **Unit (`cmd/cli/*_test.go`)**: table-driven tests for flag parsing, subcommand routing, format selection, error messages. No DB.
2. **Integration (`tests/modules/cli_test.go`)**: end-to-end invocation of subcommands against an ephemeral DB seeded by existing test util. Covers the `store → search-fuzzy → search-exact → explore` golden path.
3. **Admin operations (`src/core/admin/*_test.go` or `tests/modules/admin_test.go`)**: round-trip tests for backup → restore against a temp DB; integrity-check failure path; missing-backup path. Tag-gated to skip in CI when the SQLite extensions aren't available, matching repo convention.

**Rationale**:
- Repo already uses this layering. No reason to invent a new pattern.

**Alternatives**:
- Mock SQLite — rejected; admin operations exercise WAL/SHM behavior that mocks cannot reproduce.

## R10 — Decay config loading in bootstrap

**Decision**: Move `search.LoadDecayConfig()` + `search.GlobalDecayConfig = decayCfg` out of each binary's `main` and into `bootstrap.OpenDB`. This means every binary that opens the DB gets decay config applied at startup automatically.

**Rationale**:
- Currently `cmd/hsme/main.go:101-106` does this; if the CLI's `search-fuzzy` and `search-exact` subcommands skip this load, they'd silently behave differently from the MCP server (decay always off, no env override). That's a behavioral regression.
- Centralizing in bootstrap means future config additions don't need to be re-applied to every `main.go`.

**Alternatives**:
- Per-binary application — rejected for the regression risk above.

## Open Questions

None remaining. All Phase 0 unknowns resolved.

## Summary

Every decision lands on stdlib-only, leverages existing repo patterns, and avoids new third-party dependencies. The biggest implementation risk was the Online Backup API availability — that has been verified at the dependency level. The plan is ready to proceed to data-model and contracts.
