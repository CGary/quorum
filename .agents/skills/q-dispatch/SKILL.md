---
name: q-dispatch
description: Drive one implement-phase external fleet delegation cycle (quorum fleet route -> human confirmation -> quorum fleet bundle -> quorum fleet dispatch -> ADR 0011 outcome report) for a Quorum task that already has a worktree, 01-blueprint.yaml, and 02-contract.yaml. Use to hand a task's implementation off to an external delegate CLI (agy/opencode/aider) instead of implementing it directly.
user-invocable: true
---

# /q-dispatch - Quorum Fleet Dispatch Face

## 🌐 Communication Protocol (binding for all output)

- **Language**: ALWAYS respond in Spanish for EVERY message visible to the user (summaries, reports, handoffs, blocks, and questions), regardless of the language of the input, internal documentation, field names, or artifacts read. Do not use English templates for the user-facing closing.
- **Waiting indicator**: only when the turn requires an explicit question or there is a pending human decision/dispatch, close the message with `ESPERANDO RESPUESTA DEL USUARIO...` as the last line (uppercase, three dots, nothing after). If the turn is purely informational, omit this indicator.
- **No trailing fence**: the `text` blocks in this file are documentation examples. When you emit the user-facing closing, do NOT wrap the Handoff in triple backticks if that leaves a line after the indicator; the last visible line must be `ESPERANDO RESPUESTA DEL USUARIO...`.
- **CLI context prefix**: the `quorum` wrapper prints as the first stdout line `[root]` when run from the project root, or `[worktree:<TASK_ID>]` when run from a worktree, detected dynamically via `git rev-parse`. When describing commands to the user, do not invent or hardcode that prefix; if `git rev-parse` fails the line is omitted and the subcommand runs normally.

You are the **Fleet Dispatch Face**: the single-phase human interface to the EXTERNAL fleet
(`quorum fleet route`, `quorum fleet bundle`, `quorum fleet dispatch`). You decide no routing and
compute no selection yourself — every candidate, level, and outcome comes from those CLIs. Claude
Code subagents (`.claude/agents/`) are out of scope for this skill; they belong to orchestrator
scaffolding, not to `/q-dispatch`.

## Scope

v1 covers only the **implement** phase for one task. `brief`/`decompose`/`blueprint`/`accept`
stay orchestrator-run; `verify` has no LLM step; review routing is a router concern, not a
`/q-dispatch` concern. This is a **single-phase** skill: it runs one route/confirm/dispatch cycle
(or one bounded reroute loop within that cycle) and stops.

## Authority

Read, in this order: `02-contract.yaml` (touch/forbid, binding for the aider guard),
`01-blueprint.yaml`, `00-spec.yaml` (`risk`, human-declared and never overwritten by the router).

## 1. Preconditions

Before anything else, confirm all of:

- The task is under `.ai/tasks/active/<TASK_ID>/`.
- A worktree exists at `worktrees/<TASK_ID>/` (created by `quorum task start`).
- `01-blueprint.yaml` and `02-contract.yaml` are present in the task directory.

If any precondition is missing, stop informationally — no transition, no state mutation, no wait
indicator (this is not a question, just a status report):

```text
Precondiciones: incompletas
Falta: <lista de lo que falta: active/, worktree, 01-blueprint.yaml, 02-contract.yaml>

Pasos siguientes (los despacha el orquestador, NO yo):
- [Obligatorio] Resolver lo faltante (p. ej. correr /q-blueprint <TASK_ID> si falta 01/02, o
  quorum task start <TASK_ID> si falta el worktree) y volver a invocar /q-dispatch <TASK_ID>.
```

## 2. Complexity Score

Build the stdin JSON for `quorum analyze complexity-score` from the blueprint's
`affected_files`/`symbols` and `.agents/policies/complexity.yaml`:

```json
{"blueprint": {"affected_files": [], "symbols": [], "migration": false, "public_api": false, "schema_change": false}, "policy": {"calibrated": false, "s_max_files": 2, "s_max_symbols": 3, "l_max_files": 5}}
```

Capture the advisory `band` (S/M/L). You never decide the band yourself; it is `complexity_band`
input to the router.

## 3. Fleet Route

Build the stdin JSON for `quorum fleet route`. On the first call for this dispatch cycle,
`exclusions` is always an empty list:

```json
{"task_id": "<TASK_ID>", "phase": "implement", "risk": "<risk from 00-spec.yaml>", "complexity_band": "<S|M|L>", "exclusions": []}
```

Parse the `RouteResult`: `candidate {agent, model, level}`, `reroute_budget`,
`review_family_degraded`, `blocked`, `reasons`, `inputs_snapshot`. If `blocked` is set (e.g.
`no_viable_candidate`), no candidate survived filtering — show `reasons` as an actionable
informational message and end the turn (no wait indicator; this is a dead end, not a question):

```text
Ruteo: sin candidato viable
Motivo: no_viable_candidate
Señales: <reasons[] tal cual las devolvió el router>

Pasos siguientes (los despacha el orquestador, NO yo):
- [Obligatorio] Revisar routing.yaml/config.yaml/agents.yaml (transportes inactivos, exclusiones
  agotadas) o implementar la tarea directamente con /q-implement <TASK_ID>.
```

## 4. Decision Display + Mandatory Confirmation

Unconditionally in v1, for every risk level, show the router's decision and wait for explicit
human confirmation before bundling or dispatching:

```text
=== Decisión de ruteo ===

Agente: <candidate.agent>
Modelo: <candidate.model>
Nivel: <candidate.level>
Reroute budget: <reroute_budget> salto(s) disponibles
Diversidad de familia de revisión degradada: <si review_family_degraded, nota breve; si no, omitir la línea>
Resumen de inputs_snapshot: risk=<risk> band=<complexity_band> phase=implement router_version=<router_version> hashes(config/routing/agents)=<primeros 8 chars de cada hash>
Por qué: <reasons[] o señales relevantes del RouteResult>

[Obligatorio] ¿Confirmás el dispatch?
Respondé: si / elegir otro / cancelar

ESPERANDO RESPUESTA DEL USUARIO...
```

If the human answers "elegir otro", append `candidate` to `exclusions` and re-run step 3 (this is
the same accumulating-exclusions mechanism as a reroute, driven by human choice instead of an
infrastructure signal). If "cancelar", end the turn informationally with no dispatch.

## 5. Aider Mechanical-Single-File Guard

Run this AFTER route and BEFORE confirmation is acted upon, whenever `candidate.agent == "aider"`.
`.agents/fleet/agents.yaml` documents aider's mechanical/single-file restriction only as
**operational intent, explicitly "NOT enforced by core.Route"**; `config.yaml` only
de-prioritizes aider in `policies.fleet_transport_order`. Neither enforces the restriction. This
SKILL.md is therefore the **sole enforcement point**:

- Read `02-contract.yaml` `touch`. If `len(touch) > 1`, the change is not single-file.
- Judge mechanicalness from `01-blueprint.yaml`/`02-contract.yaml`: a change is mechanical when it
  is a scoped, deterministic edit (formatting, single well-defined function/doc, no novel design
  judgment); anything requiring architectural decisions is not mechanical.
- If either check fails, do not proceed to confirmation for this candidate. Show a Spanish
  warning and re-route with aider appended to `exclusions` (never reset to empty), then go back to
  step 4 with the new candidate:

```text
Guardia aider: rechazada
Motivo: <touch tiene N archivos (>1) y/o el cambio no es mecánico>

Re-ruteando con aider excluido...
```

## 6. Fleet Bundle

Run `quorum fleet bundle <TASK_ID>` for the confirmed candidate. It writes
`dispatch/<dispatch_id>/{prompt.md,manifest.json}` under the task directory, where `dispatch_id`
is `bundle_hash[:12]`. Capture `bundle_path` (the `prompt.md` path) and `dispatch_id` from stdout.

## 7. Fleet Dispatch

Build the stdin JSON for `quorum fleet dispatch`:

```json
{"task_id": "<TASK_ID>", "agent": "<candidate.agent>", "model": "<candidate.model>", "bundle_path": "<prompt.md path>", "dispatch_id": "<dispatch_id>"}
```

`timeout_s` is optional. This writes `dispatch/<dispatch_id>/result.json`. Parse `outcome.class`,
`outcome.noop`, `outcome.cause`, `outcome.blocked {path, reason, severity}`, `diff`,
`forensic_ref`, `applied`.

## 8. ADR 0011 Outcome Handling

Derive the presentation case from `outcome.class` + `outcome.noop` + `applied` + `diff.empty`,
per `docs/adr/0011-attempt-reroute-blocked-trace.md`:

- `outcome.class == "attempt"`, `applied == true`, `diff.empty == false` -> **attempt_done**.
- `outcome.class == "attempt"`, `diff.empty == true`, `outcome.noop == true` -> **noop**.
- `outcome.class == "attempt"`, `diff.empty == true`, `outcome.noop == false` -> **attempt_failed**.
- `outcome.class == "reroute"` -> **reroute** (cause is `quota_red`/`timeout`/`wrapper_broken`).
- `outcome.class == "blocked"` -> **blocked**.

### attempt_done

```text
=== Dispatch: attempt_done ===

Diff aplicado: <diff.files_changed> archivo(s), +<diff.insertions>/-<diff.deletions>
Referencia forense: <forensic_ref>

Pasos siguientes (los despacha el orquestador, NO yo):
- [Obligatorio] /q-verify <TASK_ID> — corre verify.commands sobre el diff aplicado.
```

### attempt_failed / noop

```text
=== Dispatch: attempt_failed | noop ===

Evidencia: diff vacío. <notas del delegate si las hay>

Pasos siguientes (los despacha el orquestador, NO yo):
- [Opcional] Reintentar con otro candidato: volver al paso 3 con el candidato agregado a exclusions.
- [Opcional] Implementar la tarea directamente con /q-implement <TASK_ID>.
- Si querés descartar el intento del contrato: [ROOT] quorum task back <TASK_ID> (rollback humano).
```

### reroute

Append the failed candidate to the accumulated `exclusions` (never reset to empty), re-invoke
`quorum fleet route` (step 3), and show the next candidate for confirmation, bounded by
`reroute_budget`:

```text
=== Dispatch: reroute ===

Causa: quota_red | timeout | wrapper_broken
Saltos de reroute restantes: <reroute_budget - saltos ya usados>

[Obligatorio] ¿Confirmás el dispatch al siguiente candidato?
Respondé: si / elegir otro / cancelar

ESPERANDO RESPUESTA DEL USUARIO...
```

When `reroute_budget` is exhausted and the router returns `blocked: no_viable_candidate`, stop
with the step 3 no-viable-candidate message (informational, no wait indicator).

### blocked (rich Spanish question)

Format `outcome.blocked {path, reason, severity}` as a rich question: context, evidence,
consequential options, a recommendation, and an open option, ending in the mandatory indicator:

```text
=== Dispatch: BLOCKED ===

Contexto: el delegate emitió una señal BLOCKED y quedó a la espera de una decisión humana.
Evidencia: path=<outcome.blocked.path> reason=<outcome.blocked.reason> severity=<outcome.blocked.severity>

Opciones (con consecuencias):
1. Resolver lo señalado y re-despachar /q-dispatch <TASK_ID> — retoma el ciclo desde route.
2. Renegociar el contrato con /q-blueprint <TASK_ID> — si <path> está fuera de touch/read.
3. Implementar manualmente con /q-implement <TASK_ID> — si el bloqueo es de diseño, no de contrato.
Recomendación: <opción sugerida según severity: critical -> 2; minor -> 1>
Otra opción: contame qué preferís si ninguna de las anteriores encaja.

ESPERANDO RESPUESTA DEL USUARIO...
```

## Output

This report is user-visible: emit every template above in Spanish, with no English labels.
`/q-dispatch` never writes a lifecycle artifact itself — routing/dispatch telemetry (
`routing_decision`, `dispatch_started`, `dispatch_finished`, `reroute`, `blocked_question`, etc.)
is written by the CLIs directly into `07-trace.json`, never by this skill.

## Rules

- Do not compute or decide routing; only invoke `quorum fleet route`/`quorum fleet bundle`/
  `quorum fleet dispatch` and translate their output.
- Do not hardcode a model or agent name as a routing decision; every candidate comes from
  `RouteResult`.
- Do not relax the pre-dispatch confirmation by risk level; v1 requires it for every risk level.
- Do not dispatch to aider when the mechanical/single-file guard fails; re-route instead.
- Do not reset accumulated `exclusions` to empty on a reroute; only append.
- Do not introduce a new `events[]` type value or a new lifecycle artifact slot (03/08/09/10).
- **Language**: user chat is always Spanish; this skill persists no artifact of its own, so there
  is no field-value English rule to apply here.

## 🛑 Handoff (single-phase boundary)

This skill executes ONLY one route/confirm/dispatch cycle (with its bounded reroute sub-loop) for
the implement phase. `/q-dispatch` is not one of the three constitutionally authorized forward
auto-transitions (`/q-brief`, `/q-decompose`, `/q-blueprint`); it never runs a state transition.

Do NOT activate any other skill. Do NOT auto-run `/q-verify` on `attempt_done` — mark it
`[Obligatorio]` only. Do NOT call `quorum task back` — reference it only as human-only guidance.
Do NOT write `04-implementation-log.yaml`, `05-validation.json`, `06-review.json`, or `07-trace.json`
events yourself; those are written by the CLIs or by the phases that own them.

Close the final message exactly with one of the outcome blocks above (informational cases omit
the wait indicator; confirmation/blocked/reroute-confirm cases require it), followed by:

```text
No hay transición de estado: el worktree y la rama siguen iguales.

Si querés volver atrás:
- [WORKTREE:<TASK_ID>] git -C worktrees/<TASK_ID> reset --hard HEAD~1 — descarta el último commit
  aplicado por el delegate (solo si attempt_done ya aplicó un diff).
- [ROOT] quorum task back <TASK_ID> — borra worktree y rama (perdés commits no mergeados).
```

Auto-chaining violates Rule #9.
