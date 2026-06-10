# Specification: HSME CLI Polish

**Mission ID**: `01KQ5PACY0K8RA6HN1ZGB9TPBD`
**Mission Slug**: `hsme-cli-polish-01KQ5PAC`
**Mission Type**: software-dev
**Status**: Draft
**Created**: 2026-04-26
**Target Branch**: `main`

## Purpose

### TLDR

Fix 6 bugs in the `hsme-cli` binary so the CLI is production-grade: correct flag parsing regardless of position, human-readable text output by default, correct backup naming, safe restore order, graph stats in status, and configurable watch interval.

### Stakeholder Context

Operators use `hsme-cli` daily for system inspection and admin operations. Bugs discovered in the post-launch audit make the default text output unusable and introduce data-safety risk in restore. Fixing these is a prerequisite for promoting `hsme-cli` as the primary ops interface.

## User Scenarios & Testing

### Primary Scenario — Operator queries with flags after positional

**Actor**: HSME operator
**Trigger**: Operator runs `hsme-cli search-exact "ollama" --format=json --limit 1`
**Expected**: JSON output with results
**Current bug**: Text Go-map output (`map[results:[...]]`) because `--format` is ignored when it appears after the positional argument

### Secondary Scenario — Operator stores content and reads human output

**Actor**: HSME operator
**Trigger**: `echo "test" | hsme-cli store --source-type note`
**Expected**: Human-readable confirmation: `stored memory 1011 [note]`
**Current bug**: Raw Go representation: `map[memory_id:1010 status:stored]`

### Tertiary Scenario — Operator runs status dashboard

**Actor**: HSME operator
**Trigger**: `hsme-cli status`
**Expected**: Worker state, queue counts, AND graph node/edge counts
**Current bug**: Graph counts missing; only memories and tasks shown

### Quaternary Scenario — Operator uses watch mode

**Actor**: HSME operator
**Trigger**: `hsme-cli status --watch --interval 5s`
**Expected**: Dashboard refreshes every 5 seconds
**Current bug**: `--interval` flag does not exist; always refreshes at hardcoded 2 seconds

## Functional Requirements

### FR-001 — Flag parsing after positional arguments

**Requirement**: All flags (`--format`, `--limit`, `--project`, etc.) must be parsed correctly regardless of whether they appear before or after positional arguments.

**Current bug**: `flag.NewFlagSet().Parse()` stops at the first non-flag argument. Flags after positional args are silently ignored.

**Fix approach**: Re-parse `os.Args` to extract flags that appear after the subcommand and positional args, then merge with the `FlagSet` parsed values before dispatch.

**Status**: Required

### FR-002 — Human-readable text output

**Requirement**: `--format=text` (the default) must produce human-readable, structured output — not Go's default struct representation.

**Per subcommand expected output**:

| Subcommand | Expected text format |
|-----------|---------------------|
| `store` | `Stored memory 1011 [note]` or similar readable format |
| `search-fuzzy` | One result per line: `#001 — "title snippet..." (score 0.92)` |
| `search-exact` | One result per line: `#001 — "title snippet..."` |
| `explore` | Node/edge table or indented tree |
| `admin backup` | `Backup created: backups/engram-20260426T160046Z.db (12.2 MB)` |
| `admin restore` | `Restored from backups/engram-20260426T160046Z.db — integrity OK` |
| `admin retry-failed` | `Requeued 3 tasks. Queue: 12 pending, 2 failed.` |

**Current bug**: `FormatText` is `fmt.Sprintf("%v", v)` producing `map[key:value ...]`.

**Status**: Required

### FR-003 — Correct backup file naming

**Requirement**: Backup filenames must use UTC timestamps with `T` separator: `engram-YYYYMMDDTHHMMSSZ.db`

**Current bug**: `time.Now().Format("20060102_150405")` produces `engram-20260426_144245.db` (local time, underscore).

**Fix**: `time.Now().UTC().Format("20060102T150405Z")`

**Examples**:
- Correct: `engram-20260426T144245Z.db`
- Incorrect (current): `engram-20260426_144245.db`

**Status**: Required

### FR-004 — Safe restore order (cleanup before rename)

**Requirement**: WAL/SHM sidecars must be cleaned BEFORE the atomic rename, not after. This prevents a window where the restored DB coexists with stale sidecars.

**Correct order**:
1. Verify integrity
2. Copy to temp file in same directory
3. **Remove target WAL/SHM sidecars**
4. **Atomic rename** temp → target

**Current bug**: `os.Rename` happens first (line 41), then sidecar cleanup (lines 50-51). This is the wrong order.

**Status**: Required

### FR-005 — Graph stats in status output

**Requirement**: `hsme-cli status` must include graph node and edge counts.

**Expected JSON shape**:
```json
{
  "worker_running": true,
  "memories": 1005,
  "tasks": { "pending": 12, "processing": 2, "completed": 847, "failed": 3 },
  "graph": { "nodes": 1005, "edges": 2841 }
}
```

**Current bug**: Only `memories`, `tasks`, `worker_running` are returned; `graph` object is missing.

**Status**: Required

### FR-006 — `--interval` flag for watch mode

**Requirement**: `hsme-cli status --watch --interval <duration>` must control the refresh rate.

**Valid durations**: Go duration strings (`5s`, `1m`, `500ms`, etc.)

**Default**: `2s` (preserves current behavior)

**Current bug**: `time.Sleep(2 * time.Second)` hardcoded; `--interval` flag does not exist.

**Status**: Required

## Non-Functional Requirements

| ID | Requirement | Threshold | Status |
|----|-------------|-----------|--------|
| NFR-001 | All existing tests continue to pass after fixes. | Zero new failures or skips in `go test ./...` | Required |
| NFR-002 | Backup naming change does not break `--latest` selection for existing backups. | Existing `engram-*.db` files continue to be selectable via `--latest` glob | Required |
| NFR-003 | Restore safety: corrupt backup is rejected 100% of the time. | Integrity check must run before any file modification | Required |

## Constraints

| ID | Constraint | Status |
|----|-----------|--------|
| C-001 | No new third-party dependencies may be introduced. | Mandatory |
| C-002 | Existing binary behavior for the three other binaries (`hsme`, `hsme-worker`, `hsme-ops`) must not change. Bootstrap refactor is untouched by this mission. | Mandatory |

## Success Criteria

1. `hsme-cli search-fuzzy "ollama" --format=text` produces readable text output (not Go map representation).
2. `hsme-cli search-exact "context" --limit 2 --format=json` works correctly with flags after positional.
3. `hsme-cli admin backup` creates files matching `engram-YYYYMMDDTHHMMSSZ.db` pattern (UTC, `T` separator).
4. `hsme-cli status` output includes `graph.nodes` and `graph.edges` fields.
5. `hsme-cli status --watch --interval 5s` refreshes at 5-second intervals.
6. `go test ./cmd/cli/... ./src/core/admin/... -tags sqlite_fts5_sqlite_vec` passes with zero failures.
7. Restore order is: verify → copy → cleanup sidecars → atomic rename.

## Assumptions

- The `engram-*.db` glob for `--latest` backup selection matches any file starting with `engram-` and ending with `.db`, regardless of separator (`_` or `-`). Existing backups created with `_` are readable but new backups use the correct `T` format.
- The `FormatText` rewrite will produce one-line-per-result for search subcommands and structured summaries for admin subcommands, matching the table above.

## Key Entities

| Entity | Description |
|--------|-------------|
| `hsme-cli` binary | Existing CLI binary at `cmd/cli/`, built with tags `sqlite_fts5 sqlite_vec` |
| `cmd/cli/output.go` | File requiring FormatText rewrite |
| `cmd/cli/main.go` | File requiring flag reparse logic |
| `cmd/cli/admin.go` | File requiring backup naming fix |
| `src/core/admin/restore.go` | File requiring restore order fix |
| `cmd/cli/status.go` | File requiring graph stats and --interval implementation |