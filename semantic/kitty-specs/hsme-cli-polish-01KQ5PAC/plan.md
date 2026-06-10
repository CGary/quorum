# Implementation Plan: HSME CLI Polish

**Mission ID**: `01KQ5PACY0K8RA6HN1ZGB9TPBD`
**Mission Slug**: `hsme-cli-polish-01KQ5PAC`
**Branch**: `main` | **Date**: 2026-04-26 | **Spec**: [spec.md](spec.md)

## Summary

Fix 6 bugs in the `hsme-cli` binary: flag parsing after positional args, human-readable FormatText output, correct backup naming (UTC with T separator), safe restore order (cleanup before rename), graph stats in status, and configurable watch interval.

## Technical Context

| Field | Value |
|-------|-------|
| **Language/Version** | Go 1.26.2 (per `go.mod`) |
| **Primary Dependencies** | `github.com/mattn/go-sqlite3 v1.14.42`, `github.com/asg017/sqlite-vec-go-bindings v0.1.6`, `golang.org/x/text v0.36.0` (no new deps) |
| **Storage** | SQLite with WAL mode, FTS5, sqlite-vec |
| **Testing** | Go `testing` package with table-driven tests; build tags `sqlite_fts5 sqlite_vec` |
| **Target Platform** | Linux/macOS terminal |
| **Scope** | 6 bug fixes in existing CLI code; ~200 LoC estimated |
| **Module Path** | `github.com/hsme/core` |

## Engineering Alignment (decisions cristalizadas)

| Bug | Decision | Fix |
|-----|----------|-----|
| **FR-001**: Flag parsing after positional | Re-parse `os.Args` post-FlagSet.Parse to extract trailing flags; merge into parsed flags before dispatch | `cmd/cli/main.go` |
| **FR-002**: FormatText raw Go output | Rewrite `FormatText` per subcommand: store → `Stored memory {id} [{type}]`; search → one line per result; admin → structured summaries | `cmd/cli/output.go` |
| **FR-003**: Backup naming with `_` and local time | Change to `time.Now().UTC().Format("20060102T150405Z")` | `cmd/cli/admin.go` |
| **FR-004**: Restore order (rename then cleanup) | Move WAL/SHM cleanup BEFORE `os.Rename`; swap lines 41 and 50-51 in `restore.go` | `src/core/admin/restore.go` |
| **FR-005**: Missing graph stats | Add `SELECT COUNT(*) FROM memories; SELECT COUNT(*) FROM memory_dependencies;` to status query | `cmd/cli/status.go` |
| **FR-006**: `--interval` missing | Add `--interval` flag with `time.Duration` type; use `time.Sleep(interval)` in watch loop | `cmd/cli/status.go` |

## Project Structure

**Files modified by this mission:**

```
cmd/cli/
├── main.go              # FR-001: re-parse trailing flags after FlagSet.Parse
├── output.go           # FR-002: rewrite FormatText per subcommand
├── admin.go            # FR-003: fix backup filename format
└── status.go          # FR-005: add graph stats; FR-006: add --interval flag

src/core/admin/
└── restore.go         # FR-004: swap cleanup order (cleanup → rename)

cmd/hsme/main.go       # unchanged
cmd/worker/main.go     # unchanged
cmd/ops/main.go        # unchanged
justfile               # unchanged (wrappers already correct)
```

**Files NOT modified by this mission** (per C-002):
- `src/bootstrap/` — bootstrap refactor untouched
- `src/core/search/` — search logic untouched
- `src/core/indexer/` — indexer logic untouched

## Charter Check

**Status**: SKIPPED — no charter file exists at `.kittify/charter/charter.md`.

No charter gates to evaluate. Engineering alignment is governed by this plan and the mission spec.

## Complexity Tracking

| Topic | Decision | Rationale |
|-------|----------|-----------|
| FormatText rewrite | Per-subcommand implementation | Each subcommand has different result shapes; a generic formatter can't produce the desired human-readable output. Subcommand-specific printers in `output.go` are the cleanest approach. |
| Flag reparse | Post-hoc args scan | Go's `flag` stdlib doesn't support non-linear flag ordering. A post-parse scan of `os.Args` that extracts `--flag value` pairs after positional args and merges them is the minimal, non-invasive fix. No new deps. |
| Restore order | Lines swapped in `restore.go` | The fix is a 3-line move. The integrity check already runs before the copy, so the safety guarantee is maintained. The order swap eliminates the window where a new DB coexists with stale WAL/SHM. |

## Phase 0 Output

No Phase 0 research needed — all 6 bugs have known solutions with clear fix locations.

## Phase 1 Output

### No new data model

The mission does not introduce new entities, change persistent schema, or alter API contracts. The data model from `hsme-unified-cli-01KQ59MV` is unchanged.

### Output shape change — `status`

`status` output gains a `graph` field. This is an additive JSON change (backward-compatible for JSON consumers that ignore unknown fields):

```go
// cmd/cli/status.go — StatusResult struct updated:
type StatusResult struct {
    WorkerOnline  bool         `json:"worker_running"`
    Memories     int64        `json:"memories"`
    Tasks        QueueStats   `json:"tasks"`
    Graph        GraphStats  `json:"graph"`  // NEW
}
type GraphStats struct {
    Nodes int64 `json:"nodes"`
    Edges int64 `json:"edges"`
}
```

### No new contracts

All contracts (flags, exit codes, output shapes) are unchanged except for the additive `graph` field in status JSON.

## Branch Contract (final)

- **Current branch**: `main`
- **Planning/base branch**: `main`
- **Final merge target**: `main`
- **branch_matches_target**: `true`

The mission is ready for `/spec-kitty.tasks`.