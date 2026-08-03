# 21 — Exponer gemini-3.6-flash en el transporte `agy` base (one-shot)

**Fecha:** 2026-07-31
**Origen:** sesión de trabajo sobre el skill `think-cheap` (`~/.claude/skills/think-cheap/SKILL.md`). Se añadió un "rung 0 externo" al skill: exploración autocontenida LOW/MEDIUM se delega a agy ANTES del catálogo interno de subagentes Claude. La celda de resumen/análisis usa `agy` base (one-shot `--print`, ciega al filesystem), que hoy solo ofrece gemini-3.5-flash.
**Estado:** ✅ implementada (2026-07-31) — con un diseño AMPLIADO tras revisión adversaria. Ver resolución abajo.

## Resolución (2026-07-31)

La revisión adversaria refutó la premisa "no cambia código Go ni schemas": el
enum de `agents.yaml` es input directo del router (`enumerateCandidates` en
`internal/core/fleet_route.go`), y la simetría ingenua rompía la garantía G1
del nivel 0 (reroute siempre cross-provider): con 3.6 en ambos transportes, al
fallar `agy_edit` el siguiente candidato habría sido `agy` (one-shot, incapaz
de editar) con el MISMO modelo. Diseño ratificado en tres partes:

1. **Catálogo simétrico** — el trío 3.6-flash se añadió a `agy` base (qué
   modelos EXISTEN es verdad del catálogo, no política).
2. **Capacidad declarada como data** — campo `mode: agentic | oneshot` por
   transporte en `agents.yaml` + `agents.schema.json` (opcional, ausente =
   agentic). `agy` es el único `oneshot`.
3. **Filtro por capacidad en `core.Route`** — los transportes `oneshot` se
   excluyen de la enumeración de candidatos SOLO en fase `implement`
   (data-driven, cero nombres hardcodeados; exclusión par-a-par intacta).

Evidencia: `go test ./...` verde (CGO_ENABLED=0) con dos tests nuevos de
routing (filtro en implement + elegibilidad en fase no-implement), y smoke
one-shot 3/3 (`low`/`medium`/`high`, exit 0, output sano, sin timeout,
registrado en el ledger de fleet-delegate).

**Hallazgo colateral:** declarar `agy` oneshot dejó el fallback del nivel 1
(`google/gemini-3.1-pro-low`, solo en `agy` base) sin candidato viable para
implement — era un bug latente (un one-shot jamás pudo ejecutar un implement).
Con codex deshabilitado, el nivel 1 ahora bloquea con `no_viable_candidate` en
vez de enrutar a un transporte incapaz. Decisión de política pendiente para el
humano: exponer una celda gemini en `agy_edit` para nivel 1, o ajustar
`config.yaml.levels[1]`.

## Problema

El trío `google/gemini-3.6-flash-{low,medium,high}` vive SOLO en la celda
`agy_edit` (`.agents/fleet/agents.yaml:220-228`), validado en modo agéntico
(pass@3 S/M, FLEET-19, 2026-07-30). El transporte `agy` base conserva
`gemini-3.5-flash-*` / `gemini-3.1-pro-*` porque el enum por celda es
EVIDENCIA: cada modelo listado se validó en el modo de esa celda, y 3.6 nunca
se validó en one-shot `--print`.

Consecuencia: el rung 0 del skill think-cheap usa 3.5-flash para resúmenes
cuando 3.6-flash (generación más reciente, misma suscripción) ya está
disponible en la máquina — solo falta darlo de alta con evidencia.

## Propuesta

Micro-tarea de edición directa + validación ligera, SIN cambio de código Go:

1. **Editar `.agents/fleet/agents.yaml`**: añadir
   `google/gemini-3.6-flash-low|medium|high` al enum de modelos de la celda
   `agy` base (mismas tres entradas que ya tiene `agy_edit`).
2. **Validación one-shot mínima**: por cada effort dado de alta, un
   `quorum fleet run --agent agy --model google/gemini-3.6-flash-<e> --json`
   con un prompt de resumen sobre material inline; verificar envelope sano
   (exit 0, output no vacío, sin timeout).
3. **Tests**: correr `go test ./...` (CGO_ENABLED=0) — hay tests que
   referencian `agents.yaml` (`fleet_run_test.go`, `fleet_adapter_agy_test.go`,
   preflight); confirmar que ninguno pinea el enum viejo.
4. **Commit convencional** en quorum citando la evidencia de (2) en el mensaje.

## Qué NO cubre

- No toca `agy_edit` ni ningún otro transporte.
- No retira 3.5-flash del enum (retiro = decisión aparte, cuando 3.6 acumule
  evidencia de uso real en el ledger del skill).
- ~~No cambia código Go ni schemas: `agents.yaml` es data hot-load.~~
  **REFUTADO en revisión** (ver resolución): el enum alimenta al router; la
  simetría exigió el campo `mode` + filtro por capacidad en `core.Route`.

## Coherencia con las decisiones de la serie

- "Compliance by construction / enum = evidencia": el alta va ACOMPAÑADA de su
  validación, nunca antes.
- ~~Transporte ≠ política: la celda base sigue siendo one-shot ciega; solo
  cambia su lista de modelos.~~ **MATIZADO en revisión**: la lista de modelos
  SÍ es input de la política de routing; la separación limpia se logró
  declarando la capacidad (`mode`) como data y filtrando en `Route`.
- Consumidor inmediato: skill `think-cheap` rung 0 (celda de resumen pasaría
  de 3.5-flash a 3.6-flash una vez validado). El skill se actualiza DESPUÉS
  de que esta tarea cierre, no antes.

## Esfuerzo estimado

TRIVIAL. 3 líneas de YAML + 3 runs de humo + `go test ./...`.

## Decisiones abiertas

1. ¿Dar de alta los tres efforts (low/medium/high) o solo low/medium, que son
   los que consume el rung 0? (Propuesta: los tres — el costo marginal es un
   run de humo más y evita reabrir la tarea.)
2. ¿Basta el smoke test del punto 2 o se corre la suite oculta de 9 casos
   usada en validaciones anteriores? (Propuesta: smoke basta — la suite de 9
   casos mide capacidad AGÉNTICA, irrelevante para one-shot.)

## Seguimiento

Al cerrar: actualizar `~/.claude/skills/think-cheap/SKILL.md` (celda de
resumen del rung 0 → 3.6-flash) y el snapshot de
`~/.claude/skills/fleet-delegate/references/limits.md`. Eso lo hace la sesión
origen, no esta tarea.
