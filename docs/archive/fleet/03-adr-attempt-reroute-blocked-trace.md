# 03 — ADR-B: semántica attempt/reroute/blocked + convención de eventos en trace

> **ARCHIVADO 2026-07-23 — IMPLEMENTADO.** docs/adr/0011-attempt-reroute-blocked-trace.md + eventos en internal/core/fleet_dispatch.go.

**Tipo:** ADR + (posible) extensión mínima de helper en core.
**Depende de:** 01 (G0). Independiente de 02; pueden ir en paralelo.
**Riesgo sugerido:** medium (define semántica que todo lo demás consume; errores acá se pagan en cada tarea futura).

## Contexto

Hoy conviven `routing.yaml.max_attempts` (por riesgo) y `02-contract.yaml.retry_policy`; la flota agrega reroutes por fallo de infraestructura y la decisión human-in-the-loop agrega la clase `blocked`. Sin una semántica única, un 429 quema reintentos del contrato o un modelo incapaz se disfraza de mala suerte. Además, el schema real de trace impone restricciones verificadas: `attempts[]` tiene `additionalProperties: false` (solo `phase/result/model/tokens_in/tokens_out/cost_usd/duration_s/notes`), `phase` usa `execute` (no `implement`), y `EnsureTraceAppendOnly` (`internal/core/artifact.go:12`) protege SOLO `attempts[]`, no `events[]`.

## Objetivo

ADR-B que fije, de una vez y por escrito:

### 1. Taxonomía por señal observable (no por interpretación)

| Señal al terminar el dispatch | Clase | Consume | Registro |
|---|---|---|---|
| Diff no vacío (pase o no verify) | **attempt** | `max_attempts` del contrato | `attempts[]` + ref forense |
| Exit 0 + diff vacío + notas vacías | **attempt** (flag `noop` en evento) | `max_attempts` | `attempts[]` + evento |
| Diff inválido/inaplicable | **attempt** | `max_attempts` | `attempts[]` |
| Firma de quota con diff vacío | **reroute** | `reroute_budget` del dispatch (default 2) | `events[]` + rojo en kill-switch |
| Timeout/crash con diff vacío | **reroute** | `reroute_budget` | `events[]` |
| Salida imparseable (vendor cambió flags) | **reroute** + alerta `wrapper_broken` | `reroute_budget` | `events[]` |
| Señal BLOCKED con pregunta válida | **blocked** | **nada** (ni attempts ni reroutes) | `events[]`; tarea parqueada esperando humano |

Jerarquía: el contrato manda en attempts; `reroute_budget` pertenece al dispatch y no toca el contador del contrato. `blocked` no castiga a nadie: esperar al humano no es fallo.

### 2. Mapeo a trace (con el schema actual, sin extenderlo)

- `q-implement` delegado → `attempts[].phase = "execute"`; `model` = nombre canónico; tokens/costo si `usage.source=cli_reported`, ausentes si no (jamás inventados).
- Metadata rica (dispatch_id, agent, outcome, noop, bundle_hash, forensic_ref, usage.source, diff_stat) → `events[]` (objetos libres, ya permitido).
- Convención de `events[].type` (cerrada por el ADR): `routing_decision`, `dispatch_started`, `dispatch_finished`, `reroute`, `wrapper_broken`, `quota_red`, `blocked_question`, `blocked_answer`, `review_family_degraded`. Cada evento lleva `ts` y `dispatch_id`.

### 3. El hueco de append-only en `events[]`

`EnsureTraceAppendOnly` no protege eventos: un save descuidado puede reescribirlos sin que la validación lo note. El ADR decide: **(a)** extender el helper para que los eventos existentes tampoco puedan mutarse/borrarse (propuesta recomendada: mismo criterio que attempts), o **(b)** aceptar el hueco documentado hasta Fase 3. Si (a), la implementación entra en esta tarea (función pequeña + tests espejo de los de attempts).

## Criterios de aceptación

- [ ] ADR-B aceptado en `docs/adr/` con la tabla de taxonomía y la convención de eventos.
- [ ] Decisión sobre append-only de `events[]` tomada; si (a), `EnsureTraceAppendOnly` extendido con tests (incluido el caso "payload viejo sin events").
- [ ] Documentado en CLAUDE.md/quorum.md la frase clave: implement delegado se registra como `phase: execute`.
- [ ] `go test ./...` verde.

## Decisiones abiertas para el brief

- ¿`reroute_budget` default 2 es razonable para una flota de 2 agentes? (con codex+agy, el segundo reroute ya no tiene a dónde ir salvo `claude` opcional).
- Append-only de eventos: ¿opción (a) ahora o (b) documentar y diferir?
