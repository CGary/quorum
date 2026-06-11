---
name: q-implement
description: Implement a Quorum task inside its isolated worktree using 00-spec.yaml, 01-blueprint.yaml, and strict 02-contract.yaml boundaries. Use for surgical code changes after quorum task start has prepared the worktree.
user-invocable: true
---

# /q-implement - Quorum Surgical Executor

## 🌐 Communication Protocol (binding for all output)

- **Language**: ALWAYS respond in Spanish for EVERY message visible to the user (summaries, reports, handoffs, blocks, and questions), regardless of the language of the input, internal documentation, field names, or artifacts read. Do not use English templates for the user-facing closing.
- **Waiting indicator**: only when the turn requires an explicit question or there is a pending human decision/dispatch, close the message with `ESPERANDO RESPUESTA DEL USUARIO...` as the last line (uppercase, three dots, nothing after). If the turn is purely informational, omit this indicator.
- **No trailing fence**: the `text` blocks in this file are documentation examples. When you emit the user-facing closing, do NOT wrap the Handoff in triple backticks if that leaves a line after the indicator; the last visible line must be `ESPERANDO RESPUESTA DEL USUARIO...`.
- **CLI context prefix**: the `quorum` wrapper prints as the first stdout line `[root]` when run from the project root, or `[worktree:<TASK_ID>]` when run from a worktree, detected dynamically via `git rev-parse`. When describing commands to the user, do not invent or hardcode that prefix; if `git rev-parse` fails the line is omitted and the subcommand runs normally.

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

Normal implementation starts from an `active/` task. If the task is not in
`active/` but is a **child task** in `failed/`, and this `/q-implement` dispatch
was explicitly requested by the human/orchestrator as a retry, first prepare the
retry from the repo root:

```bash
quorum task retry-prepare <TASK_ID>
```

This retry preflight is authorized only for failed child tasks. It preserves
`07-trace.json` attempts, removes stale `05-validation.json`/`06-review.json`,
restores the child to `active/`, and never invokes `quorum task back`.

If the worktree still does not exist after that preflight, stop and tell the user to run:

```bash
quorum task start <TASK_ID>
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

If unrelated dirty changes exist, stop with `BLOCKED`. For retry dispatches, a
dirty worktree must also block; do not stash, reset, discard, or call
`quorum task back` yourself.

### 3. Capture TDD Red Evidence Before Implementation

When the task uses structured acceptance criteria with `acceptance.id` and the blueprint has `test_scenarios[].covers`, capture non-vacuity evidence before implementation when a covering test command can be run:

- Identify covered acceptance ids from `01-blueprint.yaml` `test_scenarios[].covers`; ignore legacy plain-string acceptance criteria without ids.
- For each covered `acceptance_id` that has a concrete fast test command available, run that command before implementation changes make it pass.
- Record each observation in `04-implementation-log.yaml` top-level optional `tdd_red_runs[]` as `{acceptance_id, command, red_exit_code}`.
- If `red_exit_code` is `0`, record the run anyway and surface it as an immediate vacuity finding: the covering test passed before implementation and is invalid TDD evidence.
- If no safe covering command exists for an id, do not invent evidence; leave it missing so `/q-accept` can report missing evidence as advisory human context.
- This RED capture belongs only in `04-implementation-log.yaml`; never write `05-validation.json` from `/q-implement`.

### 4. Implement Surgically

- Modify only files allowed by `touch`.
- Respect every invariant from `00-spec.yaml`.
- Follow strategy from `01-blueprint.yaml`.
- Add or update tests when acceptance or blueprint requires behavior coverage.
- Avoid broad refactors, formatting-only rewrites, dependency changes, generated churn, or opportunistic cleanup.

### 5. Boundary Check

Before finishing, compare changed files to `touch` and `forbid.files`:

```bash
git -C worktrees/<TASK_ID> diff --name-only
```

If any changed file is outside `touch` or matches `forbid.files`, revert only the violating changes or stop as `BLOCKED`.

### 6. Implementation Log

Create or append `.ai/tasks/active/<TASK>/04-implementation-log.yaml`. If RED runs were captured, include top-level `tdd_red_runs[]`; field values must be concise English:

```yaml
task_id: FEAT-001
summary: Implemented contract-scoped change in allowed files.
tdd_red_runs:
  - acceptance_id: AC-1
    command: go test ./internal/core -run TestSpecificAcceptance
    red_exit_code: 1
entries:
  - changed_files:
      - path/to/file.py
    notes:
      - Added behavior required by acceptance criteria.
    verify_pending: true
```
Keep YAML shallow. `summary` must be second key.

### 7. Git Commit

[Mandatory] Before declaring DONE, you must consolidate your work by formally recording your changes on the Git branch for the task. Run the following commands from the worktree directory (`worktrees/<TASK_ID>/`):

```bash
git add -A
git commit -m "feat(core): <concise technical summary of the changes in English>"
```

## Output

The short technical signal may use stable tokens (`DONE`/`BLOCKED`), but the user-visible message and the final handoff must be in Spanish. If you emit a short signal before the handoff, use only one of these forms:

```text
DONE: <concise technical summary in English or technical Spanish>
```

or

```text
BLOCKED: <specific reason; prefer writing the user-facing explanation in Spanish>
```

When the block is caused by a contract boundary problem, such as a required
file missing from `touch`, the specific reason MUST use this parseable form:

```text
BLOCKED: missing_file=<path>; reason=<text>; severity=<critical|minor>
```

Use `severity=critical` when implementation cannot proceed under the current
contract. Use `severity=minor` when work can continue but the contract omission
should be recorded for later renegotiation analysis. This text shape maps to the
future renegotiation-request fields `path`, `reason`, and `severity`.

## Rules

- Do not run slow BDD suites.
- Do not merge.
- Do not edit task schemas or policies unless explicitly in `touch`.
- Do not expand the contract yourself. If the contract is wrong, block.
- **Language**: The generated `04-implementation-log.yaml` field values MUST be written in concise English, even if the user chat was in Spanish.

## 🛑 Handoff (single-phase boundary)

This skill executes ONLY the **Implementation** phase. The task is already in active/ with a worktree created by `/q-blueprint`; there is no state transition to auto-run.

Do NOT activate any other skill. Do NOT run `verify.commands` (that is `/q-verify`'s phase). Do NOT write `05-validation.json`, `06-review.json`, or review entries in `07-trace.json`. Do NOT decide retries on your own. Do NOT merge or open a PR.

Close the final message exactly with one of these blocks (in Spanish):

**Success case**:
```text
=== Fin de fase: Implementación ===

Resultado: DONE
Resumen técnico: <una línea>

Artefactos producidos:
- Diff committeado en la rama ai/<TASK_ID> (worktrees/<TASK_ID>/)
- .ai/tasks/active/<TASK_ID>-<slug>/04-implementation-log.yaml

No hay transición de estado: el worktree y la rama siguen iguales.

Pasos siguientes (los despacha el orquestador, NO yo):
1. [Obligatorio] /q-verify <TASK_ID> — corre verify.commands del contrato dentro del worktree y escribe 05-validation.json.

Si querés volver atrás:
- [WORKTREE:<TASK_ID>] git -C worktrees/<TASK_ID> reset --hard HEAD~1 — descarta el último commit del worktree.
- [ROOT] quorum task back <TASK_ID> — borra worktree y rama (perdés todos los commits no mergeados).

```

**Blocked case**:
```text
=== Fin de fase: Implementación ===

Resultado: BLOCKED
Razón específica: <descripción; si es bloqueo de contrato, usar `BLOCKED: missing_file=<path>; reason=<text>; severity=<critical|minor>`>

Pasos siguientes (los despacha el orquestador, NO yo):
- Si el contrato es incorrecto (touch insuficiente, forbid mal puesto, verify.commands inadecuados):
  1. [Obligatorio] /q-blueprint <TASK_ID> — rediseñar 01/02.
- Si la spec es ambigua o cambió la intención:
  1. [Obligatorio] [ROOT] quorum task back <TASK_ID> (dos veces si hace falta) hasta volver a inbox/ y luego /q-brief <TASK_ID>.
- Si el bloqueo es ambiental (dependencia faltante, permisos):
  1. [Obligatorio] Resolución manual fuera del agent loop, luego re-despachar /q-implement <TASK_ID>.

ESPERANDO RESPUESTA DEL USUARIO...
```

Auto-chaining into `/q-verify` violates Rule #9.
