---
name: spec-kitty.merge
description: Merge a completed mission into the target branch.
user-invocable: true
---
# /spec-kitty.merge - Merge Mission

**Version**: 0.12.0+

## Purpose

Merge all completed work packages and mission artifacts into the target branch.
This is the final step in the mission lifecycle after acceptance.

---

## User Input

The content of the user's message that invoked this skill (everything after the skill invocation token, e.g. after `/spec-kitty.<command>` or `$spec-kitty.<command>`) is the User Input referenced elsewhere in these instructions.

You **MUST** consider this user input before proceeding (if not empty).
## Implementation

Execute the following terminal command:

```bash
spec-kitty merge
```

## Success Criteria

- All changes from the mission branch are merged into the target branch.
- Mission is marked as merged in its status.
- Cleanup of temporary worktrees and branches may be performed.
