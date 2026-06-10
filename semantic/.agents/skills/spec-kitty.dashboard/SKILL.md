---
name: spec-kitty.dashboard
description: Open the Spec Kitty dashboard in your browser.
user-invocable: true
---
# /spec-kitty.dashboard - Open Dashboard

**Version**: 0.12.0+

## Purpose

Launch the Spec Kitty dashboard in your browser to visualize mission progress,
kanban lanes, and work package status.

---

## User Input

The content of the user's message that invoked this skill (everything after the skill invocation token, e.g. after `/spec-kitty.<command>` or `$spec-kitty.<command>`) is the User Input referenced elsewhere in these instructions.

You **MUST** consider this user input before proceeding (if not empty).
## Implementation

Execute the following terminal command:

```bash
spec-kitty dashboard
```

## Additional Options

- To specify a preferred port: `spec-kitty dashboard --port 8080`
- To stop the dashboard: `spec-kitty dashboard --kill`

## Success Criteria

- Browser opens automatically to the dashboard.
- Dashboard URL is clearly displayed in the console.
