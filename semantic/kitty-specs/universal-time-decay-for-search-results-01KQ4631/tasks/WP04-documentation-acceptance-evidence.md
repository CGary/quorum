---
work_package_id: WP04
title: Documentation & Acceptance Evidence
dependencies:
- WP03
requirement_refs:
- FR-014
- NFR-002
- NFR-003
- NFR-004
- NFR-007
- C-001
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: main
base_commit: cbad757af0b1f5be2f80e9569e196baa712c24db
created_at: '2026-04-26T06:20:47Z'
subtasks:
- T011
- T012
agent: "gemini:1.5-pro:architect:reviewer"
shell_pid: "1854008"
history: []
agent_profile: implementer-ivan
authoritative_surface: data/benchmarks/
execution_mode: code_change
owned_files:
- README.md
- data/benchmarks/**
role: implementer
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Document how operators enable, benchmark, validate, and roll back time-decay, then preserve at least one benchmark run as auditable acceptance evidence.

## Branch Strategy
Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge into main.

## Execution Constraint
Do not change the frozen eval-set or frozen baseline artifacts. Documentation and evidence must describe how to use them, not mutate them.

## Subtasks

### T011: Document runtime usage, env vars, benchmark invocation, and rollback flow in `README.md`
**Purpose**: Make the new feature safe to operate without reading the mission docs first.
**Steps**:
1. Add a dedicated README section for `RRF_TIME_DECAY` and `RRF_HALF_LIFE_DAYS`.
2. Document the `cmd/bench-decay` invocation and the meaning of the generated reports.
3. Explain the default-off safety posture and the one-step rollback path.
4. Mention the exact-search sampling requirement so reviewers know what to look for in reports.

### T012: Produce and retain at least one benchmark audit run showing mission acceptance metrics
**Purpose**: Leave behind concrete evidence that the selected half-life meets the spec thresholds.
**Steps**:
1. Run the harness against the current corpus using the documented defaults or the accepted tuned half-life.
2. Preserve the resulting run directory under `data/benchmarks/` (or commit a stable sample if the repo allows it).
3. Confirm the report explicitly covers pure-recency gain, adversarial preservation, and decay-off byte-equivalence.
4. Note any remaining caveats in the README or verification notes if thresholds require human follow-up.

## Implementation Sketch
- Update README once the harness interface is final.
- Run the benchmark after code is stable.
- Preserve the output and sanity-check the reported thresholds before closing the mission.

## Risks
- Documentation drifting from the actual CLI flags or env semantics.
- Forgetting to preserve a benchmark run, leaving acceptance without audit evidence.
- Treating a partial or failed benchmark as mission-ready evidence.

## Definition of Done
- README explains the full operator workflow.
- At least one benchmark evidence run is retained.
- Acceptance notes can point directly to concrete report files.

## Activity Log

- 2026-04-26T06:41:51Z – gemini:1.5-pro:architect:implementer – shell_pid=1847351 – Started implementation via action command
- 2026-04-26T06:47:02Z – gemini:1.5-pro:architect:implementer – shell_pid=1847351 – Documentation and reports complete
- 2026-04-26T06:47:03Z – gemini:1.5-pro:architect:reviewer – shell_pid=1854008 – Started review via action command
- 2026-04-26T06:47:04Z – gemini:1.5-pro:architect:reviewer – shell_pid=1854008 – Review passed
