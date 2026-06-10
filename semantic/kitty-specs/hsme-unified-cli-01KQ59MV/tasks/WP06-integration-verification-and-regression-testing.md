---
work_package_id: WP06
title: Integration Verification and Regression Testing
dependencies:
- WP04
- WP05
requirement_refs:
- NFR-003
- NFR-004
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
subtasks:
- T027
- T028
- T029
agent: "gemini:o3:reviewer:reviewer"
shell_pid: "2116487"
history:
- date: '2026-04-26T16:47:42Z'
  action: tasks generated
  actor: tasks skill
agent_profile: ''
authoritative_surface: tests/modules/
execution_mode: code_change
model: ''
owned_files:
- tests/modules/...
- log/...
role: ''
tags: []
---

## ⚡ Do This First: Load Agent Profile

Before reading anything else, load the implementer agent profile:

```
/ad-hoc-profile-load implementer
```

This injects your role identity, skill directives, and execution context. All other instructions in this prompt are subordinate to the profile load.

---

## Objective

Verify the full end-to-end operator loop works without bash, restore safety is guaranteed, and all existing tests still pass.

---

## Context

### Feature directory

`/home/gary/dev/hsme/kitty-specs/hsme-unified-cli-01KQ59MV`

### Dependencies

- **WP04 (admin operations)** must be complete before T028 can run
- **WP05 (justfile cleanup)** must be complete before T029 can run
- This is a verification WP — no new code, only validation

### What this WP verifies

- `just test` passes with zero regressions (NFR-004)
- Restore rejects corrupt backup 100% of the time
- Operator daily ops loop works end-to-end without bash

---

## Guidance per Subtask

### T027 — Run full test suite

**Command**: `just test`

**Expected**: All tests pass. No new failures, no new skips added.

**If failures occur**: These are regressions. Investigate and fix before proceeding. The bootstrap refactor (WP01) and new CLI (WP02) are the most likely sources of regression.

**Scope**: Run with build tags `sqlite_fts5 sqlite_vec` as the project requires.

**What to check**:
- `go test ./src/bootstrap/...` — WP01 tests
- `go test ./cmd/cli/...` — WP02 tests
- `go test ./src/core/admin/...` — WP04 tests
- `go test ./tests/modules/...` — integration tests
- Any other `go test` targets the project has

---

### T028 — Verify restore refuses corrupt backup

**Test**: Corrupt backup rejection must be 100% effective.

**Steps**:
1. Create a valid backup first: `hsme-cli admin backup`
2. Corrupt it: `echo "this is not a valid SQLite DB" > backups/engram-xxx.db`
3. Attempt restore: `hsme-cli admin restore --from backups/engram-xxx.db`
4. Verify: Exit code must be `2`, error message must mention integrity check failure, live DB must be UNCHANGED (not overwritten with corrupt data)

**Additional test**:
- Create a backup
- Truncate it (partial/corrupt): `truncate -s 100 backups/engram-xxx.db`
- Attempt restore
- Verify: Same behavior — exit 2, clear message, DB untouched

**Verification**: Run this test twice with different corruption methods. Both must be rejected.

---

### T029 — Verify operator daily ops loop

**Test**: Complete operator loop without any bash dependency.

**Sequence to run manually**:

```bash
# 1. Check system health
hsme-cli status

# 2. Process failed tasks
hsme-cli admin retry-failed

# 3. Create a backup
hsme-cli admin backup

# 4. Search memories
hsme-cli search-fuzzy "context" --limit 5
hsme-cli search-exact "context" --limit 5

# 5. Explore graph
hsme-cli explore --entity-name "hsme" --direction upstream

# 6. Store a note
echo "test memory" | hsme-cli store --source-type note

# 7. Restore from latest backup
hsme-cli admin restore --latest

# 8. Verify JSON output is parseable
hsme-cli status --format=json | jq .
hsme-cli search-fuzzy "test" --format=json | jq .
```

**Verify**:
- All commands exit 0 (except restore with no backups → exit 2 is OK)
- JSON output is valid and parseable by `jq`
- `status --watch` works in a real terminal (if available)
- No bash scripts are invoked as part of the loop
- `just status`, `just backup`, `just restore`, `just retry-failed` all work as wrappers

**Success criteria** (from spec.md section "Success Criteria"):
1. Operator can run complete daily ops loop without bash ✓
2. `scripts/status.sh` is removed (WP05) ✓
3. justfile targets for backup/restore/retry-failed are reduced to wrappers (WP05) ✓
4. Bootstrap consumed by all four binaries (WP01) ✓
5. Existing tests pass with zero regressions ✓
6. Restore reliably refuses corrupt backup 100% of cases ✓
7. Engineers can pipe CLI output to shell automation ✓

---

## Branch Strategy

- **Planning branch**: `main`
- **Final merge target**: `main`
- **Execution worktrees**: Allocated per computed lane from `lanes.json`.

---

## Definition of Done

- [ ] `just test` passes with zero failures
- [ ] Corrupt backup is rejected with exit 2, clear message, DB untouched
- [ ] Daily ops loop works end-to-end via CLI
- [ ] JSON outputs are valid and `jq`-parseable
- [ ] `just status`, `just backup`, `just restore`, `just retry-failed` all work

---

## Risks & Reviewer Guidance

**Risk — Test failures**: If `just test` fails, this WP blocks merge. Investigate each failure as a regression from the bootstrap refactor or CLI addition.

**Risk — Corrupt restore safety**: This is a critical data safety requirement. Verify with two different corruption methods before declaring done.

**Reviewer**: After T027, confirm test output. After T028, verify the two corruption test cases. After T029, confirm the full ops loop works and JSON is parseable.

---

## Implementation Notes

- This WP is verification only — no new code should be written unless a test reveals a bug
- If a bug is found during verification, fix it in the appropriate earlier WP, then re-run verification
- Document any discrepancies found during verification

## Activity Log

- 2026-04-26T18:19:20Z – gemini:o3:implementer:implementer – shell_pid=2112714 – Started implementation via action command
- 2026-04-26T18:21:22Z – gemini:o3:implementer:implementer – shell_pid=2112714 – All verification tasks passed. Full test suite is green. Restore safety verified.
- 2026-04-26T18:21:27Z – gemini:o3:reviewer:reviewer – shell_pid=2116487 – Started review via action command
- 2026-04-26T18:21:39Z – gemini:o3:reviewer:reviewer – shell_pid=2116487 – Review passed: Full test suite is green. Integration and regression testing verified.
