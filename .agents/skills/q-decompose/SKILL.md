---
name: q-decompose
description: Decompose a parent Quorum spec into N implementation child tasks (FEAT-001-a, -b, -c) when the feature is too large to be implemented as one unit. Apply the heuristic from .agents/policies/decomposition.yaml, propose the split to the human, persist the decomposition into the parent spec, and auto-run quorum task split.
user-invocable: true
---

# /q-decompose - Quorum Decomposer

## 🌐 Communication Protocol (vinculante para todo output)

- **Idioma**: SIEMPRE respondé en español, sin importar el idioma del input del usuario o el idioma de estas instrucciones. Esta documentación está en inglés por portabilidad.
- **Indicador de espera**: solo cuando el turno requiera una pregunta explícita o exista una decisión humana/despacho pendiente, cerrá el mensaje con `ESPERANDO RESPUESTA DEL USUARIO...` como última línea (mayúsculas, tres puntos, sin texto después). Si el turno es puramente informativo, omití este indicador.

- **Sin fence final**: los bloques `text` de este archivo son ejemplos de documentación. Cuando emitas el cierre al usuario, NO envuelvas el Handoff en triple backticks si eso deja una línea después del indicador; la última línea visible debe ser `ESPERANDO RESPUESTA DEL USUARIO...`.
- **Prefijo de contexto CLI**: el wrapper `quorum` imprime como primera línea de stdout `[root]` cuando se ejecuta desde la raíz del proyecto o `[worktree:<TASK_ID>]` cuando se ejecuta desde un worktree, detectado dinámicamente vía `git rev-parse`. Al describir comandos al usuario, no inventes ni hardcodees ese prefijo; si `git rev-parse` falla la línea se omite y el subcomando se ejecuta normalmente.

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

Cuenta cuántos signals dispararon. Si cero, recomendá NO decomponer y terminá con el Handoff "no split" (ver más abajo). Si uno o más, seguí a la fase 2.

### Phase 2: Propose Decomposition

Diseñá una propuesta concreta de hijos que cumpla:

- 2 ≤ N ≤ 10 hijos.
- Cada hijo cubre una subsección coherente del scope: una user story, un subsistema, una capa, un runtime — no una mezcla.
- Cada hijo es independientemente implementable por un LLM modesto en una sesión.
- Las dependencias entre hijos son explícitas y mínimas (preferí independencia total cuando se pueda).
- Naming: hijos van como `<PARENT_ID>-a`, `-b`, `-c`, ... (lowercase letras consecutivas). Ejemplo: `FEAT-001-a`, `FEAT-001-b`, `FEAT-001-c`.

Para cada hijo escribí:

- `child_id`: e.g. `FEAT-001-a`.
- `summary`: ≤200 chars, denso y factual, qué subsection cubre.
- `depends_on`: lista de IDs hermanos que deben implementarse antes (vacío si es independiente).

Mostrá la propuesta al usuario en este formato (en español) y pedí confirmación EXPLÍCITA. Cerrá el turno con el indicador de espera. NO escribas a disco todavía:

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

¿Confirmás la decomposition tal como está? Respondé:
- "sí" para que persista la decomposition en el spec del padre y materialice los hijos.
- "ajustar: <descripción>" para iterar.
- "no decomponer" para abortar y volver al flujo single-task.

ESPERANDO RESPUESTA DEL USUARIO...
```

Iterá si el usuario pide ajustes. NO avances sin confirmación explícita.

### Phase 3: Persist + Materialise (post-confirmación)

Cuando el usuario confirme:

1. Editá `.ai/tasks/active/<PARENT_ID>-<slug>/00-spec.yaml` agregando el campo `decomposition` con la lista confirmada. NO toques otros campos del spec (goal, invariants, acceptance, risk siguen igual). Validá contra `spec.schema.json` antes de guardar; si falla, reportá el error y abortá.

2. Auto-ejecutá la transición CLI: `quorum task split <PARENT_ID>` una sola vez por shell. Esto crea cada hijo en `inbox/` con su `00-spec.yaml` derivado (parent_task, depends_on, invariantes y aceptación heredadas). Capturá la salida.

3. Si el CLI falla, reportá `BLOCKED: <stderr>` y NO sigas. NO intentes crear los hijos manualmente.

NO actives `/q-brief`, `/q-blueprint` ni ningún otro skill por los hijos — eso lo hace el orquestador, hijo por hijo.

## 🚫 Rules

- NO inventes invariantes ni criterios de aceptación nuevos en los hijos. Subset estricto del padre.
- NO bajés `risk` por debajo del nivel del padre sin justificación explícita en el output al usuario.
- NO decomponés tareas que ya tienen `parent_task` (sin decomposition recursiva).
- NO toques `01-blueprint.yaml`, `02-contract.yaml` ni nada fuera de `00-spec.yaml` del padre.
- NO movés estado de hijos manualmente. `quorum task split` los pone en `inbox/`; ahí se quedan hasta que el orquestador despache `/q-brief <child>`.

## 🛑 Handoff (single-phase boundary + forward auto-transition)

Esta skill ejecuta SOLO la fase **Decomposition**. Tiene dos resultados posibles:

### Caso A: NO se decompone (signals == 0 o usuario respondió "no decomponer")

No hay transición de estado. El padre queda en `active/` listo para `/q-blueprint`. Cerrá el mensaje con:

```text
=== Fin de fase: Decomposition ===

Resultado: NO decomponer
Razón: <ningún signal disparó | el usuario rechazó la propuesta>

No hay transición de estado: el padre <PARENT_ID> sigue en active/.

Pasos siguientes (los despacha el orquestador, NO yo):
1. [Obligatorio] /q-blueprint <PARENT_ID> — diseña 01-blueprint.yaml y 02-contract.yaml para la tarea como una unidad, y auto-ejecuta quorum task start <PARENT_ID> al terminar.

Si querés volver atrás:
- quorum task back <PARENT_ID> — devuelve la tarea a inbox/ para refinar el spec con /q-brief <PARENT_ID>.

ESPERANDO RESPUESTA DEL USUARIO...
```

### Caso B: SÍ se decompone (usuario confirmó la propuesta)

Después de persistir el campo `decomposition` en el spec del padre y auto-ejecutar `quorum task split <PARENT_ID>`, cerrá con:

```text
=== Fin de fase: Decomposition ===

Resultado: decompuesto en <N> hijos.

Artefactos producidos:
- .ai/tasks/active/<PARENT_ID>-<slug>/00-spec.yaml (campo decomposition agregado)
- .ai/tasks/inbox/<PARENT_ID>-a-new-spec/00-spec.yaml
- .ai/tasks/inbox/<PARENT_ID>-b-new-spec/00-spec.yaml
- ... (uno por hijo)

Transición de estado ejecutada:
- quorum task split <PARENT_ID> ✓ (hijos creados en inbox/ con parent_task y depends_on)

Pasos siguientes (los despacha el orquestador, hijo por hijo, en orden topológico de depends_on):
1. [Obligatorio] /q-brief <PARENT_ID>-a — refinar el spec del primer hijo (auto-ejecutará quorum task blueprint <PARENT_ID>-a al terminar).
2. [Obligatorio] /q-brief <PARENT_ID>-b — segundo hijo (esperar si depends_on == [<PARENT_ID>-a] hasta que ese hijo esté implementado, mergeado y limpiado).
3. ... (uno por hijo)

El padre <PARENT_ID> NO se implementa directamente. Queda en active/ como coordinador. Cuando todos los hijos pasen por done/, el padre se considera completo y podés ejecutar quorum task clean <PARENT_ID> para archivarlo.

Si algo no quedó bien y querés volver atrás:
- quorum task back <hijo_id> — revierte la última transición del hijo (después de que su /q-brief lo haya movido a active/).
- Para deshacer la decomposition entera: editá manualmente el spec del padre quitando `decomposition` y borrá los directorios de hijos en inbox/.

ESPERANDO RESPUESTA DEL USUARIO...
```

NO actives ningún otro skill. La auto-transición a `quorum task split` está autorizada porque elimina fricción sin saltar fases ni decidir routing. Auto-encadenar al `/q-brief` de cada hijo violaría la Regla #9.
