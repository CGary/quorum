---
name: spec-kitty.tasks-finalize
description: "Validate dependencies, finalize WP metadata, and commit all task artifacts."
user-invocable: true
---
# /spec-kitty.tasks-finalize - Finalize Tasks

**Version**: 0.12.0+

## Purpose

Validate dependencies, finalize Work Package metadata (including requirement
mapping), and commit all task artifacts to the target branch.

---

## User Input

The content of the user's message that invoked this skill (everything after the skill invocation token, e.g. after `/spec-kitty.<command>` or `$spec-kitty.<command>`) is the User Input referenced elsewhere in these instructions.

You **MUST** consider this user input before proceeding (if not empty).
## Implementation

Execute the following terminal command from the repository root:

```bash
spec-kitty agent mission finalize-tasks --json
```

## Success Criteria

- Dependencies are validated (no cycles, no invalid references).
- Requirement references are validated against the specification.
- Task artifacts are committed to the target branch.
- JSON output confirms success and provide a commit hash.
