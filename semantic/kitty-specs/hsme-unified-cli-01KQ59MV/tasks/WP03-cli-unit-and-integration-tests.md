---
work_package_id: WP03
title: CLI Unit and Integration Tests
dependencies:
- WP02
requirement_refs:
- NFR-006
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
subtasks:
- T018
- T019
agent: "gemini:o3:reviewer:reviewer"
shell_pid: "2103793"
history:
- date: '2026-04-26T16:47:42Z'
  action: tasks generated
  actor: tasks skill
agent_profile: ''
authoritative_surface: cmd/cli/
execution_mode: code_change
model: ''
owned_files:
- cmd/cli/main_test.go
- cmd/cli/output_test.go
- cmd/cli/store_test.go
- cmd/cli/search_test.go
- cmd/cli/explore_test.go
- cmd/cli/status_test.go
- cmd/cli/admin_test.go
- tests/modules/cli_test.go
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

Add test coverage for the CLI: table-driven unit tests for parsing, dispatch, formatting, and error paths; end-to-end integration tests against ephemeral DBs.

---

## Context

### Feature directory

`/home/gary/dev/hsme/kitty-specs/hsme-unified-cli-01KQ59MV`

### Dependencies

- **WP02 must be complete** — the `cmd/cli/` code must exist before tests can be written.
- The test pattern should mirror existing repo patterns in `tests/modules/`.

### What this WP produces

```
cmd/cli/main_test.go         # flag parsing, subcommand routing, --help handling
cmd/cli/output_test.go       # TTY detection, text/JSON formatting, color logic
cmd/cli/store_test.go        # store subcommand flag parsing, error paths
cmd/cli/search_test.go       # search-fuzzy/exact flag parsing
cmd/cli/explore_test.go      # explore flag parsing
cmd/cli/status_test.go       # status flag parsing, --watch error (no TTY)
cmd/cli/admin_test.go        # admin subcommand routing, mutually exclusive flags

tests/modules/cli_test.go    # end-to-end CLI integration tests
```

---

## Guidance per Subtask

### T018 — Write `cmd/cli/*_test.go` (unit tests)

**Files**: `cmd/cli/main_test.go`, `output_test.go`, `store_test.go`, `search_test.go`, `explore_test.go`, `status_test.go`, `admin_test.go`

**Pattern**: Go table-driven tests (`testing` package).

**Build tags**: `//go:build sqlite_fts5 sqlite_vec` on files that call bootstrap functions.

**Test categories per file**:

**`main_test.go`**:
- Unknown subcommand → exit 1 + top-level help printed
- `help` with no subcommand → top-level help printed
- `help <valid-subcommand>` → subcommand help printed
- No args → exit 1 + top-level help printed
- `--help` on known subcommand → subcommand help printed

**`output_test.go`**:
- `IsTTY()` returns false when stdout is piped (test by checking behavior)
- `ShouldColor()` respects `NO_COLOR` env
- `FormatJSON` produces valid JSON for a struct
- `FormatJSON` fails gracefully for non-serializable values
- `WriteError` in text mode writes to stderr
- `WriteError` in JSON mode writes `{"error":"...", "code":N}` to stderr

**`store_test.go`**:
- Missing `--source-type` → exit 1
- `--source-type code` (valid) → proceeds to try DB open (may fail in unit test without DB, that's OK — verify flag parsing first)
- Empty stdin on TTY → hint printed (this requires mocking stdin — can skip in unit test if complex)

**`search_test.go`**:
- Missing positional arg → exit 1
- `--limit` flag parsed correctly
- Valid `search-fuzzy <query>` parses correctly
- Valid `search-exact <keyword>` parses correctly

**`explore_test.go`**:
- Missing positional arg → exit 1
- `--direction` values validated (accept: upstream, downstream, both; reject others with exit 1)
- `--max-depth` and `--max-nodes` parsed correctly

**`status_test.go`**:
- `--watch` with non-TTY stdout → error + exit 1 (mock `IsTTY` to return false)
- `--interval` parsed as duration

**`admin_test.go`**:
- `admin` with no subcommand → help + exit 1
- `admin retry-failed` → route correct
- `admin backup` → route correct
- `admin restore` with neither `--from` nor `--latest` → exit 1
- `admin restore` with both `--from` and `--latest` → exit 1
- `admin restore` with `--from <path>` only → proceed (may fail without DB, verify flag parsing)
- `admin restore` with `--latest` only → proceed

**Important**: No actual DB needed for unit tests. Use mock interfaces or verify flag parsing without opening connections. For tests that call bootstrap functions, use build tag guards.

---

### T019 — Write `tests/modules/cli_test.go` (integration tests)

**File**: `tests/modules/cli_test.go` (new file)

**Purpose**: End-to-end CLI invocation against an ephemeral DB, covering the golden path `store → search-fuzzy → search-exact → explore`.

**Pattern**: Subprocess invocation of `hsme-cli` binary. See existing integration tests in `tests/modules/` for conventions.

**Build tags**: `//go:build sqlite_fts5 sqlite_vec` — requires SQLite extensions.

**Test sequence**:

1. **Setup**: Create temp DB, run migrations, seed with known data (use existing test util if available).
2. **`store` test**: Run `hsme-cli store --source-type note --project test <<< "hello world"`. Verify exit 0, JSON output contains `memory_id`.
3. **`search-fuzzy` test**: Run `hsme-cli search-fuzzy "hello" --limit 5`. Verify exit 0, JSON output contains `results` array.
4. **`search-exact` test**: Run `hsme-cli search-exact "hello" --limit 5`. Verify exit 0, JSON output contains `results` array.
5. **`explore` test**: Run `hsme-cli explore <canonical-name>`. Verify exit 0, output is valid JSON.
6. **`status` test**: Run `hsme-cli status`. Verify exit 0, JSON contains `worker_online`, `queue`, `graph`.
7. **`admin retry-failed` test**: Verify exit 0, JSON contains `requeued` count.
8. **`admin backup` test**: Run backup, verify exit 0, backup file created, JSON contains `backup_path`, `size_bytes`.
9. **`admin restore` test**: Take the backup from step 8, restore it, verify exit 0 and `integrity_ok: true`.

**Cleanup**: Delete temp DB and any backup files after each test.

**Error handling**: If `hsme-cli` binary is not built yet, skip tests with a clear message via `testing.Skip()`.

---

## Branch Strategy

- **Planning branch**: `main`
- **Final merge target**: `main`
- **Execution worktrees**: Allocated per computed lane from `lanes.json`.

---

## Definition of Done

- [ ] `go test ./cmd/cli/...` passes (unit tests)
- [ ] `go test ./tests/modules/... -tags sqlite_fts5_sqlite_vec` passes (integration tests)
- [ ] All table-driven test cases cover both happy path and error paths
- [ ] Integration tests cover the full `store → search → explore` loop
- [ ] No skipped tests except for build-tag-gated cases (SQLite extensions not available)

---

## Risks & Reviewer Guidance

**Risk — Integration tests need built binary**: `tests/modules/cli_test.go` should check if `hsme-cli` binary exists and skip with a clear message if not (don't fail the test suite just because the binary hasn't been built yet).

**Risk — Build tag conflicts**: Ensure `//go:build sqlite_fts5 sqlite_vec` is consistent across all test files that need the DB.

**Reviewer**: After WP03, run `go test ./cmd/cli/... -v` and verify each test case is exercised. Check coverage for error paths (unknown subcommand, missing required flags, etc.).

## Activity Log

- 2026-04-26T18:10:54Z – gemini:o3:implementer:implementer – shell_pid=2100438 – Started implementation via action command
- 2026-04-26T18:12:46Z – gemini:o3:implementer:implementer – shell_pid=2100438 – Implemented CLI unit and integration tests.
- 2026-04-26T18:12:51Z – gemini:o3:reviewer:reviewer – shell_pid=2103793 – Started review via action command
- 2026-04-26T18:13:11Z – gemini:o3:reviewer:reviewer – shell_pid=2103793 – Review passed: Unit and integration tests for CLI are comprehensive and passing.
