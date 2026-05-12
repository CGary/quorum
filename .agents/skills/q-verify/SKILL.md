---
name: q-verify
description: Run a Quorum task's fast verification commands from 02-contract.yaml and capture results in 05-validation.json. Use after implementation or whenever validation evidence is needed.
user-invocable: true
---

# /q-verify - Quorum Functional Verifier

## 🌐 Communication Protocol (vinculante para todo output)

- **Idioma**: SIEMPRE respondé en español.
- **Indicador de espera**: solo cuando el turno requiera una pregunta explícita o exista una decisión humana/despacho pendiente, cerrá el mensaje con `ESPERANDO RESPUESTA DEL USUARIO...` como última línea (mayúsculas, tres puntos, sin texto después). Si el turno es puramente informativo, omití este indicador.
- **Sin fence final**: los bloques `text` de este archivo son ejemplos de documentación. Cuando emitas el cierre al usuario, NO envuelvas el Handoff en triple backticks si eso deja una línea después del indicador; la última línea visible debe ser `ESPERANDO RESPUESTA DEL USUARIO...`.
- **Prefijo de contexto CLI**: el wrapper `quorum` imprime como primera línea de stdout `[root]` cuando se ejecuta desde la raíz del proyecto o `[worktree:<TASK_ID>]` cuando se ejecuta desde un worktree, detectado dinámicamente vía `git rev-parse`. Al describir comandos al usuario, no inventes ni hardcodees ese prefijo; si `git rev-parse` falla la línea se omite y el subcomando se ejecuta normalmente.

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

## 🛑 Handoff (single-phase boundary)

This skill ejecuta SOLO la fase **Verification**. No hay transición de estado para auto-ejecutar — el worktree ya existe y la tarea sigue en active/.

NO actives ningún otro skill. NO edites código fuente para arreglar fallos (eso es `/q-implement`). NO decidas reintentos. NO escribas `06-review.json` ni juzgues el diff. NO corras la suite BDD (es compuerta humana).

Cerrá el mensaje final exactamente con este bloque (en español):

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
- quorum task back <TASK_ID> — borra worktree y rama (perdés commits no mergeados).

ESPERANDO RESPUESTA DEL USUARIO...
```

Auto-encadenar viola la Regla #9.
