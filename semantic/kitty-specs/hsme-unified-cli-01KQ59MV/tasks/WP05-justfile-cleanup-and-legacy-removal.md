---
work_package_id: WP05
title: Justfile Cleanup and Legacy File Removal
dependencies:
- WP02
requirement_refs:
- FR-070
- FR-071
- FR-072
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
subtasks:
- T024
- T025
- T026
agent: "gemini:o3:reviewer:reviewer"
shell_pid: "2112270"
history:
- date: '2026-04-26T16:47:42Z'
  action: tasks generated
  actor: tasks skill
agent_profile: ''
authoritative_surface: justfile
execution_mode: code_change
model: ''
owned_files:
- justfile
role: ''
---
---

## ⚡ Do This First: Load Agent Profile

Before reading anything else, load the implementer agent profile:

```
/ad-hoc-profile-load implementer
```

This injects your role identity, skill directives, and execution context. All other instructions in this prompt are subordinate to the profile load.

---

## Objective

Refactor the `justfile` to delegate `status`, `backup`, `restore`, `retry-failed` to the new CLI, and remove the shell scripts that are now superseded.

---

## Context

### Feature directory

`/home/gary/dev/hsme/kitty-specs/hsme-unified-cli-01KQ59MV`

### Dependencies

- **WP02 must be complete** — the `hsme-cli` binary must exist and be runnable before these wrappers can be tested.
- Read `research.md` R8 for the cleanup pattern.

### What this WP modifies/deletes

```
justfile                    # status/backup/restore/retry-failed → wrappers
scripts/status.sh           # DELETE after parity verification
ideas/cli-tool.md           # DELETE after spec/plan supersede
```

---

## Guidance per Subtask

### T024 — Refactor `justfile` targets

**File**: `justfile` (modify)

**Current targets** (approximate — find the actual definitions):
```
status:        ; @bash scripts/status.sh
backup:        ; @sqlite3 ...
restore:       ; @sqlite3 ...
retry-failed:  ; @bash -c ...
```

**New targets** (R8 from research.md):
```just
status:       ; @./hsme-cli status
backup:       ; @./hsme-cli admin backup
restore:      ; @./hsme-cli admin restore --latest
retry-failed: ; @./hsme-cli admin retry-failed
```

**Verification**: After updating, run `just status` and confirm it prints the same information as before (parity check). Compare output with `bash scripts/status.sh` — all fields the bash script shows must be present in the CLI output.

**Keep**: Other targets (`build`, `test`, `install`, etc.) are untouched.

**Check**: Before modifying, read the current `justfile` to find the exact `status`, `backup`, `restore`, `retry-failed` target definitions. The SQL in `backup`/`restore`/`retry-failed` may be inline in the justfile.

---

### T025 — Remove `scripts/status.sh`

**File**: `scripts/status.sh` (delete)

**Precondition**: Parity verified in T024. Once `just status` works identically via the CLI wrapper, the bash script is redundant.

**Verification before delete**: Run both and compare:
```bash
just status
bash scripts/status.sh
```

The output must contain the same fields:
- Worker online/offline state
- Queue counts (pending, processing, completed, failed)
- Graph counts (nodes, edges)

If any field is missing from `hsme-cli status`, fix WP02 before deleting this script.

**Deletion**: `rm scripts/status.sh`

---

### T026 — Remove `ideas/cli-tool.md`

**File**: `ideas/cli-tool.md` (delete)

**Rationale**: The spec.md and plan.md for this mission supersede the earlier design doc. The spec/plan are authoritative; the ideas file is now stale.

**Deletion**: `rm ideas/cli-tool.md`

**Check**: Verify this file exists before attempting deletion (`ls ideas/`).

---

## Branch Strategy

- **Planning branch**: `main`
- **Final merge target**: `main`
- **Execution worktrees**: Allocated per computed lane from `lanes.json`.

---

## Definition of Done

- [ ] `just status` works via CLI wrapper
- [ ] `just backup` works via CLI wrapper
- [ ] `just restore` works via CLI wrapper
- [ ] `just retry-failed` works via CLI wrapper
- [ ] `scripts/status.sh` deleted
- [ ] `ideas/cli-tool.md` deleted
- [ ] `just` commands all still work (quick smoke test)

---

## Risks & Reviewer Guidance

**Risk — Parity gap**: If `hsme-cli status` output is missing fields that `scripts/status.sh` provided, do NOT delete the script yet. Report the gap and fix WP02 first.

**Risk — `ideas/cli-tool.md` might not exist**: Check `ls ideas/` first. If the file doesn't exist, skip deletion and note it.

**Reviewer**: After T024, run `just status` and verify output matches expected fields. After T025 and T026, verify the files are gone and `just` still works.

## Activity Log

- 2026-04-26T18:17:18Z – gemini:o3:implementer:implementer – shell_pid=2110010 – Started implementation via action command
- 2026-04-26T18:18:58Z – gemini:o3:implementer:implementer – shell_pid=2110010 – Refactored justfile and removed legacy files.
- 2026-04-26T18:19:03Z – gemini:o3:reviewer:reviewer – shell_pid=2112270 – Started review via action command
- 2026-04-26T18:19:14Z – gemini:o3:reviewer:reviewer – shell_pid=2112270 – Review passed: All legacy scripts removed and justfile updated (including watch-status).
