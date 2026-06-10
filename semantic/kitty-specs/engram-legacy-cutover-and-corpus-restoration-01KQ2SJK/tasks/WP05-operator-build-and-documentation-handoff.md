---
work_package_id: WP05
title: Operator Build and Documentation Handoff
dependencies:
- WP03
- WP04
requirement_refs:
- FR-007
- FR-008
- NFR-005
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
base_branch: kitty/mission-engram-legacy-cutover-and-corpus-restoration-01KQ2SJK
created_at: '2026-04-26T02:52:52Z'
subtasks:
- T018
- T019
- T020
- T021
agent: "gemini:1.5-pro:architect:reviewer"
shell_pid: "1599924"
history: []
agent_profile: implementer-ivan
authoritative_surface: README.md
execution_mode: code_change
owned_files:
- justfile
- .gitignore
- README.md
- docs/legacy-cutover-checklist.md
role: implementer
tags: []
---

## ⚡ Do This First: Load Agent Profile
```bash
spec-kitty agent profile load --id implementer-ivan
```

## Objective
Make the finished migration operable from the repository root without forcing operators to read mission planning artifacts: build targets, repo docs, ignore rules, and a concise cutover checklist should all exist in the main codebase surface.

## Branch Strategy
Current branch at workflow start: main. Planning/base branch for this feature: main. Completed changes must merge back into main. Execution worktrees will later be allocated per computed lane from `lanes.json`; do not assume this WP runs directly in the root checkout.

## Context
This WP is the handoff layer after the underlying migrator and search filter work are complete. It should explain:
- how to build/run the migrator from the repo root
- where reports land
- how backup/delta/cutover work operationally
- what the new `project` filter does
- that Claude Code should write only to HSME after cutover

## Detailed Guidance

### T018: Add `justfile` targets for building/running the migrator and keep migration outputs ignored from Git
**Purpose**: Give operators a discoverable repo-native command surface.

**Steps**:
1. Add `justfile` targets for at least building the migrator and the expected run modes.
2. Keep target names clear and aligned with the README instructions.
3. Update `.gitignore` if needed so generated migration outputs remain untracked while preserving the existing DB-protection rules.
4. Avoid disturbing unrelated build targets.

### T019: Update repository-facing documentation for the migrator workflow, backup expectations, and delta cutover sequence
**Purpose**: Put the operational flow where maintainers naturally look first.

**Steps**:
1. Update `README.md` with a concise section on the migrator workflow.
2. Document full run, manual cutover, delta replay, and report locations.
3. Mention backup expectations and failure posture clearly.
4. Keep the README practical; link to the checklist doc for the step-by-step operator flow.

### T020: Document the new `project` filter and the single-source-of-truth cutover outcome for operators
**Purpose**: Explain the user-visible search improvement and post-cutover ownership model.

**Steps**:
1. Document `project` as an optional search input for both fuzzy and exact search.
2. Explain that after cutover, HSME is the only write target for Claude Code.
3. Clarify that the legacy Engram DB remains read-only historical state, not the active backend.
4. Keep the wording aligned with the mission glossary and avoid ambiguous synonyms.

### T021: Add a concise operator checklist artifact for T0/T+24h cutover verification from the repo root
**Purpose**: Provide a short operational checklist that can be used without reading the full mission quickstart.

**Steps**:
1. Create `docs/legacy-cutover-checklist.md`.
2. Include the minimal step sequence for:
   - dry run
   - full run
   - `claude mcp remove engram`
   - delta replay
   - T0 snapshot
   - T+24h snapshot and diff
3. Keep it short, command-focused, and consistent with the final CLI/justfile names.
4. Mention where reports/checklist outputs should be stored.

## Implementation Sketch
- Add final just targets and ignore rules first.
- Update README next.
- Finish with the concise checklist once command names and outputs are fully settled.

## Risks
- Documentation drifting from actual CLI targets or flags.
- Forgetting to explain the manual Claude MCP reconfiguration boundary.
- Accidentally versioning generated migration output.

## Definition of Done
- Repo root exposes usable migrator commands.
- `.gitignore` safely excludes migration outputs.
- README covers the migrator, cutover, and project-filter behavior.
- A concise operator checklist exists under `docs/`.

## Reviewer Guidance
Verify that:
- documented command names match the final `justfile` and binary flags
- checklist steps reflect the real fail-loud cutover order
- generated data is not accidentally tracked

## Activity Log

- 2026-04-26T03:35:35Z – gemini:1.5-pro:architect:implementer – shell_pid=1597145 – Started implementation via action command
- 2026-04-26T03:37:25Z – gemini:1.5-pro:architect:implementer – shell_pid=1597145 – Operator build targets, documentation, and cutover checklist implemented. The system is ready for the manual cutover process.
- 2026-04-26T03:37:31Z – gemini:1.5-pro:architect:reviewer – shell_pid=1599924 – Started review via action command
- 2026-04-26T03:37:44Z – gemini:1.5-pro:architect:reviewer – shell_pid=1599924 – Review passed: Operator-facing documentation, build targets, and cutover checklist are clear, accurate, and ready for use in production.
