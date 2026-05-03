---
description: Check current mission and work package status.
---

# /spec-kitty.status - Check Mission Status

**Version**: 0.12.0+

## Purpose

Check the current status of the mission, including the breakdown of work packages
by lane (kanban status).

---

## User Input

```text
$ARGUMENTS
```

You **MUST** consider the user input before proceeding (if not empty).

## Implementation

Execute the following terminal command:

```bash
spec-kitty agent tasks status
```

## Success Criteria

- Current kanban board is displayed.
- Status of each work package is clearly shown.
- Mission-level completion percentage is updated.
