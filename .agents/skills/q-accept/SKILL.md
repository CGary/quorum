---
name: q-accept
description: Validate Quorum task readiness for human merge by checking validation, review, trace, contract, and optional BDD gate instructions. Use after q-review approves a task.
user-invocable: true
---

# /q-accept - Quorum Human Merge Gate

## 🌐 Communication Protocol (binding for all output)

- **Language**: ALWAYS respond in Spanish for EVERY message visible to the user (summaries, reports, handoffs, blocks, and questions), regardless of the language of the input, internal documentation, field names, or artifacts read. Do not use English templates for the user-facing closing.
- **Waiting indicator**: only when the turn requires an explicit question or there is a pending human decision/dispatch, close the message with `ESPERANDO RESPUESTA DEL USUARIO...` as the last line (uppercase, three dots, nothing after). If the turn is purely informational, omit this indicator.
- **No trailing fence**: the `text` blocks in this file are documentation examples. When you emit the user-facing closing, do NOT wrap the Handoff in triple backticks if that leaves a line after the indicator; the last visible line must be `ESPERANDO RESPUESTA DEL USUARIO...`.
- **CLI context prefix**: the `quorum` wrapper prints as the first stdout line `[root]` when run from the project root, or `[worktree:<TASK_ID>]` when run from a worktree, detected dynamically via `git rev-parse`. When describing commands to the user, do not invent or hardcode that prefix; if `git rev-parse` fails the line is omitted and the subcommand runs normally.

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

This mini-report is user-visible: emit it in Spanish and do not copy English labels. Use this format:

```text
Aceptación: ready|not_ready
Tarea: <TASK_ID>
Acción humana requerida:
- Correr compuerta BDD: <comando o none>
- Inspeccionar diff en worktrees/<TASK_ID>
- Mergear manualmente si está conforme
Bloqueantes:
- <none o lista>
```

## Rules

- Do not run merge commands.
- Do not move task to `done`; use `quorum task clean <TASK_ID>` only if the human explicitly asks after merge.
- Do not run slow BDD automatically unless explicitly instructed by the human.
- Do not override failed validation or rejected review.

## 🛑 Handoff (single-phase boundary)

This skill executes ONLY the **Merge Gate** phase. There is no state transition to auto-run — merge and clean are explicit human actions (Rule #6).

Do NOT activate any other skill. Do NOT run `git merge`, `git push`, `gh pr merge`, or any merge command. Do NOT run `quorum task clean` on your own. Do NOT run the BDD suite: report it as a mandatory human gate if the contract defines it.

Close the final message exactly with one of these blocks (in Spanish):

**Ready case**:
```text
=== Fin de fase: Compuerta de aceptación ===

Veredicto: ready

Pasos siguientes que tiene que ejecutar el HUMANO (no el orquestador automático):
1. [Obligatorio si 02-contract.yaml.acceptance.bdd_suite está definido] Correr la suite BDD:
   <comando bdd>
2. [Obligatorio] Inspeccionar el diff: git -C worktrees/<TASK_ID> diff main..ai/<TASK_ID>
3. [Obligatorio] Mergear manualmente:
   git checkout main && git merge ai/<TASK_ID>

Pasos posteriores (los despacha el orquestador después del merge):
4. [Obligatorio] [ROOT] quorum task clean <TASK_ID> — archiva la tarea en done/ y borra el worktree.
5. [Opcional pero recomendado] /q-memory <TASK_ID> — captura lecciones, decisiones o patrones durables.

Si querés volver atrás antes de mergear:
- [ROOT] quorum task back <TASK_ID> — borra worktree y rama (perdés commits no mergeados).

```

**Not_ready case**:
```text
=== Fin de fase: Compuerta de aceptación ===

Veredicto: not_ready
Bloqueantes:
- <lista concreta>

Pasos siguientes (los despacha el orquestador, NO yo):
- Si el bloqueo es de validación (05-validation.json failed):
  1. [Obligatorio] /q-implement <TASK_ID> → /q-verify <TASK_ID> hasta resolver.
- Si el bloqueo es de revisión (06-review.json revise/reject):
  1. [Obligatorio] /q-implement <TASK_ID> con los fix_tasks → /q-verify <TASK_ID> → /q-review <TASK_ID>.
- Si hay archivos prohibidos tocados o refactor no pedido:
  1. [Obligatorio] /q-implement <TASK_ID> revirtiendo los cambios fuera de scope.

```

Auto-merging or auto-chaining violates Rules #9, #6, and #7.
