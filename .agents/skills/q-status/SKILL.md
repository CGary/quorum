---
name: q-status
description: Inspect Quorum task state, artifact readiness, and next recommended action across .ai/tasks. Use when checking task progress, listing Quorum tasks, diagnosing missing artifacts, or deciding the next workflow step.
user-invocable: true
---

# /q-status - Quorum Mission Control

## 🌐 Communication Protocol (binding for all output)

- **Language**: ALWAYS respond in Spanish for EVERY message visible to the user (summaries, reports, handoffs, blocks, and questions), regardless of the language of the input, internal documentation, field names, or artifacts read. Do not use English templates for the user-facing closing.
- **Waiting indicator**: only when the turn requires an explicit question or there is a pending human decision/dispatch, close the message with `ESPERANDO RESPUESTA DEL USUARIO...` as the last line (uppercase, three dots, nothing after). If the turn is purely informational, omit this indicator.
- **No trailing fence**: the `text` blocks in this file are documentation examples. When you emit the user-facing closing, do NOT wrap the Handoff in triple backticks if that leaves a line after the indicator; the last visible line must be `ESPERANDO RESPUESTA DEL USUARIO...`.
- **CLI context prefix**: the `quorum` wrapper prints as the first stdout line `[root]` when run from the project root, or `[worktree:<TASK_ID>]` when run from a worktree, detected dynamically via `git rev-parse`. When describing commands to the user, do not invent or hardcode that prefix; if `git rev-parse` fails the line is omitted and the subcommand runs normally.

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
quorum task list
```

If a task ID is provided, also run:

```bash
quorum task status <TASK_ID>
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

- Missing `00-spec.yaml` → run `quorum task specify <ID>` then `/q-brief <ID>`.
- In `inbox/` with `00-spec.yaml` only → use `/q-brief <ID>`; if it succeeds it auto-runs `quorum task blueprint <ID>`.
- In `active/` with `decomposition` → report child locations; next is the first child whose `depends_on` siblings are `done/` (usually `/q-brief <child>` if still in `inbox/`), or `quorum task clean <PARENT_ID>` if all children are `done/`.
- In `active/` with `00-spec.yaml` but no `01-blueprint.yaml`/`02-contract.yaml` → if it is a parent or standalone task (no `parent_task`), use `/q-decompose <ID>` if the scope may be large, otherwise `/q-blueprint <ID>`. If it is a child task (has `parent_task`), ALWAYS use `/q-blueprint <ID>` and omit `/q-decompose`.
- Has `01-blueprint.yaml` and `02-contract.yaml` but no worktree → normally `/q-blueprint` should have auto-run `quorum task start`; recommend re-dispatching `/q-blueprint <ID>` or manually running `quorum task start <ID>` only as repair.
- Active with contract and worktree but no implementation log → use `/q-implement`.
- Has implementation but no validation → use `/q-verify`.
- Has passing validation but no review → use `/q-review`.
- Has approved review → use `/q-accept` for human merge readiness.
- Done → optionally use `/q-memory` to capture lessons.

## Output

This report is user-visible: emit it in Spanish. The technical state names (`inbox`, `active`, `done`, `failed`) may remain as tokens:

```text
Tarea: <TASK_ID o all>
Ubicación: <inbox|active|done|failed|mixed>
Artefactos:
- 00-spec.yaml: present|missing
...
Estado: <una oración>
Siguiente: <comando o skill exacto>
```

## Rules

- Do not infer success from prose; read artifacts.
- Do not run verification commands here.
- Do not create, move, or delete tasks.

## 🛑 Handoff (single-phase boundary)

This skill executes ONLY the **Read-only State Report** phase. It is strictly read-only and has no state transition to auto-run.

Do NOT activate any other skill. You may run only read-only commands (`quorum task list` and `quorum task status <TASK_ID>`). Do NOT run state-mutating commands (`specify`, `blueprint`, `start`, `clean`, `back`, `split`) — reporting is the whole job. Do NOT modify any artifact, including `07-trace.json`.

Close the final message exactly with this block (in Spanish):

```text
=== Fin de fase: Estado ===

Reporte: emitido en este turno (read-only, no se persiste).

Pasos siguientes (los despacha el orquestador, NO yo):
1. [Obligatorio] <comando o skill exacto recomendado en el reporte arriba — copiá la línea "Next" del reporte>
2. [Opcional] Si el reporte muestra múltiples tareas, podés despachar /q-status <TASK_ID> para zoom-in en una específica.

Si querés volver atrás en alguna tarea:
- [ROOT] quorum task back <TASK_ID> — revierte la última transición de esa tarea.

```

Auto-chaining into the recommended action violates Rule #9.
