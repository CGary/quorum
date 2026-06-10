---
work_package_id: WP01
title: FormatText Rewrite
dependencies: []
requirement_refs:
- FR-002
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-hsme-cli-polish-01KQ5PAC
base_commit: 9d65ffa10332509a7c716a87a02fdcb73a5f8c1b
created_at: '2026-04-26T20:26:31.075308+00:00'
subtasks:
- T001
- T002
- T003
agent: "gemini:o3:reviewer:reviewer"
shell_pid: "2221074"
history:
- date: '2026-04-26T20:17:55Z'
  action: tasks generated
  actor: tasks skill
agent_profile: ''
authoritative_surface: cmd/cli/output.go
execution_mode: code_change
model: ''
owned_files:
- cmd/cli/output.go
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

Rewrite `cmd/cli/output.go` to replace the raw `fmt.Sprintf("%v", v)` `FormatText` function with human-readable per-subcommand text formatters. All 7 subcommands must produce readable text output when `--format=text` (the default) is used.

---

## Context

### Feature directory

`/home/gary/dev/hsme/kitty-specs/hsme-cli-polish-01KQ5PAC`

### Bug description

The current `FormatText` is:

```go
// cmd/cli/output.go
func FormatText(v interface{}) string {
    return fmt.Sprintf("%v", v)
}
```

This produces raw Go struct representations like `map[memory_id:1010 status:stored]` instead of human-readable output.

### Files to modify

- `cmd/cli/output.go` — only this file

### What to keep unchanged

- `IsTTY()`, `ShouldColor()`, `FormatJSON()`, `WriteResult()`, `WriteError()` — these work correctly
- ANSI color codes in text output (green for ok, red for errors) — keep using `\033[32m` / `\033[31m` patterns already in the codebase

---

## Guidance per Subtask

### T001 — Rewrite FormatText for `store` result

**Result type** (from `src/core/indexer/indexer.go` or the store handler):
```go
type StoreResult struct {
    MemoryID int64  `json:"memory_id"`
    Status   string `json:"status"`  // e.g. "stored"
}
```

**Current bad output**: `map[memory_id:1010 status:stored]`

**Target good output**: `Stored memory 1011 [note]` or similar readable format.

**Implementation**: Create `FormatStoreResult(r StoreResult) string` and call it from the `store` handler in `cmd/cli/store.go`. Update `store.go` to use the new formatter instead of calling `FormatText` directly.

**Validation**:
```bash
echo "test" | hsme-cli store --source-type note
# Expected: "Stored memory {id} [note]"
# NOT: "map[memory_id:{id} status:stored]"
```

---

### T002 — Rewrite FormatText for `search-fuzzy` and `search-exact`

**Result types** (from `src/core/search/`):
```go
// search-fuzzy
type FuzzyResultEnvelope struct {
    Results []search.MemorySearchResult `json:"results"`
}

type MemorySearchResult struct {
    MemoryID int64   `json:"memory_id"`
    Snippet string   `json:"snippet"`
    Score   float64  `json:"score"`
    // ... other fields
}

// search-exact
type ExactResultEnvelope struct {
    Results []search.ExactMatchResult `json:"results"`
}
```

**Current bad output**: `map[results:[{1 1 0 # Technical...]]` (Go map+struct repr)

**Target good output** — one line per result:
```
#001 — "# Technical Specification: HSME..." (score: 0.92)
#002 — "## 1. Objective..." (score: 0.87)
```

**Implementation**: Create `FormatSearchResults(results []MemorySearchResult) string` and `FormatExactResults(results []ExactMatchResult) string`. Update `cmd/cli/search.go` to use these formatters.

**Validation**:
```bash
hsme-cli search-exact "ollama" --format=text --limit 2
# Expected: one result per line with ID and snippet
# NOT: "map[results:[{...}]]"
```

---

### T003 — Rewrite FormatText for `explore`, `admin backup`, `admin restore`, `admin retry-failed`

**`explore` result**: `search.TraceDependencies` returns a structured graph payload. Format as an indented tree or node/edge table.

**Example target**:
```
Knowledge Graph:
  Node: hsme (entity)
    → depends on: ollama (entity)
    → depends on: sqlite (entity)
  Node: ollama (entity)
    ← depended on by: hsme
```

**`admin backup` result type**:
```go
type AdminBackupResult struct {
    Backup string `json:"backup"`  // path
    Status string `json:"status"`  // "ok"
    Size   int64  `json:"size"`   // bytes
}
```

**Target output**: `Backup created: backups/engram-20260426T160046Z.db (12.2 MB)`

**`admin restore` result type**:
```go
type AdminRestoreResult struct {
    Restore   string `json:"restore"`
    Status   string `json:"status"`
    Integrity string `json:"integrity"`  // "ok"
}
```

**Target output**: `Restored from backups/engram-20260426T160046Z.db — integrity OK`

**`admin retry-failed` result type**:
```go
type AdminRetryResult struct {
    Requeued int64 `json:"requeued"`
    Pending  int64 `json:"pending"`
    Failed  int64 `json:"failed"`
}
```

**Target output**: `Requeued 3 tasks. Queue: 12 pending, 2 failed.`

**Implementation**: Create `FormatExploreResult`, `FormatAdminBackupResult`, `FormatAdminRestoreResult`, `FormatAdminRetryResult` functions. Update the respective handlers to use them.

**Validation**: Run each subcommand with `--format=text` and verify readable output (not Go repr).

---

## Branch Strategy

- **Planning branch**: `main`
- **Final merge target**: `main`
- **Execution worktrees**: Allocated per computed lane from `lanes.json`.

---

## Definition of Done

- [ ] `go test ./cmd/cli/...` passes
- [ ] `echo "test" | hsme-cli store --source-type note` outputs readable text (not `map[...]`)
- [ ] `hsme-cli search-fuzzy "context" --format=text` outputs one line per result
- [ ] `hsme-cli search-exact "context" --format=text` outputs one line per result
- [ ] `hsme-cli admin backup --format=text` outputs readable summary
- [ ] `hsme-cli admin restore --format=text` outputs readable summary
- [ ] `hsme-cli admin retry-failed --format=text` outputs readable summary
- [ ] `hsme-cli explore --format=text` outputs readable graph representation
- [ ] No new dependencies introduced (C-001)

---

## Risks & Reviewer Guidance

**Risk — Color in text output**: If ANSI colors are used in text mode, ensure `ShouldColor()` is respected. Don't hardcode color codes when stdout is not a TTY.

**Reviewer**: Run each subcommand with `--format=text` and verify the output is human-readable. Check that JSON output (`--format=json`) is unchanged and still correct.

## Activity Log

- 2026-04-26T20:26:31Z – gemini:o3:implementer:implementer – shell_pid=2216230 – Assigned agent via action command
- 2026-04-26T20:29:50Z – gemini:o3:implementer:implementer – shell_pid=2216230 – Ready for review
- 2026-04-26T20:29:56Z – gemini:o3:reviewer:reviewer – shell_pid=2221074 – Started review via action command
- 2026-04-26T20:32:16Z – gemini:o3:reviewer:reviewer – shell_pid=2221074 – Review passed: Specialized text formatters implemented and wired correctly.
