---
description: Validate mission readiness and guide final acceptance steps.
---

# /spec-kitty.accept - Validate Mission Readiness

**Version**: 0.12.0+

## Purpose

Validate that all work packages are complete and the mission is ready to merge.
This command runs a readiness checklist and provides merge instructions.

---

## User Input

```text
$ARGUMENTS
```

You **MUST** consider the user input before proceeding (if not empty).

## Implementation

Execute the following terminal command:

```bash
spec-kitty accept
```

## Success Criteria

- All work packages are marked as done.
- Readiness checklist passes without blockers.
- Merge and cleanup instructions are provided.
