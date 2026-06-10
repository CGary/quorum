---
work_package_id: WP02
title: Remaining Bug Fixes
dependencies: []
requirement_refs:
- FR-001
- FR-003
- FR-004
- FR-005
- FR-006
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-hsme-cli-polish-01KQ5PAC
base_commit: 9d65ffa10332509a7c716a87a02fdcb73a5f8c1b
created_at: '2026-04-26T20:32:23.267720+00:00'
subtasks:
- T004
- T005
- T006
- T007
- T008
agent: "gemini:o3:reviewer:reviewer"
shell_pid: "2231843"
history:
- date: '2026-04-26T20:17:55Z'
  action: tasks generated
  actor: tasks skill
agent_profile: ''
authoritative_surface: cmd/cli/
execution_mode: code_change
model: ''
owned_files:
- cmd/cli/admin.go
- cmd/cli/main.go
- cmd/cli/status.go
- src/core/admin/restore.go
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

Fix 5 remaining bugs: backup naming format (FR-003), restore order (FR-004), flag parsing after positional args (FR-001), graph stats in status (FR-005), and `--interval` flag (FR-006).

---

## Context

### Feature directory

`/home/gary/dev/hsme/kitty-specs/hsme-cli-polish-01KQ5PAC`

### Files to modify

| File | Bugs fixed |
|------|-----------|
| `cmd/cli/admin.go` | FR-003 (backup naming) |
| `src/core/admin/restore.go` | FR-004 (restore order) |
| `cmd/cli/main.go` | FR-001 (flag parsing) |
| `cmd/cli/status.go` | FR-005 (graph stats), FR-006 (--interval) |

---

## Guidance per Subtask

### T004 — Fix backup naming format (FR-003)

**File**: `cmd/cli/admin.go`

**Current bug**: `time.Now().Format("20060102_150405")` produces `engram-20260426_144245.db` (local time, underscore separator).

**Fix**:
```go
// Change:
time.Now().Format("20060102_150405")
// To:
time.Now().UTC().Format("20060102T150405Z")
```

**Result**: `engram-20260426T144245Z.db` (UTC, `T` separator, correct per spec/data-model).

**Validation**:
```bash
hsme-cli admin backup --format=json
# Look for: "backup": "backups/engram-20260426T..." (with T, not _)
```

**Note**: The backup file path returned to the user must match the actual created file.

---

### T005 — Fix restore order: cleanup before rename (FR-004)

**File**: `src/core/admin/restore.go`

**Current order** (WRONG):
1. Verify integrity
2. Copy to temp
3. **Rename temp → target** (line 41)
4. **Cleanup WAL/SHM** (lines 50-51) ← AFTER rename

**Correct order** (REQUIRED):
1. Verify integrity
2. Copy to temp
3. **Cleanup WAL/SHM** ← BEFORE rename
4. **Rename temp → target**

**Implementation**: Move the two `os.Remove` lines (currently 50-51) to run BEFORE `os.Rename` (currently 41).

```go
// BEFORE (current — lines ~41 and ~50-51):
if err := os.Rename(tmpPath, dbPath); err != nil {
    os.Remove(tmpPath)
    return err
}
// cleanup after rename (WRONG):
_ = os.Remove(dbPath + "-wal")
_ = os.Remove(dbPath + "-shm")

// AFTER (correct):
// cleanup BEFORE rename (CORRECT):
_ = os.Remove(dbPath + "-wal")
_ = os.Remove(dbPath + "-shm")
if err := os.Rename(tmpPath, dbPath); err != nil {
    os.Remove(tmpPath)
    return err
}
```

**Validation**: Verify the code order is: verify → copy → cleanup → rename. Read the current `restore.go` and confirm line numbers before editing.

---

### T006 — Fix flag parsing after positional args (FR-001)

**File**: `cmd/cli/main.go`

**Current bug**: `flag.NewFlagSet().Parse()` stops at the first non-flag argument. Flags like `--format=json` that appear after the positional query are silently ignored.

**Example of bug**:
```bash
hsme-cli search-exact "ollama" --format=json  # WORKS
hsme-cli search-exact --format=json "ollama"  # WORKS
hsme-cli search-exact "ollama" --limit 1      # FAILS — --limit ignored, returns all results
```

**Fix approach**: After `flagSet.Parse()` returns, scan the remaining unparsed arguments (`flagSet.Args()`) for `--flag value` pairs and merge them into the already-parsed flags.

**Implementation sketch**:
```go
// After flagSet.Parse() for the subcommand
remaining := flagSet.Args()
parsedFlags := map[string]string{}

// Scan remaining args for --flag value patterns
for i := 0; i < len(remaining); i++ {
    arg := remaining[i]
    if strings.HasPrefix(arg, "--") {
        flagName := strings.TrimPrefix(arg, "--")
        if i+1 < len(remaining) && !strings.HasPrefix(remaining[i+1], "--") {
            // This is --flag value
            parsedFlags[flagName] = remaining[i+1]
            i++
        } else if i+1 < len(remaining) {
            // Boolean flag: --flag (no value)
            parsedFlags[flagName] = "true"
        }
    }
}

// Override flag values that were not set by FlagSet
// Use flag.Lookup to check if a flag was explicitly set
for name, value := range parsedFlags {
    f := flagSet.Lookup(name)
    if f != nil && f.Value.String() == "" {
        f.Value.Set(value)  // only override if still zero
    }
}
```

**Key insight**: Only override if the flag value is still zero/empty. This preserves flags that were correctly parsed before the positional.

**Edge cases to handle**:
- `--format json` (space, not `=`) — parse both `--flag value` and `--flag=value`
- Boolean flags like `--watch` (no value)
- Flags already correctly parsed from pre-positional position (don't double-override)

**Validation**:
```bash
hsme-cli search-exact "ollama" --format=json --limit 1
# Must return valid JSON (not Go map repr)

hsme-cli search-exact "ollama" --limit 1 --format=json
# Same result — order of flags shouldn't matter
```

---

### T007 — Add graph stats to status output (FR-005)

**File**: `cmd/cli/status.go`

**Current missing**: `status` only returns `memories`, `tasks`, `worker_running`. Graph counts are absent.

**SQL to add** (from the original `scripts/status.sh`):
```sql
SELECT COUNT(*) FROM memories;
SELECT COUNT(*) FROM memory_dependencies;
```

**Expected JSON output**:
```json
{
  "worker_running": true,
  "memories": 1005,
  "tasks": { "pending": 12, "processing": 2, "completed": 847, "failed": 3 },
  "graph": { "nodes": 1005, "edges": 2841 }
}
```

**Implementation**:
1. Add `GraphStats` struct: `type GraphStats struct { Nodes int64 \`json:"nodes"\`, Edges int64 \`json:"edges"\` }`
2. Add `Graph` field to `StatusResult`: `Graph GraphStats \`json:"graph"\``
3. Add SQL queries in `getStatus()` to get node and edge counts
4. Populate `Graph` field in the result

**Validation**:
```bash
hsme-cli status --format=json | jq .
# Output must contain: "graph": {"nodes": N, "edges": M}
```

---

### T008 — Add `--interval` flag to status watch (FR-006)

**File**: `cmd/cli/status.go`

**Current bug**: `time.Sleep(2 * time.Second)` hardcoded in watch loop.

**Fix**:
```go
// Add flag registration in setup():
var interval time.Duration
flag.DurationVar(&interval, "interval", 2*time.Second, "watch refresh interval")

// In watch loop, replace hardcoded sleep:
- time.Sleep(2 * time.Second)
+ time.Sleep(interval)
```

**Important**: The flag is only meaningful when `--watch` is set. If `--watch` is false, the interval flag should be accepted but not used (don't fail if `--interval` is used without `--watch`).

**Validation**:
```bash
hsme-cli status --help
# Should list: --interval duration (default 2s)

hsme-cli status --watch --interval 5s
# Should refresh every 5 seconds (test by counting iterations in a fixed time window)
```

---

## Branch Strategy

- **Planning branch**: `main`
- **Final merge target**: `main`
- **Execution worktrees**: Allocated per computed lane from `lanes.json`.

---

## Definition of Done

- [ ] `go test ./cmd/cli/... ./src/core/admin/...` passes
- [ ] `hsme-cli admin backup` creates file with `engram-YYYYMMDDTHHMMSSZ.db` format (UTC, T separator)
- [ ] `src/core/admin/restore.go` has cleanup BEFORE rename (verify by reading the file)
- [ ] `hsme-cli search-exact "ollama" --format=json --limit 1` returns JSON (not Go map)
- [ ] `hsme-cli search-exact "ollama" --limit 1 --format=json` returns JSON (flags work in any order)
- [ ] `hsme-cli status --format=json | jq .` output includes `"graph": {"nodes": N, "edges": M}`
- [ ] `hsme-cli status --help` shows `--interval duration` flag
- [ ] No new dependencies introduced (C-001)

---

## Risks & Reviewer Guidance

**Risk — T005 (restore order)**: This is a data-safety fix. Verify by reading the actual line order in `restore.go` before and after the edit. The `os.Remove` calls must appear BEFORE `os.Rename`.

**Risk — T006 (flag parsing)**: Complex edge case — boolean flags vs value flags. Test with `--watch` (boolean) and `--limit` (value) in various positions. Ensure pre-parsed flags are not overwritten.

**Risk — T007 (graph stats)**: Make sure the `graph` field is `null` in JSON when the query fails (don't crash). Use `omitempty` in the struct tag.

**Reviewer**: Run all validation commands above. Verify restore order by reading `restore.go` and confirming cleanup lines are before the rename line.

## Activity Log

- 2026-04-26T20:32:23Z – gemini:o3:implementer:implementer – shell_pid=2224354 – Assigned agent via action command
- 2026-04-26T20:37:37Z – gemini:o3:implementer:implementer – shell_pid=2224354 – Ready for review
- 2026-04-26T20:37:44Z – gemini:o3:reviewer:reviewer – shell_pid=2231843 – Started review via action command
- 2026-04-26T20:40:15Z – gemini:o3:reviewer:reviewer – shell_pid=2231843 – Review passed: Remaining bugs fixed and status enhanced. Verified backup naming format (UTC, T separator), restore order (cleanup before rename), trailing flag parsing, and graph stats JSON structure.
