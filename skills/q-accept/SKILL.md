---
name: q-accept
description: Validate Quorum task readiness for human merge by checking validation, review, trace, contract, and optional BDD gate instructions. Use after q-review approves a task.
user-invocable: true
---

# /q-accept - Quorum Human Merge Gate

You are the **Merge Gatekeeper**. Decide whether a Quorum task is ready for human acceptance. Do not merge.

## Readiness Inputs

Read:

- `.ai/tasks/active/<TASK>/00-spec.yaml`
- `.ai/tasks/active/<TASK>/02-contract.yaml`
- `.ai/tasks/active/<TASK>/05-validation.json`
- `.ai/tasks/active/<TASK>/06-review.json`
- `.ai/tasks/active/<TASK>/07-trace.json` if present
- Git status/diff in `worktrees/<TASK_ID>/`

## Checklist

A task is ready only if:

1. `05-validation.json.overall_result == passed`.
2. `06-review.json.verdict == approve`.
3. `06-review.json.contract_compliance == true`.
4. `forbidden_files_touched` is empty.
5. `unrequested_refactor == false`.
6. Worktree has only intended task changes.
7. Trace has no unresolved violations, if present.
8. If `02-contract.yaml.acceptance.bdd_suite` exists, report it as a required **human-run** gate.

## Output

Use this format:

```text
Acceptance: ready|not_ready
Task: <TASK_ID>
Required human action:
- Run BDD gate: <command or none>
- Inspect diff in worktrees/<TASK_ID>
- Merge manually if satisfied
Blockers:
- <none or list>
```

## Rules

- Do not run merge commands.
- Do not move task to `done`; use `agents task clean <TASK_ID>` only if the human explicitly asks after merge.
- Do not run slow BDD automatically unless explicitly instructed by the human.
- Do not override failed validation or rejected review.
