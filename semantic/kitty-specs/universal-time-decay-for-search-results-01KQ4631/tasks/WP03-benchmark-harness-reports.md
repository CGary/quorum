---
work_package_id: WP03
title: Benchmark Harness & Reports
dependencies:
- WP01
- WP02
requirement_refs:
- FR-011
- FR-012
- FR-013
- FR-015
- NFR-001
- NFR-002
- NFR-005
- NFR-006
- C-005
- C-006
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: main
base_commit: cbad757af0b1f5be2f80e9569e196baa712c24db
created_at: '2026-04-26T06:20:47Z'
subtasks:
- T008
- T009
- T010
agent: "gemini:1.5-pro:architect:reviewer"
shell_pid: "1846934"
history: []
agent_profile: implementer-ivan
authoritative_surface: cmd/bench-decay/
execution_mode: code_change
owned_files:
- cmd/bench-decay/**
- tests/modules/bench_decay_test.go
role: implementer
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Add the standalone benchmark tool that proves the new ranking behavior is worth enabling and that the safety thresholds remain intact.

## Branch Strategy
Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main.

## Execution Constraint
Treat the frozen eval set and baseline files as read-only inputs. The harness may read them, copy results into reports, and fail loudly on mismatches, but it must not rewrite those sources.

## Subtasks

### T008: Build `cmd/bench-decay` to run paired OFF/ON evaluations against the frozen corpus
**Purpose**: Provide a reusable operational binary rather than a one-off test command.
**Steps**:
1. Add the CLI entry point and flag parsing for DB/eval/baseline/half-life/output paths.
2. Open SQLite in read-only mode.
3. Run the full frozen query set in paired mode: decay OFF and decay ON.
4. Ensure the harness can invoke both `search_fuzzy` and sampled `search_exact` paths against the same snapshot.

### T009: Emit JSON and Markdown benchmark reports under `data/benchmarks/<run_id>/` and include at least 5 `search_exact` samples
**Purpose**: Preserve reviewable evidence, not just console output.
**Steps**:
1. Define internal result/report structs that mirror the baseline layout closely enough for diffing.
2. Write `report.json`, `delta.json`, and `report.md` under a generated run directory.
3. Record category metrics, rank deltas, promoted/demoted memories, and exact-search sample ordering.
4. Surface the threshold checks clearly in the Markdown summary.

### T010: Add harness smoke coverage for CLI/report generation and read-only DB access
**Purpose**: Keep the tool from silently regressing after the mission lands.
**Steps**:
1. Add a smoke test that exercises argument parsing and output generation against a controlled DB fixture.
2. Add coverage that the harness refuses invalid half-life values.
3. Verify read-only DB opening behavior and basic report file creation.

## Implementation Sketch
- Create the CLI and internal runner types first.
- Implement paired execution and metric aggregation second.
- Add report writers third.
- Finish with smoke coverage for the operator-facing behavior.

## Risks
- Benchmark code accidentally reusing process-global config in a way that leaks state between OFF and ON runs.
- Report schema drifting too far from the frozen baseline to compare safely.
- Read-only DB flags behaving differently across environments.

## Definition of Done
- `cmd/bench-decay` exists and is runnable with the documented flags.
- A run writes JSON + Markdown reports under `data/benchmarks/<run_id>/`.
- At least 5 exact-search samples are included.
- Smoke coverage exists for the basic CLI/report path.

## Activity Log

- 2026-04-26T06:36:02Z – gemini:1.5-pro:architect:implementer – shell_pid=1839836 – Started implementation via action command
- 2026-04-26T06:41:37Z – gemini:1.5-pro:architect:implementer – shell_pid=1839836 – Ready for review
- 2026-04-26T06:41:37Z – gemini:1.5-pro:architect:reviewer – shell_pid=1846934 – Started review via action command
- 2026-04-26T06:41:38Z – gemini:1.5-pro:architect:reviewer – shell_pid=1846934 – Review passed
