# Tasks: HSME CLI Polish

**Mission**: `hsme-cli-polish-01KQ5PAC`
**Date**: 2026-04-26
**NOW_UTC_ISO**: 2026-04-26T20:17:55Z
**Planning branch**: `main`
**Merge target**: `main`
**Branch matches target**: true

## Subtask Index

| ID | Description | WP | Parallel |
|----|-------------|----|----------|
| T001 | Rewrite FormatText for `store` result: readable `Stored memory {id} [{type}]` | WP01 | [P] | [D] |
| T002 | Rewrite FormatText for `search-fuzzy` and `search-exact`: one line per result with score | WP01 | [D] |
| T003 | Rewrite FormatText for `explore`, `admin backup`, `admin restore`, `admin retry-failed`: structured summaries | WP01 | [D] |
| T004 | Fix backup naming: `time.Now().UTC().Format("20060102T150405Z")` | WP02 | [D] |
| T005 | Fix restore order: cleanup WAL/SHM BEFORE atomic rename | WP02 | [D] |
| T006 | Fix flag parsing after positional: re-parse trailing flags from `os.Args` | WP02 |  | [D] |
| T007 | Add graph stats (nodes + edges) to `status` output | WP02 |  | [D] |
| T008 | Add `--interval` flag to `status --watch` | WP02 |  | [D] |

---

## WP01 — FormatText Rewrite

**Goal**: Replace raw `fmt.Sprintf("%v", v)` with human-readable per-subcommand text formatters.

**Priority**: P0 (UX-critical — default text output is currently unusable)

**Test criteria**: `go test ./cmd/cli/...` passes; `--format=text` produces readable output for all 7 subcommands.

### Included subtasks

- [x] T001 Rewrite FormatText for `store` result
- [x] T002 Rewrite FormatText for `search-fuzzy` and `search-exact`
- [x] T003 Rewrite FormatText for `explore`, `admin backup`, `admin restore`, `admin retry-failed`

### Implementation sketch

1. In `cmd/cli/output.go`, add `FormatStoreResult`, `FormatSearchResults`, `FormatExploreResult`, `FormatAdminBackupResult`, `FormatAdminRestoreResult`, `FormatAdminRetryResult` functions.
2. Each formatter produces a `string` — not a Go debug representation.
3. Update callers to use the new formatters instead of `FormatText`.
4. Keep `FormatJSON` unchanged — it already works correctly.
5. `IsTTY()` and `ShouldColor()` are unchanged.

### Dependencies

None (independent, all in same file).

### Risks

- None — this is a pure rewrite, no logic changes.

### Estimated prompt size

~280 lines

---

## WP02 — Remaining Bug Fixes

**Goal**: Fix FR-003 (backup naming), FR-004 (restore order), FR-001 (flag parsing), FR-005 (graph stats), FR-006 (--interval).

**Priority**: P0 (critical UX + data safety)

**Test criteria**: `go test ./cmd/cli/... ./src/core/admin/...` passes; all 5 success criteria from spec.md verified.

### Included subtasks

- [x] T004 Fix backup naming to UTC with `T` separator
- [x] T005 Fix restore order: cleanup WAL/SHM before atomic rename
- [x] T006 Fix flag parsing: re-parse trailing flags from `os.Args`
- [x] T007 Add graph stats (nodes + edges) to `status` output
- [x] T008 Add `--interval` flag to `status --watch`

### Implementation sketch

**T004 — Backup naming** (`cmd/cli/admin.go`):
- Change `time.Now().Format("20060102_150405")` to `time.Now().UTC().Format("20060102T150405Z")`
- Verify backup path returned to user matches the created file

**T005 — Restore order** (`src/core/admin/restore.go`):
- Move WAL/SHM cleanup (currently lines 50-51) to BEFORE the `os.Rename` (currently line 41)
- New order: verify → copy → cleanup sidecars → atomic rename
- Keep integrity check as step 1 (already correct)

**T006 — Flag parsing** (`cmd/cli/main.go`):
- After `flagSet.Parse()`, scan remaining `os.Args` for `--flag value` pairs
- Merge into a `map[string]string` and apply to already-parsed flags
- Pattern: iterate `flagSet.Args()` looking for `--key` followed by a non-flag value

**T007 — Graph stats** (`cmd/cli/status.go`):
- Add to `getStatus()` SQL: `SELECT COUNT(*) FROM memories`, `SELECT COUNT(*) FROM memory_dependencies`
- Add `GraphStats` to `StatusResult` struct: `type GraphStats struct { Nodes int64 \`json:"nodes"`, Edges int64 \`json:"edges"\` }`
- Update `formatStatus()` to include graph section

**T008 — --interval** (`cmd/cli/status.go`):
- Add `--interval` flag: `var interval time.Duration; flag.DurationVar(&interval, "interval", 2*time.Second, "watch interval")`
- Replace `time.Sleep(2 * time.Second)` with `time.Sleep(interval)` in watch loop

### Dependencies

T004, T005, T007, T008 are independent per-file fixes. T006 (flag parsing) depends on the existing subcommand structure but is self-contained.

### Risks

- T005 (restore order): Ensure cleanup happens BEFORE rename — this is the critical data-safety fix.
- T006 (flag parsing): Must not break flags that ARE correctly parsed before the positional.

### Estimated prompt size

~420 lines

---

## Summary

| Work Package | Subtasks | Focus | Est. Lines |
|-------------|----------|-------|------------|
| WP01 | T001–T003 (3) | FormatText rewrite in output.go | ~280 |
| WP02 | T004–T008 (5) | Remaining 5 bugs in 4 files | ~420 |
| **Total** | **8 subtasks** | **2 work packages** | **~700 lines** |

**Size distribution**: WP01 (~280 lines), WP02 (~420 lines)

**Size validation**: ✓ All WPs within ideal range (<700 lines)

**MVP scope**: WP01 (FormatText) is the primary UX deliverable. WP02 has FR-001 (flag parsing) and FR-004 (restore safety) as equally critical.

**Parallelization**: WP01 and WP02 are independent and can run in parallel.

---

## Branch Strategy

- **Planning branch**: `main`
- **Final merge target**: `main`
- All execution in `.worktrees/` via `spec-kitty next --agent <name> --mission hsme-cli-polish-01KQ5PAC`

---

_Next_: Run `/spec-kitty.implement` to execute work packages.