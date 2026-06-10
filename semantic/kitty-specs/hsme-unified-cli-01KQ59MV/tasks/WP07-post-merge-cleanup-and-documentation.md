---
work_package_id: WP07
title: Post-Merge Cleanup and Documentation
dependencies:
- WP06
requirement_refs:
- NFR-005
planning_base_branch: main
merge_target_branch: main
branch_strategy: Planning artifacts for this feature were generated on main. During /spec-kitty.implement this WP may branch from a dependency-specific base, but completed changes must merge back into main unless the human explicitly redirects the landing branch.
subtasks: []
agent: "gemini:o3:reviewer:reviewer"
shell_pid: "2117745"
history:
- date: '2026-04-26T16:47:42Z'
  action: tasks generated
  actor: tasks skill
agent_profile: ''
authoritative_surface: kitty-specs/hsme-unified-cli-01KQ59MV/
execution_mode: planning_artifact
model: ''
owned_files:
- kitty-specs/hsme-unified-cli-01KQ59MV/meta.json
- kitty-specs/hsme-unified-cli-01KQ59MV/status.json
- kitty-specs/hsme-unified-cli-01KQ59MV/status.events.jsonl
role: ''
tags: []
---

## ⚡ Do This First: Load Agent Profile

Before reading anything else, load the curator profile (this WP is about cleanup and documentation):

```
/ad-hoc-profile-load curator
```

This injects your role identity, skill directives, and execution context. All other instructions in this prompt are subordinate to the profile load.

---

## Objective

Wrap up the mission: archive it, sync delta specs to main, update skill registry if new patterns were introduced.

---

## Context

### Feature directory

`/home/gary/dev/hsme/kitty-specs/hsme-unified-cli-01KQ59MV`

### Dependencies

- **WP06 must be complete** — verification must pass before archiving.

---

## Guidance

### Archive mission

After WP06 verification passes:

1. Run the spec-kitty archive step (or equivalent `sdd-archive` if configured).
2. This syncs delta specs to main specs and archives the completed mission.

**If no archive command is available**: Manually mark mission as complete by adding `status: completed` to `meta.json`.

### Sync delta specs to main

The delta specs (spec.md, plan.md, data-model.md, research.md, contracts/, quickstart.md) are the mission's authoritative documentation. After merge, they become the record of what was built.

No action needed if spec-kitty handles this automatically.

### Skill registry update

If this mission introduced new patterns (e.g., CLI subcommand pattern, admin operations package, bootstrap package pattern), check if the skill registry needs updating.

**New patterns introduced**:
- `cmd/cli/` subcommand dispatcher (stdlib `flag` + manual dispatch pattern)
- `src/core/admin/` package
- `src/bootstrap/` package

**Check**: Run `spec-kitty profiles list` or look at `.kittify/` for skill registry location. If there's a `skill-registry.md`, update it with the new patterns if they're worth documenting.

---

## Definition of Done

- [ ] Mission archived in spec-kitty system (or `meta.json` updated to `status: completed`)
- [ ] Delta specs synced to main (if automatic)
- [ ] Skill registry updated if new patterns warrant it
- [ ] `ls kitty-specs/hsme-unified-cli-01KQ59MV/` shows completed mission artifacts

---

## Risks & Reviewer Guidance

**Risk — No archive command available**: If `spec-kitty archive` or `sdd-archive` doesn't exist, manually mark the mission complete. Don't fail on missing tooling.

**Risk — Skill registry location unknown**: If you can't find the skill registry, skip this step. It's optional.

**Reviewer**: After this WP, the mission directory should be marked as complete and any new patterns should be captured in the skill registry.

## Activity Log

- 2026-04-26T18:21:45Z – gemini:o3:curator:curator – shell_pid=2117479 – Started implementation via action command
- 2026-04-26T18:21:56Z – gemini:o3:curator:curator – shell_pid=2117479 – Cleanup and documentation complete.
- 2026-04-26T18:21:57Z – gemini:o3:reviewer:reviewer – shell_pid=2117745 – Started review via action command
- 2026-04-26T18:21:58Z – gemini:o3:reviewer:reviewer – shell_pid=2117745 – Approved.
