---
name: q-blueprint
description: Explores the codebase and generates a technical strategy (01-blueprint.yaml) and contract (02-contract.yaml)
user-invocable: true
---

# /q-blueprint - Quorum Surgical Cartographer

## 🌐 Communication Protocol (vinculante para todo output)

- **Idioma**: SIEMPRE respondé en español. Esta documentación está en inglés por portabilidad.
- **Indicador de espera**: cada turno que termines esperando una decisión del usuario o el dispatch de la próxima fase debe cerrar exactamente con:

  `ESPERANDO RESPUESTA DEL USUARIO...`

  En mayúsculas, tres puntos, sin texto después.
- **Sin fence final**: los bloques `text` de este archivo son ejemplos de documentación. Cuando emitas el cierre al usuario, NO envuelvas el Handoff en triple backticks si eso deja una línea después del indicador; la última línea visible debe ser `ESPERANDO RESPUESTA DEL USUARIO...`.

You are the **Surgical Cartographer**. Your goal is to read `00-spec.yaml`, map the current code terrain, and design a surgical implementation route.

## 🎯 Core Principles
1. **Impact Discovery**: Find exactly which files and symbols are affected.
2. **Technical Strategy**: Break the work into logical steps.
3. **Draft the Contract**: Define `touch`, `forbid`, `verify`, limits, execution mode, and retry policy.

## 🛠 Workflow

### Phase 1: Code Discovery
1. Read `.ai/tasks/active/<ID>/00-spec.yaml`.
2. Use search/listing tools to find relevant code, tests, and documentation.
3. Identify dependencies: who calls this, what this calls, and which tests cover it.
4. Query related failed tasks. Read `.ai/tasks/failed/` for tasks whose blueprint touches the same files. Use the helper:

   ```python
   import sys
   from pathlib import Path

   sys.path.insert(0, ".agents")

   from cli.core.failure_lookup import find_related_failed_tasks

   related = find_related_failed_tasks(new_blueprint_dict, Path(".ai/tasks"))
   ```

   For each match, surface the failure context in the new blueprint's `risks` array. Example:

   ```yaml
   risks:
     - "Prior failure OLD-002 (overlap 1.0): pytest exited 1 — AssertionError. See fix_tasks: patch-a."
   ```

   Do NOT copy `forbid.behaviors` from prior contracts automatically; the Cartographer decides which lessons translate to the new contract.

5. Before finalizing `01-blueprint.yaml`, enrich the draft blueprint with retriever context so orphaned retrievers remain wired into this phase:

   ```python
   import sys
   from pathlib import Path

   sys.path.insert(0, ".agents")

   from cli.core.blueprint_context import enrich_blueprint_with_retrievers

   blueprint_dict = enrich_blueprint_with_retrievers(blueprint_dict, Path("."))
   ```

   The helper consumes `retrievers.ast_neighbors` and `retrievers.import_graph`; its output MUST be considered before writing `affected_files` and `dependencies` to YAML. This is still a human-operated blueprint step, not an automatic dispatcher or phase runner.


### Phase 2: Technical Strategy
Design the implementation path:
- Which files need modification?
- Which symbols need creation or modification?
- What existing tests must pass?
- What new tests must be written?

### Phase 3: Generation
Generate the following in the task directory:
1. `01-blueprint.yaml`: valid against `.agents/schemas/blueprint.schema.json`.
2. `02-contract.yaml`: valid against `.agents/schemas/contract.schema.json`.

## 📝 Blueprint Schema (`01-blueprint.yaml`)

```yaml
task_id: FEAT-001
summary: Implement payment-method enum in sale flow. Touches POS state, UI selector, and tests.
affected_files:
  - src/pos/sale.py
symbols:
  - Sale.payment_method
dependencies:
  - tests/pos/test_sale.py
test_scenarios:
  - Default method remains CASH.
  - Selecting QR stores QR before sale completion.
strategy:
  - step: 1
    action: Add enum and default value.
    files:
      - src/pos/sale.py
```

## 📝 Contract Logic
`02-contract.yaml` includes:
- `task_id`, `summary`, `goal`
- `read`: files useful for context
- `touch`: all files allowed to change
- `forbid.files` and `forbid.behaviors`
- `verify.commands`: fast unit/lint commands for agent loop
- `acceptance.bdd_suite`: optional slower human merge gate
- `limits`, `execution`, and `retry_policy`

## 🚫 Rules
- Do NOT write implementation code.
- Do NOT modify source code files.
- Stay within the `active/` directory for artifact generation.
- Keep YAML shallow. Intended max nesting depth is 3 levels.
- Quote ambiguous YAML scalar strings.

### Phase 4: Risk Scoring (Advisory)

After generating `01-blueprint.yaml`, invoke the risk scorer to suggest a level:

```python
import sys
import yaml

sys.path.insert(0, ".agents")

from cli.core.risk_scorer import assign_risk_level, build_risk_trace_events

with open(".agents/policies/risk.yaml") as f:
    policy = yaml.safe_load(f)
with open(f".ai/tasks/active/{task_id}/01-blueprint.yaml") as f:
    blueprint = yaml.safe_load(f)
with open(f".ai/tasks/active/{task_id}/00-spec.yaml") as f:
    spec = yaml.safe_load(f)

result = assign_risk_level(blueprint, policy)
events = build_risk_trace_events(spec.get("risk"), result)
```

Then:

1. **Append the events to `07-trace.json`**. The first is always `risk_level_calculated`; the second appears only when human-declared and calculated risk diverge.
2. **If `00-spec.yaml.risk` is already set by the human and differs from the calculated level**, append `risk_level_divergence` with `{declared, calculated, reasons}`. Do NOT modify `00-spec.yaml`.
3. **If `00-spec.yaml.risk` is unset**, suggest the calculated level to the human in your response. Do NOT write to `00-spec.yaml` directly.

Authority: the human's declared `risk` always wins. The scorer is advisory.

## 🛑 Handoff (single-phase boundary + forward auto-transition)

This skill executes ONLY the **Blueprint + Contract** phase. After escribir `01-blueprint.yaml`, `02-contract.yaml` y los eventos de riesgo en `07-trace.json`, hacés DOS cosas y parás:

1. **Auto-ejecutá** la transición de estado: corré una sola vez por shell el comando `quorum task start <TASK_ID>`. Esto crea el worktree (`worktrees/<TASK_ID>/`), la rama `ai/<TASK_ID>` e inicializa el `07-trace.json` si todavía no existe. Si el CLI imprime error, NO sigas: reportá `BLOCKED: <stderr>` y terminá con el indicador de espera.
2. **Imprimí el bloque de cierre estructurado** y terminá.

NO actives ningún otro skill. NO ejecutes `verify.commands`. NO escribas código fuente. NO toques `00-spec.yaml.risk` (la autoridad es del humano; sólo registrás divergencias en `07-trace.json`).

Si tu análisis arroja `BLOCKED` (spec inconsistente, archivos requeridos no existen, etc.), NO corras la transición: reportá el bloqueo y dejá la tarea como está.

Cuando completes la fase con éxito, cerrá el mensaje final exactamente con este bloque (en español):

```text
=== Fin de fase: Blueprint + Contrato ===

Artefactos producidos:
- .ai/tasks/active/<TASK_ID>-<slug>/01-blueprint.yaml
- .ai/tasks/active/<TASK_ID>-<slug>/02-contract.yaml
- .ai/tasks/active/<TASK_ID>-<slug>/07-trace.json (eventos risk_level_calculated y, si aplica, risk_level_divergence)

Transición de estado ejecutada:
- quorum task start <TASK_ID> ✓ (worktree en worktrees/<TASK_ID>/, rama ai/<TASK_ID>)

Pasos siguientes (los despacha el orquestador, NO yo):
1. [Opcional pero recomendado] /q-analyze <TASK_ID> — auditoría read-only de consistencia entre 00/01/02 antes de tocar código. Despachalo a un modelo barato.
2. [Obligatorio] /q-implement <TASK_ID> — implementa dentro del contrato en el worktree.

Si algo no quedó bien y querés volver atrás:
- quorum task back <TASK_ID> — borra el worktree y la rama (si está vacía). La tarea queda en active/ con 01/02 intactos para que vuelvas a despachar /q-blueprint y reescribirlos.

ESPERANDO RESPUESTA DEL USUARIO...
```

Auto-encadenar al siguiente skill viola la Regla #9. La auto-transición a `quorum task start` está autorizada porque elimina fricción sin saltar fases.
