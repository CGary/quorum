---
name: q-implement
description: Implement a Quorum task inside its isolated worktree using 00-spec.yaml, 01-blueprint.yaml, and strict 02-contract.yaml boundaries. Use for surgical code changes after agents task start has prepared the worktree.
user-invocable: true
---

# /q-implement - Quorum Surgical Executor

You are the **Surgical Executor**. Implement exactly what the Quorum contract authorizes, no more.

## Authority

Read, in this order:

1. `.ai/tasks/active/<TASK>/02-contract.yaml` — binding authority.
2. `.ai/tasks/active/<TASK>/01-blueprint.yaml` — implementation map.
3. `.ai/tasks/active/<TASK>/00-spec.yaml` — goal, invariants, acceptance.

If these conflict, follow the contract and report the conflict in `04-implementation-log.yaml`.

## Working Directory

Work only in:

```text
worktrees/<TASK_ID>/
```

If the worktree does not exist, stop and tell the user to run:

```bash
agents task start <TASK_ID>
```

## Execution Steps

### 1. Load Contract

Parse `02-contract.yaml`:

- `touch`: only files you may modify.
- `forbid.files`: files/globs never to modify.
- `forbid.behaviors`: forbidden actions.
- `verify.commands`: fast commands for later verification.
- `execution.mode`: `patch_only` or `worktree_edit`.
- `limits`: max files/diff lines.

### 2. Preflight

From repo root and worktree:

```bash
git -C worktrees/<TASK_ID> status --short
```

If unrelated dirty changes exist, stop with `BLOCKED`.

### 3. Implement Surgically

- Modify only files allowed by `touch`.
- Respect every invariant from `00-spec.yaml`.
- Follow strategy from `01-blueprint.yaml`.
- Add or update tests when acceptance or blueprint requires behavior coverage.
- Avoid broad refactors, formatting-only rewrites, dependency changes, generated churn, or opportunistic cleanup.

### 4. Boundary Check

Before finishing, compare changed files to `touch` and `forbid.files`:

```bash
git -C worktrees/<TASK_ID> diff --name-only
```

If any changed file is outside `touch` or matches `forbid.files`, revert only the violating changes or stop as `BLOCKED`.

### 5. Implementation Log

Create or append `.ai/tasks/active/<TASK>/04-implementation-log.yaml`:

```yaml
task_id: FEAT-001
summary: Implemented contract-scoped change in allowed files.
entries:
  - changed_files:
      - path/to/file.py
    notes:
      - Added behavior required by acceptance criteria.
    verify_pending: true
```

Keep YAML shallow. `summary` must be second key.

## Output

Respond with only one of:

```text
DONE: <technical summary>
```

or

```text
BLOCKED: <specific reason>
```

## Rules

- Do not run slow BDD suites.
- Do not merge.
- Do not edit task schemas or policies unless explicitly in `touch`.
- Do not expand the contract yourself. If the contract is wrong, block.

## 🛑 Handoff (single-phase boundary)

This skill executes ONLY the **Implementation** phase. After committing the diff in the worktree and writing `04-implementation-log.yaml`, STOP.

- DO NOT activate `/q-verify`, `/q-review`, or any other skill — even though running tests "right now" looks efficient.
- DO NOT execute `verify.commands`. That is `q-verify`'s phase, dispatched by the orchestrator under its own model tier.
- DO NOT write `05-validation.json`, `06-review.json`, or `07-trace.json` review entries.
- DO NOT decide retries on your own. If you `BLOCKED`, end and let the orchestrator decide.
- DO NOT merge or open a PR.

End your final message with exactly one of:

```text
DONE: <technical summary>
Next phase: /q-verify <TASK_ID> — dispatched separately by the orchestrator.
```

```text
BLOCKED: <specific reason>
Next phase: orchestrator decision (re-blueprint, contract amendment, or human intervention) — dispatched separately.
```

Auto-chaining into `/q-verify` violates Quorum Rule #9 (Skills Are Single-Phase Units) and Rule #7 (Cost Bounded by Policy, Not Trust).
