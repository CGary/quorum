---
name: fleet-cli-usage
description: Use when an agent must run an external model transport (agy) standalone through `quorum fleet run` — a NON-LIFECYCLE runner in an explicit --cwd. Do not use for the task-bound SDC dispatch (`quorum fleet dispatch`) or for any q-* lifecycle phase.
---

# fleet-cli-usage

`quorum fleet run` runs an agent transport (default `agy`) in an explicit `--cwd`
via the policy-free `core.RunDelegate` primitive. It is agent-friendly (mk-cli
contract) and **non-lifecycle**.

## When to use

- You need to invoke a model transport directly for a gateway/testing use, with an
  explicit working directory and prompt, and you do NOT want any SDC state.

## When NOT to use

- Task-bound, forensic dispatch against a worktree → use `quorum fleet dispatch`
  (writes result.json, forensic ref, 07-trace).
- Any lifecycle phase (brief/blueprint/implement/verify/review) → use the `q-*` skills.
- `fleet run` creates NO task, worktree, git operation, forensic ref, 07-trace, or
  result.json. It never merges or commits.

## Required flags (agents)

- `--model <canonical>` — closed enum from the transport models map (see `--schema`).
- `--cwd <dir>` — existing directory the delegate runs in.
- `--input <file>` or `--input -` — the prompt (file or stdin). No inline prompt flag exists.
- Always add `--json` and `--no-input`.

## Common commands

```bash
quorum fleet run --schema
quorum fleet run --agent agy --model anthropic/claude-sonnet-4-6 --cwd . --input prompt.txt --no-input --json
cat prompt.txt | quorum fleet run --model anthropic/claude-opus-4-6 --cwd /repo --input - --no-input --json
quorum fleet run --model anthropic/claude-sonnet-4-6 --cwd . --input prompt.txt --dry-run --json
quorum fleet run --model anthropic/claude-sonnet-4-6 --cwd . --input prompt.txt --output out.txt --json
```

## JSON contract

- Success (stdout, one object): `{ok:true, command:"fleet.run", summary, data, next_actions}`.
  `data` carries `exit_code`, `output` (or `result_file` with `--output`).
- Error (stdout, one object): `{ok:false, command, error:{code, message, field, received}, retryable, suggested_fix}`.
- Under `--json` stdout is exactly one JSON object; all logs go to stderr.

## Error handling

Stable `error.code` values: `MISSING_REQUIRED_FLAG`, `INVALID_ENUM`, `FILE_NOT_FOUND`,
`TIMEOUT`, `INVALID_ARGUMENT`, `INTERNAL_ERROR`. On `INVALID_ENUM` the message lists
valid `--model` names; retry with one of them. A delegate exceeding `--timeout`
returns `TIMEOUT` (retryable).
