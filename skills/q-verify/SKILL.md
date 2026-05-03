---
name: q-verify
description: Run a Quorum task's fast verification commands from 02-contract.yaml and capture results in 05-validation.json. Use after implementation or whenever validation evidence is needed.
user-invocable: true
---

# /q-verify - Quorum Functional Verifier

You are the **Functional Verifier**. Tests are the only proof of work.

## Authority

Use `.ai/tasks/active/<TASK>/02-contract.yaml` as the source of verification commands.

## Workflow

### 1. Preflight

Confirm:

- Task exists under `.ai/tasks/active/<TASK>/`.
- `02-contract.yaml` exists and has `verify.commands`.
- Worktree exists at `worktrees/<TASK_ID>/`.

If not, stop with `blocked`.

### 2. Execute Fast Verify Commands

For each command in `verify.commands`, run it from the task worktree:

```bash
cd worktrees/<TASK_ID>
<command>
```

Capture:

- command
- exit code
- duration seconds
- output excerpt up to 2000 chars

Do not run `acceptance.bdd_suite`; that is a human merge gate.

### 3. Write `05-validation.json`

Write `.ai/tasks/active/<TASK>/05-validation.json` matching `.agents/schemas/validation.schema.json`:

```json
{
  "task_id": "FEAT-001",
  "summary": "Fast verification passed for contract commands.",
  "executed_at": "2026-04-28T00:00:00Z",
  "commands": [
    {
      "command": "pytest tests/foo.py",
      "exit_code": 0,
      "duration_s": 1.23,
      "output_excerpt": "..."
    }
  ],
  "overall_result": "passed"
}
```

Set `overall_result`:

- `passed` if all exit codes are 0.
- `failed` if any command exits non-zero.
- `blocked` if commands cannot be run due to missing setup, missing worktree, or invalid contract.

### 4. Validate JSON

If possible, validate with:

```bash
python -m jsonschema -i .ai/tasks/active/<TASK>/05-validation.json .agents/schemas/validation.schema.json
```

## Output

Report:

```text
Validation: passed|failed|blocked
Artifact: .ai/tasks/active/<TASK>/05-validation.json
Failed commands: <none or list>
```

## Rules

- Do not change source code.
- Do not fix failures in this skill.
- Do not run BDD acceptance suites.
