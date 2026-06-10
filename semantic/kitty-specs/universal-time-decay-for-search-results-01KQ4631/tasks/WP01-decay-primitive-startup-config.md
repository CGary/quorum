---
work_package_id: WP01
title: Decay Primitive & Startup Config
dependencies: []
requirement_refs:
- FR-001
- FR-004
- FR-006
- FR-008
- FR-009
- FR-010
- C-003
- C-004
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-universal-time-decay-for-search-results-01KQ4631
base_commit: 70a8328789564133a43a98f4ebbbdb0d4e746abd
created_at: '2026-04-26T06:28:29.917592+00:00'
subtasks:
- T001
- T002
- T003
agent: "gemini:1.5-pro:architect:reviewer"
shell_pid: "1833638"
history: []
agent_profile: implementer-ivan
authoritative_surface: src/core/search/
execution_mode: code_change
owned_files:
- src/core/search/decay.go
- cmd/hsme/main.go
- tests/modules/decay_test.go
role: implementer
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Create the reusable decay primitive and configuration surface that every search path will rely on. This WP is the foundation for the mission: if config validation or age math is wrong, every downstream ranking and benchmark result becomes suspect.

## Branch Strategy
Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main.

## Execution Constraint
Follow the existing Go testing style in this repository. Add or update focused tests in `tests/modules/`, and prefer validating config behavior through package-level helpers rather than ad-hoc shell logic.

## Subtasks

### T001: Implement the shared decay math primitive and age helper in `src/core/search/decay.go`
**Purpose**: Centralize the exponential half-life math so both search surfaces use one authoritative implementation.
**Steps**:
1. Add `DecayFactor(ageDays, halfLifeDays float64) float64` with `max(0, ageDays)` clamping for future timestamps.
2. Add a small helper for computing age in days from `time.Time` values if that simplifies the search call sites.
3. Keep the primitive pure: no DB access, env access, or logging.
4. Preserve deterministic floating-point behavior suitable for tests.

### T002: Load and validate `RRF_TIME_DECAY` / `RRF_HALF_LIFE_DAYS` at server startup
**Purpose**: Make the operator-facing config safe and explicit before any query is served.
**Steps**:
1. Define a `DecayConfig` type with `Enabled` and `HalfLifeDays` fields.
2. Implement env parsing with defaults: `off` and `14`.
3. Reject invalid flag values and non-positive half-life values with clear errors.
4. Expose package-level setter/getter helpers if needed to avoid broad signature churn.
5. Update `cmd/hsme/main.go` to load config during startup and exit non-zero on invalid input.

### T003: Add focused unit tests for decay math, future timestamp clamping, and invalid config values
**Purpose**: Lock in the correctness of the primitive and config surface before ranking logic depends on it.
**Steps**:
1. Add tests for `age=0`, `age=half-life`, `age=2*half-life`, and future timestamps.
2. Add tests for default config values when env vars are absent.
3. Add tests for invalid `RRF_TIME_DECAY`, non-numeric half-life, and `<= 0` half-life.
4. Ensure tests are isolated and restore env state between cases.

## Implementation Sketch
- Write the pure math and config types first.
- Add package-level config plumbing second.
- Wire startup last, then finish the unit tests that cover the full contract.

## Risks
- Startup config code accidentally introducing hidden global mutation across tests.
- Future timestamp handling drifting from the spec if age clamping is implemented in multiple places.
- Over-validating in `DecayFactor` instead of validating once at config-load time.

## Definition of Done
- `src/core/search/decay.go` exists and contains the shared primitive/config helpers.
- `cmd/hsme/main.go` fails fast on invalid decay config.
- Unit tests cover the math and config edge cases.
- No search-ranking code is modified yet in this WP.

## Activity Log

- 2026-04-26T06:31:29Z – codex – shell_pid=1828909 – Ready for review
- 2026-04-26T06:31:41Z – gemini:1.5-pro:architect:reviewer – shell_pid=1833638 – Started review via action command
- 2026-04-26T06:31:51Z – gemini:1.5-pro:architect:reviewer – shell_pid=1833638 – Review passed
