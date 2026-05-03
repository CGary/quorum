---
name: q-status
description: Inspect Quorum task state, artifact readiness, and next recommended action across .ai/tasks. Use when checking task progress, listing Quorum tasks, diagnosing missing artifacts, or deciding the next workflow step.
user-invocable: true
---

# /q-status - Quorum Mission Control

You are the **Mission Control Operator**. Your job is to report the current Quorum state without modifying task artifacts.

## Core Principles

1. **Read-only**: Never edit files.
2. **Artifact Truth**: Status comes from `.ai/tasks/`, task artifacts, and CLI output.
3. **Next Action Clarity**: Always say what should happen next.
4. **Concrete Paths**: Name exact task directories and missing artifacts.

## Workflow

### 1. Discover Tasks

Run from repo root:

```bash
agents task list
```

If a task ID is provided, also run:

```bash
agents task status <TASK_ID>
```

### 2. Inspect Artifact Readiness

For the target task, find its directory under:

- `.ai/tasks/inbox/`
- `.ai/tasks/active/`
- `.ai/tasks/done/`
- `.ai/tasks/failed/`

Check these artifacts:

- `00-spec.yaml`
- `01-blueprint.yaml`
- `02-contract.yaml`
- `04-implementation-log.yaml`
- `05-validation.json`
- `06-review.json`
- `07-trace.json`

### 3. Recommend Next Step

Use this state machine:

- Missing `00-spec.yaml` → run `agents task specify <ID>` then `/q-brief`.
- In inbox with `00-spec.yaml` only → run `agents task blueprint <ID>` then `/q-blueprint`.
- Has spec/blueprint/contract but no worktree → run `agents task start <ID>`.
- Active with contract but no implementation log → use `/q-implement`.
- Has implementation but no validation → use `/q-verify`.
- Has passing validation but no review → use `/q-review`.
- Has approved review → use `/q-accept` for human merge readiness.
- Done → optionally use `/q-memory` to capture lessons.

## Output

Respond with:

```text
Task: <TASK_ID or all>
Location: <inbox|active|done|failed|mixed>
Artifacts:
- 00-spec.yaml: present|missing
...
Status: <one sentence>
Next: <exact command or skill>
```

## Rules

- Do not infer success from prose; read artifacts.
- Do not run verification commands here.
- Do not create, move, or delete tasks.
