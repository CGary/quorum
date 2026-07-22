# ADR 0011: Taxonomía Attempt/Reroute/Blocked y Convención de Eventos en Trace

## Estado
Aceptado

## Contexto

El dispatcher de flota (ver `ideas/fleet/`) introduce dos clases de fallo nuevas sobre el contador de reintentos que ya existe a nivel de contrato: reroutes por infraestructura (quota, timeout/crash, salida imparseable del wrapper) y bloqueos human-in-the-loop (un dispatch que hace una pregunta y espera respuesta humana). Hoy `02-contract.yaml.retry_policy.max_attempts` y `.agents/policies/routing.yaml` ya gobiernan los intentos a nivel de contrato, pero nada distingue "el modelo lo intentó y falló" de "la infraestructura nunca le dio al modelo una oportunidad justa" de "el modelo está esperando una respuesta humana". Sin una semántica fija, un 429 de un vendor quemaría silenciosamente un intento del contrato, y un modelo simplemente incapaz se vería idéntico a la mala suerte.

Restricciones verificadas contra el schema y el código real (no asumidas):

- `attempts[]` en `07-trace.json` tiene `additionalProperties: false` y solo acepta `phase/result/model/tokens_in/tokens_out/cost_usd/duration_s/notes` (`.agents/schemas/trace.schema.json`). `phase` es un enum cerrado que incluye `execute`, no `implement`.
- `events[]` ya existe en el schema de trace como `{"type": "array", "items": {"type": "object", "additionalProperties": true}}` — objetos libres, sin protección append-only.
- `EnsureTraceAppendOnly` (`internal/core/artifact.go:12`) hoy solo protege `attempts[]`; no tiene noción de `events[]`.

Esta ADR no implementa el motor de dispatch, los adapters, el router ni el kill-switch — esas son tareas de flota posteriores. Fija por escrito la taxonomía y la decisión de append-only para que esas tareas futuras construyan sobre una base estable.

## Decisión

### 1. Taxonomía por señal observable

La clasificación se basa estrictamente en lo que un intento de dispatch produjo observablemente al terminar — nunca en la interpretación de *por qué* ocurrió. Esto mantiene al clasificador determinista y auditable.

| Señal al terminar el dispatch | Clase | Consume | Registro |
|---|---|---|---|
| Diff no vacío (pase o no verify) | **attempt** | `max_attempts` del contrato | `attempts[]` + referencia forense |
| Exit 0 + diff vacío + notas vacías | **attempt** (con flag `noop` en el evento asociado) | `max_attempts` del contrato | `attempts[]` + `events[]` |
| Diff inválido o inaplicable | **attempt** | `max_attempts` del contrato | `attempts[]` |
| Firma de quota con diff vacío | **reroute** | `reroute_budget` del dispatch (default 2) | `events[]` (+ señal roja de kill-switch) |
| Timeout o crash con diff vacío | **reroute** | `reroute_budget` del dispatch (default 2) | `events[]` |
| Salida imparseable (el CLI del vendor cambió flags/formato) | **reroute**, con flag `wrapper_broken` | `reroute_budget` del dispatch (default 2) | `events[]` |
| Señal `BLOCKED` con pregunta válida | **blocked** | nada | `events[]`; la tarea queda parqueada esperando respuesta humana |

Jerarquía: el `max_attempts` del contrato es autoritativo para los resultados de clase `attempt`. `reroute_budget` pertenece estrictamente a la capa de dispatch y nunca decrementa el `max_attempts` del contrato — un reroute es un reintento de infraestructura, no un intento fallido del trabajo en sí. `blocked` no consume ninguno de los dos contadores: esperar a un humano no es un fallo ni del contrato ni del dispatch.

### 2. Mapeo a trace, sin extender el schema

La forma actual de `07-trace.json` ya cubre todo lo necesario:

- Un dispatch delegado de `q-implement` que produce un resultado de clase `attempt` se registra como `attempts[].phase = "execute"` (no `"implement"` — el enum del schema no tiene ese valor). `model` lleva el nombre canónico del modelo. `tokens_in`, `tokens_out` y `cost_usd` se completan solo cuando `usage.source = cli_reported`; cuando el wrapper del CLI no reporta uso, esos campos se omiten por completo — nunca se fabrican ni se estiman.
- Todo lo que sea más rico de lo que permite la forma cerrada de `attempts[]` (dispatch id, nombre del agente, clase del resultado, flags `noop`/`wrapper_broken`, hash del bundle, referencia forense, `usage.source`, diff stat) va a `events[]`, que ya es un array de objetos libres en el schema.
- `events[].type` es un **vocabulario cerrado**, fijado por esta ADR: `routing_decision`, `dispatch_started`, `dispatch_finished`, `reroute`, `wrapper_broken`, `quota_red`, `blocked_question`, `blocked_answer`, `review_family_degraded`. Ningún otro valor es válido; agregar un tipo nuevo requiere enmendar esta ADR. Todo evento lleva `ts` (timestamp) y `dispatch_id`, para poder correlacionar eventos a lo largo del ciclo de vida de un mismo dispatch sin importar su tipo.

Esta decisión no requiere ni permite ningún cambio a `.agents/schemas/trace.schema.json` — la forma actual de `events[]` (`additionalProperties: true`) ya acomoda los campos anteriores.

### 3. Cerrar el hueco de append-only en `events[]`

`EnsureTraceAppendOnly` hoy solo inspecciona `attempts[]`; un save descuidado podría reescribir o truncar `events[]` silenciosamente sin que ninguna validación lo detecte. Esta ADR elige la **opción (a)**: extender `EnsureTraceAppendOnly` para que los `events[]` existentes queden protegidos por el mismo criterio ya aplicado a `attempts[]` — sin remoción (el array nuevo no puede ser más corto que el existente), sin reordenamiento, sin mutación de ninguna entrada existente. Solo se permite agregar eventos nuevos al final. Esto también debe cubrir el caso de un payload de trace viejo, anterior a la existencia de `events[]` (el campo está ausente), tratándolo como una base vacía, para que no falle y no bloquee la adopción en tareas ya existentes.

Este es un cambio de función pequeño y aditivo, más tests que reflejan los tests existentes de append-only de `attempts[]` en `internal/core/task_manager_test.go` (`TestValidateArtifactErrorFormatAndTraceAppendOnly`) — sin archivos nuevos, sin cambio de schema.

## Consecuencias

- **Positivas**: el manejo de reroute/blocked del dispatcher tiene un contrato fijo y auditable contra el cual implementar; la inestabilidad de infraestructura (quota, crash, wrapper roto) nunca puede consumir silenciosamente el `max_attempts` de un contrato; el bloqueo human-in-the-loop es explícitamente gratuito para la tarea.
- **Negativas**: el clasificador debe implementarse con cuidado para mantenerse basado en señales (no en interpretación), o la garantía de determinismo de la taxonomía se erosiona con el tiempo; el crecimiento sin límite de `events[]` en tareas de vida larga es un tradeoff aceptado del historial append-only (refleja el mismo tradeoff ya existente en `attempts[]`).
- **Neutrales**: esta ADR no implementa el motor de dispatch, el router, los adapters ni el kill-switch; solo fija la semántica y cierra el hueco de append-only para que esas tareas futuras construyan sobre una base estable. El valor default de `reroute_budget` (2) se acepta tal cual según las restricciones de la spec y no se relitiga aquí.

## Amendment (2026-07-18): a fourth outcome class, `staging_failed`

Implementation of the fleet dispatch engine (`internal/core/fleet_dispatch.go`) surfaced a
failure mode the original taxonomy in Decision §1 did not anticipate: `git add -A` (or the
subsequent `git diff --cached --numstat`) itself failing before any classification of the
delegate's work is even possible — an `index.lock` left over from another process, a full disk,
or a permissions problem on the worktree. This is not a vendor/model failure and not a wrapper
failure; it is local infrastructure breaking before the dispatch has a chance to observe
anything about the delegate's output. Retrofitting it into `attempt` or `reroute` would either
burn a contract attempt the model never got a fair shot at, or burn a reroute budget slot that
implies the *delegate* misbehaved, when the delegate may have done nothing wrong at all.

This amendment adds `staging_failed` as a fourth, disjoint outcome class, governed by the same
signal-based discipline as Decision §1:

| Signal at dispatch end | Class | Consumes | Record |
|---|---|---|---|
| Staging/diff computation itself errors (`git add -A` or `git diff --cached` fails) | **staging_failed** | nothing | `events[]` only (`dispatch_started` + `dispatch_finished`) |

Rules, mirroring the reasoning already established for `blocked` in Decision §1:

- **Consumes no reroute budget.** `reroute_budget` exists to bound retries against a delegate/
  vendor that is misbehaving. Local infra failure says nothing about the delegate, so it must
  not count against it.
- **Adds no exclusions.** Nothing about routing, model selection, or the delegate's fitness for
  this task is implicated — the model was never given the chance to run to a point where its
  behavior could be judged. No exclusion list, cooldown, or routing signal is written.
  Reusing `internal/core/fleet_route.go` terminology: `staging_failed` produces no reroute-style
  candidate exclusion at all, unlike quota/timeout/wrapper_broken reroutes.
- **Appends no `attempts[]` entry.** Per Decision §2, `attempts[]` records classified work
  against a contract's `max_attempts`. A staging failure produced no classifiable work.
- **Emits only `dispatch_started`/`dispatch_finished`.** No new event type is introduced — both
  already exist in Decision §2's closed vocabulary. The outcome's `class` and `cause` (the
  underlying git error text) travel in `dispatch_finished`'s existing free-form payload, exactly
  as `reroute`/`blocked` causes already do.
- **The worktree is always left untouched for forensics.** Because `diffStat.Empty` from a
  failed staging call is indeterminate (not a legitimate empty diff), `git reset --hard` must
  never run on this path — not even when a forensic ref happens to be captured, since
  `write-tree` only snapshots whatever made it into the index before `add -A` failed, and may be
  missing the exact working-tree deltas that caused the failure in the first place. Reset --hard
  remains allowed only when `diffErr == nil` **and** (the diff is confirmed legitimately empty
  **or** a forensic ref was captured against a diff that staging/diff computation itself
  succeeded in measuring). Best-effort forensic capture is still attempted on the
  `staging_failed` path (it is harmless when it fails, and useful when it succeeds), but its
  success or failure never gates the reset decision.
- **Orchestrator contract: surface to human, retry is human/orchestrator-initiated.** Consistent
  with `docs/adr/0001-q-implement-child-retry.md`, a `staging_failed` result is not something a
  skill or the dispatcher silently retries or reroutes on its own; it is surfaced as-is so a
  human or the orchestrator can decide whether to retry the dispatch, fix the underlying
  infrastructure problem, or intervene manually. This keeps `staging_failed` symmetrical with how
  `blocked` is already handled: a class that consumes neither counter and defers the next action
  to a human.

No schema change is required: `events[]` already accommodates this via its existing
`additionalProperties: true` shape, exactly as Decision §2 established for `reroute`/`blocked`.

## Amendment (2026-07-22): a fifth outcome class, `spawn_failed`

A dispatch where the delegate process itself never started (e.g. `cmd.Start()`/execve error such as E2BIG, missing or non-executable binary, permission denial) must be classified separately from `attempt`, `reroute`, `blocked`, and `staging_failed`.

This amendment adds `spawn_failed` as a fifth, disjoint outcome class, governed by the same signal-based discipline:

| Signal at dispatch end | Class | Consumes | Record |
|---|---|---|---|
| Delegate process never started (`cmd.Start()`/execve fails) | **spawn_failed** | nothing | `events[]` only (`dispatch_started` + `dispatch_finished`) |

Rules, mirroring the reasoning already established for `staging_failed`:

- **Consumes no attempt/reroute budget.** The delegate's fitness for the task was never observed, so it consumes no `max_attempts` nor `reroute_budget`.
- **Adds no exclusions.** No routing exclusion, cooldown, or penalty is written.
- **Appends no `attempts[]` entry.** No classifiable work was produced.
- **Emits only `dispatch_started`/`dispatch_finished`.** No new event type is introduced; the outcome's `class` and `cause` (the underlying execve error text) travel in `dispatch_finished`'s existing free-form payload.
- **The worktree is untouched.** A spawn failure never produces a forensic ref and never runs git reset on the worktree, since the delegate never touched it and no diff is possible (it is short-circuited before staging is even attempted).
- **Orchestrator contract: surface to human, retry is human/orchestrator-initiated.** Defers the next action to a human/orchestrator.

No `trace.schema.json` change is required or permitted; `events[]` (`additionalProperties: true`) already accommodates the new class.
