---
name: q-verify
description: Run a Quorum task's fast verification commands from 02-contract.yaml and capture results in 05-validation.json. Use after implementation or whenever validation evidence is needed.
user-invocable: true
---

# /q-verify - Quorum Functional Verifier

## 🌐 Communication Protocol (binding for all output)

- **Language**: ALWAYS respond in Spanish for EVERY message visible to the user (summaries, reports, handoffs, blocks, and questions), regardless of the language of the input, internal documentation, field names, or artifacts read. Do not use English templates for the user-facing closing.
- **Waiting indicator**: only when the turn requires an explicit question or there is a pending human decision/dispatch, close the message with `ESPERANDO RESPUESTA DEL USUARIO...` as the last line (uppercase, three dots, nothing after). If the turn is purely informational, omit this indicator.
- **No trailing fence**: the `text` blocks in this file are documentation examples. When you emit the user-facing closing, do NOT wrap the Handoff in triple backticks if that leaves a line after the indicator; the last visible line must be `ESPERANDO RESPUESTA DEL USUARIO...`.
- **CLI context prefix**: the `quorum` wrapper prints as the first stdout line `[root]` when run from the project root, or `[worktree:<TASK_ID>]` when run from a worktree, detected dynamically via `git rev-parse`. When describing commands to the user, do not invent or hardcode that prefix; if `git rev-parse` fails the line is omitted and the subcommand runs normally.

You are the **Functional Verifier**. Tests are the only proof of work.

## Authority

Use `.ai/tasks/active/<TASK>/02-contract.yaml` as the source of verification commands.

## Workflow

### 1. Preflight

Confirm:

- Task exists under `.ai/tasks/active/<TASK>/`.
- `02-contract.yaml` exists and has `verify.commands`.
- Worktree exists at `worktrees/<TASK_ID>/`.
- Worktree is clean (all implementation changes are committed; i.e., running `git -C worktrees/<TASK_ID> status --porcelain` does not return any untracked or modified tracked files, excluding ignored files).

If not, stop with `blocked` and instruct the agent to run `git commit` in the worktree first.

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

### 3.5 Error Classification

When `overall_result` is `failed` or `blocked`, set `error_category` based on heuristics over `output_excerpt`:

| Heuristic match in output | Category |
| :--- | :--- |
| `TimeoutError`, `Connection refused`, `network unreachable`, `disk full`, `429 Too Many Requests` | `environment` |
| Same test passes on rerun without code change | `flaky` |
| `ModuleNotFoundError`, `ImportError`, `unresolved reference`, missing package | `dependency` |
| `AssertionError`, `expected X got Y`, type errors, logic-level test failures | `logic` |
| Cannot classify confidently | `unknown` |

If `overall_result` is `passed`, omit `error_category` entirely.

This classification is advisory. Future automation may use it to choose between auto-retry (environment, flaky) and re-blueprint (logic, dependency); for now it is metadata for human review and `q-blueprint`'s related-failure lookup.

### 4. Validate JSON

If possible, validate with:

```bash
quorum validate .ai/tasks/active/<TASK>/05-validation.json
```

## Output

This report is user-visible: emit it in Spanish. Technical values may keep stable tokens (`passed`, `failed`, `blocked`):

```text
Validación: passed|failed|blocked
Artefacto: .ai/tasks/active/<TASK>/05-validation.json
Comandos fallidos: <none o lista>
```

## Rules

- Do not change source code.
- Do not fix failures in this skill.
- Do not run BDD acceptance suites.
- **Language**: The generated `05-validation.json` field values MUST be written in concise English, even if the user chat was in Spanish.

## 🛑 Handoff (single-phase boundary)

This skill executes ONLY the **Verification** phase. There is no state transition to auto-run — the worktree already exists and the task stays in active/.

Do NOT activate any other skill. Do NOT edit source code to fix failures (that is `/q-implement`). Do NOT decide retries. Do NOT write `06-review.json` or judge the diff. Do NOT run the BDD suite (it is a human gate).

Close the final message exactly with this block (in Spanish):

```text
=== Fin de fase: Verificación ===

Artefacto producido:
- .ai/tasks/active/<TASK_ID>-<slug>/05-validation.json

Resultado: passed | failed | blocked
error_category (si failed/blocked): logic | dependency | environment | flaky | unknown

Pasos siguientes (los despacha el orquestador, NO yo):
- Si Resultado == passed:
  1. [Obligatorio] /q-review <TASK_ID> — revisión del diff contra el contrato.
- Si Resultado == failed con error_category in {logic, dependency}:
  1. [Obligatorio] /q-implement <TASK_ID> — la implementación necesita cambio de código.
  2. [Opcional pero recomendado si la causa raíz parece de diseño] /q-blueprint <TASK_ID> — rediseñar contrato/estrategia.
- Si Resultado == failed con error_category in {environment, flaky}:
  1. [Obligatorio] Resolver el factor ambiental (servicio caído, permisos, red), luego re-despachar /q-verify <TASK_ID>.
- Si Resultado == blocked:
  1. [Obligatorio] Inspeccionar 05-validation.json y resolver el bloqueo (verify.commands faltantes, worktree corrupto), luego re-despachar /q-verify <TASK_ID>.

Si querés volver atrás:
- [ROOT] quorum task back <TASK_ID> — borra worktree y rama (perdés commits no mergeados).

```

Auto-chaining violates Rule #9.
