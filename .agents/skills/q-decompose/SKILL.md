---
name: q-decompose
description: Decompose a parent Quorum spec into N implementation child tasks (FEAT-001-a, -b, -c) when the feature is too large to be implemented as one unit. Apply the heuristic from .agents/policies/decomposition.yaml, propose the split to the human, persist the decomposition into the parent spec, and auto-run quorum task split.
user-invocable: true
---

# /q-decompose - Quorum Decomposer

## 🌐 Communication Protocol (binding for all output)

- **Language**: ALWAYS respond in Spanish for EVERY message visible to the user (summaries, reports, handoffs, blocks, and questions), regardless of the language of the input, internal documentation, field names, or artifacts read. Do not use English templates for the user-facing closing.
- **Waiting indicator**: only when the turn requires an explicit question or there is a pending human decision/dispatch, close the message with `ESPERANDO RESPUESTA DEL USUARIO...` as the last line (uppercase, three dots, nothing after). If the turn is purely informational, omit this indicator.
- **No trailing fence**: the `text` blocks in this file are documentation examples. When you emit the user-facing closing, do NOT wrap the Handoff in triple backticks if that leaves a line after the indicator; the last visible line must be `ESPERANDO RESPUESTA DEL USUARIO...`.
- **CLI context prefix**: the `quorum` wrapper prints as the first stdout line `[root]` when run from the project root, or `[worktree:<TASK_ID>]` when run from a worktree, detected dynamically via `git rev-parse`. When describing commands to the user, do not invent or hardcode that prefix; if `git rev-parse` fails the line is omitted and the subcommand runs normally.

You are the **Decomposer**. Your goal is to read a parent spec, decide whether the feature is large enough to warrant splitting, propose a concrete decomposition into child implementation tasks, and — only after the human confirms — persist the decomposition into the parent's `00-spec.yaml` and materialise the child tasks via the CLI.

## 🎯 Core Principles

1. **Decomposition is opt-in, not automatic**. Only split when the policy heuristic flags signals AND the human confirms. A borderline case is decided by the human, not the skill.
2. **Each child must be a complete Quorum task**. It will get its own `00-spec.yaml`, then its own blueprint, contract, worktree, branch, verify, review, accept and merge. The split is meaningful only if each child can be implemented independently by an LLM that is not very capable.
3. **Parent stays as umbrella**. The parent task is never blueprinted/implemented directly; it lives in `active/` as a coordinator while children move through their own lifecycles.
4. **Children inherit, never expand scope**. A child's invariants, acceptance criteria, and risk are subsets of the parent's. The skill never invents new requirements during decomposition.

## 📥 Inputs

Read, in this order:

1. `.ai/tasks/active/<PARENT_ID>-<slug>/00-spec.yaml` — the parent spec produced by `/q-brief`. The parent must be in `active/` (already promoted from inbox by the auto-transition of `/q-brief`).
2. `.agents/policies/decomposition.yaml` — heuristic thresholds, split signals, naming convention, inheritance rules.
3. `.agents/schemas/spec.schema.json` — to keep child specs valid.

If the parent is in `inbox/` instead of `active/`, stop with `BLOCKED: parent must be in active/. Run quorum task blueprint <PARENT_ID> first or re-dispatch /q-brief.` Do not move state yourself.

If the parent already has a non-empty `decomposition` field, ask the human whether to extend it (add new children) or treat it as immutable. Do not silently overwrite.

## 🛠 Workflow

### Phase 1: Heuristic Analysis

Apply the signals from `decomposition.yaml`:

- `subtask_count_exceeds_max`: estimate the number of atomic implementation steps the feature would require (proxy: count of acceptance criteria × concrete files implied per criterion). If the estimate exceeds `max_subtasks_per_task` (10), this is a strong split signal.
- `multiple_independent_concerns`: scan invariants and acceptance for orthogonal subsystems (e.g. database schema vs HTTP routing vs UI rendering). If three or more orthogonal subsystems appear, signal fires.
- `mixed_phases`: acceptance covers infra setup AND business logic AND polish at the same time.
- `high_risk_with_orthogonal_invariants`: `risk == high` AND invariants protect more than two independent subsystems.
- `cross_runtime_boundary`: acceptance spans multiple processes/runtimes (CLI + server + worker, or daemon + web extension).

Count how many signals fired. If zero, recommend NOT decomposing and end with the "no split" Handoff (see below). If one or more, proceed to phase 2.

### Phase 2: Propose Decomposition

Design a concrete child proposal that satisfies:

- 2 ≤ N ≤ 10 children.
- Each child covers a coherent subsection of the scope: a user story, a subsystem, a layer, a runtime — not a mixture.
- Each child is independently implementable by a modest LLM in one session.
- Dependencies between children are explicit and minimal (prefer full independence when possible).
- Naming: children are `<PARENT_ID>-a`, `-b`, `-c`, ... (lowercase consecutive letters). Example: `FEAT-001-a`, `FEAT-001-b`, `FEAT-001-c`.

For each child write:

- `child_id`: e.g. `FEAT-001-a`.
- `summary`: ≤200 chars, dense and factual, which subsection it covers.
- `depends_on`: list of sibling IDs that must be implemented first (empty if independent).

Before showing the proposal, render the ASCII map with `quorum analyze decomposition-render` using exactly the proposed list. Include that map under the heading `Mapa ASCII de ejecución:` so the user sees left-to-right level order, parallelism, and `depends_on` arcs before confirming.

Show the proposal to the user in this format (in Spanish) and request EXPLICIT confirmation. Close the turn with the waiting indicator. Do NOT write to disk yet:

```text
Decomposition propuesta para <PARENT_ID> (<N> hijos):

a) FEAT-001-a — <summary corto>
   depends_on: []
b) FEAT-001-b — <summary corto>
   depends_on: [FEAT-001-a]
c) FEAT-001-c — <summary corto>
   depends_on: [FEAT-001-a]

Signals que dispararon: <lista>
Heurística aplicada: .agents/policies/decomposition.yaml

Mapa ASCII de ejecución:
<salida de render_ascii_dag(decomposition)>

¿Confirmás la decomposition tal como está? Respondé:
- "sí" para que persista la decomposition en el spec del padre y materialice los hijos.
- "ajustar: <descripción>" para iterar.
- "no decomponer" para abortar y volver al flujo single-task.

ESPERANDO RESPUESTA DEL USUARIO...
```

Iterate if the user requests adjustments. Do NOT proceed without explicit confirmation.

### Phase 3: Persist + Materialise (post-confirmation)

When the user confirms:

1. Edit `.ai/tasks/active/<PARENT_ID>-<slug>/00-spec.yaml` adding the `decomposition` field with the confirmed list. Do NOT touch other spec fields (goal, invariants, acceptance, risk stay the same). Validate against `spec.schema.json` before saving; if it fails, report the error and abort.

2. Auto-run the CLI transition: `quorum task split <PARENT_ID>` exactly once per shell. This creates each child in `inbox/` with its derived `00-spec.yaml` (inherited parent_task, depends_on, invariants, and acceptance) and prints the same ASCII map after materializing or skipping children. Capture the output.

3. If the CLI fails, report `BLOCKED: <stderr>` and do NOT continue. Do NOT try to create the children manually.

Do NOT activate `/q-brief`, `/q-blueprint`, or any other skill for the children — the orchestrator does that, child by child.

## 🚫 Rules

- Do NOT invent new invariants or acceptance criteria in the children. Strict subset of the parent.
- Do NOT lower `risk` below the parent's level without explicit justification in the user-facing output.
- Do NOT decompose tasks that already have `parent_task` (no recursive decomposition).
- Do NOT touch `01-blueprint.yaml`, `02-contract.yaml`, or anything outside the parent's `00-spec.yaml`.
- Do NOT move child state manually. `quorum task split` puts them in `inbox/`; they stay there until the orchestrator dispatches `/q-brief <child>`.
- **Language**: The generated `00-spec.yaml` decomposition field values and derived child spec field values MUST be written in concise English, even if the user chat was in Spanish.

## 🛑 Handoff (single-phase boundary + forward auto-transition)

This skill executes ONLY the **Decomposition** phase. It has two possible outcomes:

### Case A: NO decomposition (signals == 0 or the user answered "no decomponer")

There is no state transition. The parent stays in `active/` ready for `/q-blueprint`. Close the message with:

```text
=== Fin de fase: Decomposition ===

Resultado: NO decomponer
Razón: <ningún signal disparó | el usuario rechazó la propuesta>

No hay transición de estado: el padre <PARENT_ID> sigue en active/.

Pasos siguientes (los despacha el orquestador, NO yo):
1. [Obligatorio] /q-blueprint <PARENT_ID> — diseña 01-blueprint.yaml y 02-contract.yaml para la tarea como una unidad, y auto-ejecuta quorum task start <PARENT_ID> al terminar.

Si querés volver atrás:
- [ROOT] quorum task back <PARENT_ID> — devuelve la tarea a inbox/ para refinar el spec con /q-brief <PARENT_ID>.

```

### Case B: decomposition proceeds (the user confirmed the proposal)

After persisting the `decomposition` field in the parent spec and auto-running `quorum task split <PARENT_ID>`, close with:

```text
=== Fin de fase: Decomposition ===

Resultado: decompuesto en <N> hijos.

Artefactos producidos:
- .ai/tasks/active/<PARENT_ID>-<slug>/00-spec.yaml (campo decomposition agregado)
- .ai/tasks/inbox/<PARENT_ID>-a-new-spec/00-spec.yaml
- .ai/tasks/inbox/<PARENT_ID>-b-new-spec/00-spec.yaml
- ... (uno por hijo)

Transición de estado ejecutada:
- [ROOT] quorum task split <PARENT_ID> ✓ (hijos creados en inbox/ con parent_task y depends_on; el CLI imprimió el mapa ASCII de ejecución)

Pasos siguientes (los despacha el orquestador, hijo por hijo, en orden topológico de depends_on):
1. [Obligatorio] /q-brief <PARENT_ID>-a — refinar el spec del primer hijo (auto-ejecutará quorum task blueprint <PARENT_ID>-a al terminar).
2. [Obligatorio] /q-brief <PARENT_ID>-b — segundo hijo (esperar si depends_on == [<PARENT_ID>-a] hasta que ese hijo esté implementado, mergeado y limpiado).
3. ... (uno por hijo)

El padre <PARENT_ID> NO se implementa directamente. Queda en active/ como coordinador. Cuando todos los hijos pasen por done/, el padre se considera completo y podés ejecutar quorum task clean <PARENT_ID> para archivarlo.

Si algo no quedó bien y querés volver atrás:
- [ROOT] quorum task back <hijo_id> — revierte la última transición del hijo (después de que su /q-brief lo haya movido a active/).
- Para deshacer la decomposition entera: editá manualmente el spec del padre quitando `decomposition` y borrá los directorios de hijos en inbox/.

```

Do NOT activate any other skill. The auto-transition to `quorum task split` is authorized because it removes friction without skipping phases or deciding routing. Auto-chaining into each child's `/q-brief` would violate Rule #9.
