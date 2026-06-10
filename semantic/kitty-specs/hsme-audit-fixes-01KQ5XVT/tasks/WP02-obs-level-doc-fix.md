---
work_package_id: WP02
title: Fix OBS_LEVEL Documentation Mismatch (FR-002)
dependencies: []
requirement_refs:
- FR-002
- FR-004
- NFR-002
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-hsme-audit-fixes-01KQ5XVT
base_commit: dac3ef13d27e4bdafeedb434cc01af9c2c24f149
created_at: '2026-04-27T11:39:33.462997+00:00'
subtasks:
- T004
- T005
agent: "gemini:flash:reviewer:reviewer"
shell_pid: "2811773"
history: []
authoritative_surface: README.md
execution_mode: code_change
owned_files:
- README.md
- tests/bdd/observability_env.feature
role: implementer
tags: []
---

## ⚡ Do This First: Load Agent Profile

```bash
spec-kitty agent profile load --id implementer-ivan
```

If implementer-ivan is not available, load the best available implementer profile:
```bash
spec-kitty agent profile list --json
```

## Objective

Fix FR-002: The README documents `OBS_LEVEL` as the environment variable for observability, but the code correctly reads `HSME_OBS_LEVEL`. This is a documentation-only fix — the code at `src/observability/config.go:32` already reads `HSME_OBS_LEVEL` correctly. The bug is that anyone following the README ends up with observability silently OFF.

## Branch Strategy

Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main. Execution worktrees are allocated per computed lane from `lanes.json`.

## Context from Spec and Plan

**Root cause** (confirmed from HSME memory 1015):
- `src/observability/config.go:32` reads: `Level: parseLevel(os.Getenv("HSME_OBS_LEVEL"))`
- `README.md:127` documents: `"env": { "OBS_LEVEL": "basic" }`
- `grep -rn "HSME_OBS_LEVEL\|OBS_LEVEL"` shows exactly two hits — the code and the README
- All sibling vars are correctly prefixed: `HSME_OBS_SAMPLE_RATE`, `HSME_OBS_RAW_RETENTION_DAYS`, etc.
- This is 100% a documentation bug — no code change needed

**Fix**: Replace `OBS_LEVEL` → `HSME_OBS_LEVEL` in README.md

**Key constraints**:
- C-002: Fix must be documentation (README update) + verification that HSME_OBS_LEVEL is the only variable read

**HSME Memory**: 1015 (root cause), 988 (observability runtime)

## Subtasks

### T004: Fix README.md — Change OBS_LEVEL to HSME_OBS_LEVEL

**Purpose**: Fix the documented environment variable name in README.

**Steps**:
1. Read `README.md` and locate line ~127 where `OBS_LEVEL` appears in the MCP client config example
2. Replace ALL occurrences of `OBS_LEVEL` with `HSME_OBS_LEVEL` in the README:
   - The MCP client config example: `"env": { "HSME_OBS_LEVEL": "basic" }`
   - Any other references to OBS_LEVEL in the README
3. grep to confirm no remaining `OBS_LEVEL` references:
   ```bash
   grep -n "OBS_LEVEL" README.md
   ```
   (should return zero results — only HSME_OBS_LEVEL should remain)
4. Verify all sibling variables still use `HSME_OBS_*` prefix:
   ```bash
   grep -n "HSME_OBS_" README.md
   ```
5. Verify the code at `src/observability/config.go:32` reads `HSME_OBS_LEVEL` (it should — this is confirm-only):
   ```bash
   grep -n "HSME_OBS_LEVEL\|OBS_LEVEL" src/observability/config.go
   ```
   (should show only HSME_OBS_LEVEL being read)

**Files to modify**:
- `README.md`

**Validation**:
- [ ] `grep "OBS_LEVEL" README.md` returns 0 results
- [ ] All observability env vars in README use `HSME_OBS_*` prefix
- [ ] Code at config.go:32 reads only `HSME_OBS_LEVEL` (no code change, just verification)

**Risk**: Low — only documentation. But must be thorough (grep all occurrences).

---

### T005: Add godog BDD Test — OBS_LEVEL Wrong vs HSME_OBS_LEVEL Correct

**Purpose**: NFR-002 — Add a BDD scenario that FAILS before the doc fix and PASSES after.

Note: The test doesn't test the code (code is correct). It tests that:
1. Using the documented variable (`HSME_OBS_LEVEL`) produces observability data
2. Using the old wrong variable (`OBS_LEVEL`) produces no observability data

**Steps**:
1. Create `tests/bdd/observability_env.feature`:

```gherkin
Feature: Observability environment variable

  Scenario: HSME_OBS_LEVEL produces trace data (correct variable)
    Given the hsme process is started with env HSME_OBS_LEVEL=trace
    And the user performs store and search operations
    Then obs_traces has > 0 rows
    And obs_spans has > 0 rows
    And obs_events has > 0 rows

  Scenario: OBS_LEVEL (wrong) produces no trace data
    Given the hsme process is started with env OBS_LEVEL=trace
    And the user performs store and search operations
    Then obs_traces has 0 rows
    And obs_spans has 0 rows
    And obs_events has 0 rows

  Scenario: README correctly documents HSME_OBS_LEVEL
    Given a user reads the README observability section
    When they configure HSME_OBS_LEVEL as documented
    Then observability produces data as expected
```

2. Create `tests/bdd/observability_env_test.go` — godog test suite that:
   - Starts hsme process with wrong env (`OBS_LEVEL=trace`) — verifies 0 rows in obs_* tables
   - Starts hsme process with correct env (`HSME_OBS_LEVEL=trace`) — verifies 6+/33+/2+ rows
   - Can use the existing isolated test setup from audit (tmp dir, fresh DB)
3. Run `godog run tests/bdd/observability_env.feature` — the second scenario (wrong var) will FAIL because README says `OBS_LEVEL` works (but it doesn't). After T004 fix, README will say `HSME_OBS_LEVEL` and the test scenarios align with reality.

**Files to create**:
- `tests/bdd/observability_env.feature`
- `tests/bdd/observability_env_test.go`

**Validation**:
- [ ] godog scenario "HSME_OBS_LEVEL produces trace data" PASSES (correct var works)
- [ ] godog scenario "OBS_LEVEL (wrong) produces no trace data" PASSES (wrong var doesn't work — this is expected behavior)
- [ ] After T004, README correctly documents HSME_OBS_LEVEL so new users aren't misled

**Note**: The "wrong var" scenario passing is actually the EXPECTED result — it documents that `OBS_LEVEL` doesn't work. The doc fix (T004) ensures users use the right variable from the start.

## Definition of Done

- [ ] `grep "OBS_LEVEL" README.md` returns 0 results (only HSME_OBS_LEVEL remains)
- [ ] godog test PASSES showing HSME_OBS_LEVEL works and OBS_LEVEL doesn't
- [ ] Code at config.go verified to read only HSME_OBS_LEVEL (no code change needed)
- [ ] No files outside `owned_files` modified

## Risks

| Risk | Mitigation |
|------|------------|
| Remaining OBS_LEVEL in some edge section of README | Full grep before marking done |
| Test setup complexity (process spawn, DB isolation) | Use the same isolated test approach from the audit (tmp dir, fresh DB) |

## Reviewer Guidance

- Verify T004 is purely documentation — no Go code changed
- Run `grep "OBS_LEVEL" README.md` — must be 0 results
- Run `grep "HSME_OBS_" README.md` — should show all correctly-prefixed vars
- Run `godog run tests/bdd/observability_env.feature` — should pass
- The code at `src/observability/config.go` should NOT be modified (already correct)

## Activity Log

- 2026-04-27T11:51:15Z – claude – shell_pid=2481918 – Fixed FR-002 by correcting README.md variable name. Added BDD scenarios verifying that HSME_OBS_LEVEL works correctly while OBS_LEVEL does not produce traces.
- 2026-04-27T15:41:07Z – gemini:flash:reviewer:reviewer – shell_pid=2811773 – Started review via action command
- 2026-04-27T15:47:26Z – gemini:flash:reviewer:reviewer – shell_pid=2811773 – Review passed: README correctly updated to document HSME_OBS_LEVEL. BDD tests verify that the correct variable enables observability while the previous one doesn't. Restored .mcp.json which was accidentally deleted.
