---
description: Validate dependencies, finalize WP metadata, and commit all task artifacts.
---

# /spec-kitty.tasks-finalize - Finalize Tasks

**Version**: 0.12.0+

## Purpose

Validate dependencies, finalize Work Package metadata (including requirement
mapping), and commit all task artifacts to the target branch.

---

## User Input

```text
$ARGUMENTS
```

You **MUST** consider the user input before proceeding (if not empty).

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
