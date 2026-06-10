# Tasks: HSME Unified CLI

**Mission**: `hsme-unified-cli-01KQ59MV`
**Date**: 2026-04-26
**NOW_UTC_ISO**: 2026-04-26T16:47:42Z
**Planning branch**: `main`
**Merge target**: `main`
**Branch matches target**: true

## Subtask Index

| ID | Description | WP | Parallel |
|----|-------------|----|----------|
| T001 | Create `src/bootstrap/bootstrap.go` with `OpenDB` and `OpenWithEmbedder` functions | WP01 |  | [D] |
| T002 | Create `src/bootstrap/config.go` with `Config` struct and `LoadFromEnv()` | WP01 |  | [D] |
| T003 | Move decay config loading into `OpenDB` | WP01 |  | [D] |
| T004 | Create `src/bootstrap/bootstrap_test.go` with init verification tests | WP01 |  | [D] |
| T005 | Wire `cmd/hsme/main.go` to use `bootstrap.OpenWithEmbedder` | WP01 |  | [D] |
| T006 | Wire `cmd/worker/main.go` to use `bootstrap.OpenWithEmbedder` | WP01 |  | [D] |
| T007 | Wire `cmd/ops/main.go` to use bootstrap (DB-only or with embedder, TBD per WP) | WP01 |  | [D] |
| T008 | Add `cli-build` and `cli-install` justfile targets | WP01 |  | [D] |
| T009 | Create `cmd/cli/output.go` — TTY detection, text/JSON formatting, ANSI colors | WP02 | [D] |
| T010 | Create `cmd/cli/flags.go` — shared flag definitions (--db, --ollama-host, --embedding-model, --format) | WP02 | [D] |
| T011 | Create `cmd/cli/help.go` — top-level help and per-subcommand help | WP02 | [D] |
| T012 | Create `cmd/cli/main.go` — entry point, subcommand dispatcher, --help handling | WP02 |  | [D] |
| T013 | Create `cmd/cli/store.go` — `hsme-cli store` with stdin read, --source-type, --supersedes, --force-reingest | WP02 |  | [D] |
| T014 | Create `cmd/cli/search.go` — `hsme-cli search-fuzzy` and `hsme-cli search-exact` handlers | WP02 |  | [D] |
| T015 | Create `cmd/cli/explore.go` — `hsme-cli explore` handler | WP02 |  | [D] |
| T016 | Create `cmd/cli/status.go` — `hsme-cli status` with --watch, --interval, worker detection | WP02 |  | [D] |
| T017 | Create `cmd/cli/admin.go` — `hsme-cli admin retry-failed`, `backup`, `restore` dispatch | WP02 |  | [D] |
| T018 | Write `cmd/cli/*_test.go` — table-driven unit tests for flag parsing, dispatch, format selection, error messages | WP03 |  | [D] |
| T019 | Write `tests/modules/cli_test.go` — end-to-end CLI integration tests against ephemeral DB | WP03 |  | [D] |
| T020 | Create `src/core/admin/retry.go` — re-queue failed/exhausted tasks | WP04 |  | [D] |
| T021 | Create `src/core/admin/backup.go` — SQLite Online Backup API wrapper | WP04 |  | [D] |
| T022 | Create `src/core/admin/restore.go` — atomic restore with integrity check + WAL/SHM cleanup | WP04 |  | [D] |
| T023 | Write `src/core/admin/admin_test.go` — integration tests for backup/restore round-trip, integrity failure, missing backup | WP04 |  | [D] |
| T024 | Refactor `justfile` status/backup/restore/retry-failed to one-line wrappers or remove | WP05 |  | [D] |
| T025 | Remove `scripts/status.sh` after parity verification | WP05 |  | [D] |
| T026 | Remove `ideas/cli-tool.md` after spec/plan supersede | WP05 |  | [D] |
| T027 | Run full test suite (`just test`) and verify zero regressions | WP06 |  | [D] |
| T028 | Verify restore refuses corrupt backup (100% of cases) | WP06 |  | [D] |
| T029 | Verify operator daily ops loop works end-to-end without bash | WP06 |  | [D] |

---

## WP01 — Shared Bootstrap Package

**Goal**: Create `src/bootstrap/` package consumed by all four binaries; refactor existing binaries to use it.

**Priority**: P0 (foundation — all other WPs depend on this)

**Test criteria**: `go test ./src/bootstrap/...` passes; existing binaries (`hsme`, `hsme-worker`, `hsme-ops`) behave identically after refactor.

### Included subtasks

- [x] T001 Create `src/bootstrap/bootstrap.go` with `OpenDB` and `OpenWithEmbedder` functions
- [x] T002 Create `src/bootstrap/config.go` with `Config` struct and `LoadFromEnv()`
- [x] T003 Move decay config loading into `OpenDB`
- [x] T004 Create `src/bootstrap/bootstrap_test.go` with init verification tests
- [x] T005 Wire `cmd/hsme/main.go` to use `bootstrap.OpenWithEmbedder`
- [x] T006 Wire `cmd/worker/main.go` to use `bootstrap.OpenWithEmbedder`
- [x] T007 Wire `cmd/ops/main.go` to use bootstrap (DB-only or with embedder, TBD per WP)
- [x] T008 Add `cli-build` and `cli-install` justfile targets

### Implementation sketch

1. `bootstrap.go` defines `OpenDB(cfg Config) (*sql.DB, error)` and `OpenWithEmbedder(cfg Config) (*sql.DB, *ollama.Embedder, error)`.
2. `OpenDB` applies `search.LoadDecayConfig()` before returning — this is the only place decay is loaded.
3. `OpenWithEmbedder` calls `OpenDB`, then creates and validates the embedder via `ValidateEmbeddingConfig`.
4. `config.go` defines `Config` with DBPath/OllamaHost/EmbeddingModel/EmbeddingDim and `LoadFromEnv()` method that reads env vars with defaults.
5. Each existing binary's `main.go` replaces its inline init with one bootstrap call.
6. `ops` binary needs embedder only if it processes embeddings; confirm and adjust.
7. Justfile gets `cli-build` (compile with tags `sqlite_fts5 sqlite_vec` to `bin/hsme-cli`) and `cli-install` (copy to `$INSTALL_PATH`).

### Dependencies

None (this is the first WP).

### Risks

- If `ops` binary needs embedder (not clear from current code), `OpenWithEmbedder` path is needed; verify in code before implementing.

### Estimated prompt size

~420 lines

---

## WP02 — CLI Dispatcher and All Subcommand Handlers

**Goal**: Create `cmd/cli/` with all nine subcommand handlers wired to core functions.

**Priority**: P0 (core deliverable — operator-facing interface)

**Test criteria**: `hsme-cli --help` prints top-level help; each subcommand `--help` emits usage; subcommands route to correct core functions.

### Included subtasks

- [x] T009 Create `cmd/cli/output.go` — TTY detection, text/JSON formatting, ANSI colors
- [x] T010 Create `cmd/cli/flags.go` — shared flag definitions (--db, --ollama-host, --embedding-model, --format)
- [x] T011 Create `cmd/cli/help.go` — top-level help and per-subcommand help
- [x] T012 Create `cmd/cli/main.go` — entry point, subcommand dispatcher, --help handling
- [x] T013 Create `cmd/cli/store.go` — `hsme-cli store` with stdin read, --source-type, --supersedes, --force-reingest
- [x] T014 Create `cmd/cli/search.go` — `hsme-cli search-fuzzy` and `hsme-cli search-exact` handlers
- [x] T015 Create `cmd/cli/explore.go` — `hsme-cli explore` handler
- [x] T016 Create `cmd/cli/status.go` — `hsme-cli status` with --watch, --interval, worker detection
- [x] T017 Create `cmd/cli/admin.go` — `hsme-cli admin retry-failed`, `backup`, `restore` dispatch

### Implementation sketch

1. `main.go` uses stdlib `flag`; parses `os.Args[1]` for subcommand name; dispatches to handler.
2. Unknown subcommand → print help listing available subcommands + exit 1.
3. Per subcommand, parse args into typed struct (per data-model.md), invoke core function, format output.
4. Subcommands that don't need embedder use `bootstrap.OpenDB`; those that do use `bootstrap.OpenWithEmbedder`.
5. `output.go` implements `FormatText` and `FormatJSON` helpers; `IsTTY()` check on stdout; `NO_COLOR` env / `--no-color` flag respected.
6. `status --watch` uses `time.Ticker` with `signal.NotifyContext` for clean interrupt; requires TTY.
7. Worker detection via `/proc/*/comm` scan on Linux, `pgrep` fallback on macOS; `worker_online: false` if undetectable.

### Parallel opportunities

T009, T010, T011 are independently implementable (different files, no shared state).

### Dependencies

- T009, T010, T011 can start immediately (design docs already specify the interfaces)
- T012–T017 depend on T009, T010, T011 being available first

### Risks

- `status --watch` without TTY should error clearly (not just silent failure)
- `store` from TTY without pipe must print hint and exit cleanly (not hang on stdin)

### Estimated prompt size

~550 lines

---

## WP03 — CLI Unit and Integration Tests

**Goal**: Add test coverage for CLI parsing, output formatting, error paths, and end-to-end integration.

**Priority**: P1 (required by NFR-006 and NFR-004)

**Test criteria**: All new tests pass; `go test ./cmd/cli/...` and `go test ./tests/modules/...` green.

### Included subtasks

- [x] T018 Write `cmd/cli/*_test.go` — table-driven unit tests for flag parsing, dispatch, format selection, error messages
- [x] T019 Write `tests/modules/cli_test.go` — end-to-end CLI integration tests against ephemeral DB

### Implementation sketch

1. `cmd/cli/` gets a `_test.go` per handler file (e.g., `main_test.go`, `search_test.go`, `output_test.go`).
2. Table-driven tests cover: unknown subcommand → exit 1 + help; `--help` on valid subcommand; `--format=json` error output shape; `--format=text` human-readable output.
3. No actual DB in unit tests — mock the bootstrap calls or use interface.
4. `tests/modules/cli_test.go` runs CLI as subprocess against ephemeral DB; seeds via existing test util; covers `store → search-fuzzy → search-exact → explore` golden path.
5. Build tag `sqlite_fts5 sqlite_vec` required for integration tests (matches existing repo convention).

### Dependencies

- WP02 (code must exist before tests can be written)

### Risks

- Integration tests need `sqlite_fts5` and `sqlite_vec` build tags; ensure CI skips gracefully when unavailable (matching existing pattern).

### Estimated prompt size

~280 lines

---

## WP04 — Admin Operations Package

**Goal**: Create `src/core/admin/` with retry, backup, restore logic and integration tests.

**Priority**: P1 (admin operations are a core deliverable)

**Test criteria**: `go test ./src/core/admin/...` passes; backup/restore round-trip preserves data; corrupt backup is rejected.

### Included subtasks

- [x] T020 Create `src/core/admin/retry.go` — re-queue failed/exhausted tasks
- [x] T021 Create `src/core/admin/backup.go` — SQLite Online Backup API wrapper
- [x] T022 Create `src/core/admin/restore.go` — atomic restore with integrity check + WAL/SHM cleanup
- [x] T023 Write `src/core/admin/admin_test.go` — integration tests for backup/restore round-trip, integrity failure, missing backup

### Implementation sketch

1. `retry.go`: port SQL from justfile (`retry-failed` target) into a `RetryFailed(ctx, db) (int64, error)` function; return count of requeued rows.
2. `backup.go`: implement `Backup(ctx, db, destPath string) (int64, error)` using `*sqlite3.SQLiteConn.Backup()` API; return bytes written.
3. `restore.go`: implement `Restore(ctx, db, srcPath string) (*RestoreResult, error)` with integrity check via `PRAGMA integrity_check`, temp file copy, WAL/SHM cleanup, atomic rename.
4. All three use `src/core/admin/` package so `cmd/cli/admin.go` stays thin (parsing only).
5. Tests use temporary DBs; backup/restore round-trip verifies data integrity; integrity-failure path verifies rejection.

### Dependencies

- WP01 (bootstrap must be available for admin package to use)

### Risks

- SQLite Online Backup API must be verified against actual `mattn/go-sqlite3` API surface (confirmed in research but implementation check required).
- Restore must clean `-wal` and `-shm` sidecars even if they don't exist (`os.Remove` ignore `os.ErrNotExist`).

### Estimated prompt size

~350 lines

---

## WP05 — Justfile Cleanup and Legacy File Removal

**Goal**: Reduce justfile targets to wrappers, remove shell scripts that are now superseded.

**Priority**: P2 (cleanup, not blocking for functionality)

**Test criteria**: `just status` still works as one-line wrapper; `scripts/status.sh` deleted; `ideas/cli-tool.md` deleted.

### Included subtasks

- [x] T024 Refactor `justfile` status/backup/restore/retry-failed to one-line wrappers
- [x] T025 Remove `scripts/status.sh` after parity verification
- [x] T026 Remove `ideas/cli-tool.md` after spec/plan supersede

### Implementation sketch

1. Update justfile:
   ```just
   status:       ; @./hsme-cli status
   backup:       ; @./hsme-cli admin backup
   restore:      ; @./hsme-cli admin restore --latest
   retry-failed: ; @./hsme-cli admin retry-failed
   ```
2. Confirm `scripts/status.sh` parity by running `hsme-cli status` vs `bash scripts/status.sh` and comparing output fields.
3. Delete `scripts/status.sh` and `ideas/cli-tool.md`.
4. Verify `just status` still works after deletion.

### Dependencies

- WP02 (CLI must exist and be runnable before cleanup)

### Risks

- None major — wrappers are low-risk.

### Estimated prompt size

~180 lines

---

## WP06 — Integration Verification and Regression Testing

**Goal**: Verify full end-to-end operator loop, restore safety, and zero test regressions.

**Priority**: P1 (must pass before merge)

**Test criteria**: `just test` green; restore refuses corrupt backup 100%; daily ops loop works without bash.

### Included subtasks

- [x] T027 Run full test suite (`just test`) and verify zero regressions
- [x] T028 Verify restore refuses corrupt backup (100% of cases)
- [x] T029 Verify operator daily ops loop works end-to-end without bash

### Implementation sketch

1. Run `just test` and confirm all pass.
2. Create a corrupt backup (write garbage to a `.db` file), attempt `hsme-cli admin restore --from <corrupt>`; confirm exit 2 and clear error message.
3. Run the full daily ops loop manually:
   - `hsme-cli status`
   - `hsme-cli admin retry-failed`
   - `hsme-cli admin backup`
   - `hsme-cli admin restore --latest`
   - `hsme-cli search-fuzzy "context" --limit 5`
   - `hsme-cli search-exact "context" --limit 5`
   - `hsme-cli explore --entity-name "hsme" --direction upstream`
4. Verify JSON output is parseable by `jq`.
5. Confirm no bash dependency remains.

### Dependencies

- WP04 (admin operations must be implemented)
- WP05 (justfile wrappers must be in place)

### Risks

- None — this is a verification WP, not new implementation.

### Estimated prompt size

~200 lines

---

## WP07 — Post-Merge Cleanup and Documentation

**Goal**: Archive completed mission, update skill registry, sync delta specs to main.

**Priority**: P3 (post-merge, not blocking)

**Test criteria**: Mission archived; skills updated; delta specs synced.

### Included subtasks

- [ ] (No new subtasks — this WP wraps up existing work)
  - Archive mission after successful WP06 verification
  - Run `sdd-archive` or equivalent to sync delta specs to main
  - Update skill registry if new patterns were introduced

### Implementation sketch

1. After WP06 passes, run the archive step.
2. This WP is intentionally minimal — most "archive" work is automatic via spec-kitty finalize.

### Dependencies

- WP06 (must complete before archive)

### Risks

- None.

### Estimated prompt size

~80 lines

---

## Summary

| Work Package | Subtasks | Focus | Estimated Lines |
|-------------|----------|-------|----------------|
| WP01 | T001–T008 (8) | Shared bootstrap package + justfile targets | ~420 |
| WP02 | T009–T017 (9) | CLI dispatcher + all 9 subcommand handlers | ~550 |
| WP03 | T018–T019 (2) | CLI unit + integration tests | ~280 |
| WP04 | T020–T023 (4) | Admin operations (retry, backup, restore) | ~350 |
| WP05 | T024–T026 (3) | Justfile cleanup + legacy file removal | ~180 |
| WP06 | T027–T029 (3) | Integration verification + regression testing | ~200 |
| WP07 | — (wrap-up) | Archive + documentation | ~80 |
| **Total** | **29 subtasks** | **7 work packages** | **~2060 lines** |

**Size distribution**: WP01 (8 subtasks, ~420 lines), WP02 (9 subtasks, ~550 lines), WP03 (2 subtasks, ~280 lines), WP04 (4 subtasks, ~350 lines), WP05 (3 subtasks, ~180 lines), WP06 (3 subtasks, ~200 lines), WP07 (wrap-up, ~80 lines).

**Size validation**: ✓ All WPs within ideal range (3-10 subtasks, 200-700 lines). WP02 has 9 subtasks at ~550 lines — at the upper end but acceptable.

**MVP scope**: WP01 (bootstrap) + WP02 (CLI dispatcher) are the core deliverable. WP03–WP04 add testing and admin operations. WP05–WP07 are polish/cleanup.

**Parallelization highlights**:
- WP01 runs first (foundation, no dependencies)
- WP02 can start once design docs (data-model, contracts) are available — T009/T010/T011 are parallelizable within WP02
- WP03 waits for WP02 code
- WP04 waits for WP01 (bootstrap)
- WP05 waits for WP02 (CLI must be runnable)
- WP06 waits for WP04 + WP05
- WP07 waits for WP06

**Execution order**:
```
WP01 → WP02 → WP03 (test after code)
      ↗ WP04 (bootstrap needed)
      ↘ WP05 (CLI needed)
          → WP06 → WP07
```

---

_Next_: Run `/spec-kitty.analyze` to cross-artifact consistency check, then `/spec-kitty.implement` to execute work packages.