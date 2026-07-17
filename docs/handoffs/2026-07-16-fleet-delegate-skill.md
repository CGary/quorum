# Handoff — Skill `fleet-delegate` (sesión 2026-07-15/16)

## Objetivo

Crear un skill global de Claude Code que delegue tareas a agentes externos vía
`quorum fleet run` (y a subagentes internos como fallback) con **prioridad
principal: ahorrar tokens de Claude**. La efectividad es secundaria: se aceptan
más fallos de primera pasada a cambio de que cada fallo cueste ~$0.

**Entregable**: `/home/gary/.claude/skills/fleet-delegate/` (SKILL.md +
`references/limits.md`). No es un flujo Quorum/SDC — es un wrapper de routing
sobre el runner non-lifecycle `quorum fleet run`.

## Fuentes de verdad

- `/home/gary/dev/quorum/docs/fleet-run-for-agents.md` — contrato del CLI +
  evaluación empírica de modelos free (2026-07-15).
- `/home/gary/dev/quorum/.agents/fleet/agents.yaml` — transportes: `agy`
  (activo, suscripción Gemini, one-shot `--print`, SIN edición agéntica),
  `opencode` (activo, $0, ÚNICO agéntico con tool-loop), `aider` (activo, $0,
  message-file + lista de archivos), `codex` y `claude` (inactivos).
- 7 agentes internos en `~/.claude/agents/`: arquitecto (opus),
  ejecutor-complejo (opus), ejecutor (sonnet), ejecutor-mecanico (haiku),
  especificador (sonnet), revisor (sonnet), revisor-profundo (opus).

## Decisiones de diseño (con razones)

1. **Eje de routing = necesidad de feedback, no dificultad** (idea del usuario,
   validada). One-shot autocontenido → externo; iterativo/exploratorio → interno.
   Matiz: opencode tiene su PROPIO tool-loop, así que "one-shot para el
   orquestador" puede ser multi-paso para el delegado, gratis.
2. **Rechazado: un agente por combinación modelo/esfuerzo** (~20 archivos).
   Razones: el tool Agent ya parametriza model/effort por invocación; un agente
   sin rol no aporta prompt de comportamiento; mantenimiento combinatorio.
   Diseño final: roles × parámetros. agy ya materializa el esfuerzo en sus
   claves canónicas (`gemini-3.5-flash-low|medium|high`, `pro-low|high`).
3. **Escalera de 5 peldaños** (el más barato capaz gana): opencode Tier A ($0)
   → aider ($0) → agy Flash → agy Pro/Claude 4.6 → subagente interno
   (haiku→sonnet→opus). Interno solo si: 429/saturación, necesita contexto de
   la conversación, o 2 intentos externos fallaron.
4. **El hilo principal solo hace 3 cosas**: clasificar, redactar prompt,
   verificar.

## Riesgos identificados y mitigaciones adoptadas

| # | Riesgo | Mitigación (en el skill) |
|---|---|---|
| R1 | Clasificar mal es asimétrico (delegar mal cuesta MÁS tokens que hacerlo directo) | M2: gate de 4 preguntas, conservador hacia interno |
| R2 | Verificación barata no ve bugs sutiles / falsos éxitos (exit 0 sin diff, tests ajustados) | M4: diff no vacío + scope + bandera roja en tests tocados; M5: verificación en 3 niveles |
| R3 | Deuda de calidad acumulativa e invisible | M6: snippet de convenciones por proyecto (`.agents/fleet-delegate-conventions.md`); M2.4: solo código de bajo valor arquitectónico |
| R4 | Free tier: 20 req/min compartido, 429 cuenta contra cuota diaria, lista decae | M3: reglas duras (1 run, no retry 429, timeout explícito); M7: enum vivo de `--schema`, nunca listas hardcodeadas |
| R5 | Serializar contexto al prompt cuesta tokens de Claude | M2.2: prompt debe caber en ~30 líneas o no se delega |
| R6 | Latencia bloquea el hilo | Regla: <1 min interno con haiku → no delegar |
| R7 | Sin sandbox (non-lifecycle): el diff es la única forense | M1: working tree limpio obligatorio pre-run; `git checkout .` tras TIMEOUT |

**Residual aceptado deliberadamente** (no intentar eliminar): bugs bajo
cobertura débil, estilo de modelos free, decaimiento de Tier A (sin
re-validación automática — descartada por costo de mantenimiento).

## Datos empíricos clave (foto 2026-07-15 — DECAE)

- Tier A opencode (6 modelos, 9/9 en suite oculta, claves con `-free` en vez de
  `:free`): nemotron-3-nano-omni-30b-a3b-reasoning (11s), laguna-xs-2.1 (12s),
  north-mini-code (14s), nemotron-3-ultra-550b-a55b (18s, 1M ctx),
  laguna-m.1 (19s), nemotron-nano-9b-v2 (72s, último recurso).
- Tier B (single-shot OK, agéntico FAIL — nunca en opencode/aider):
  nemotron-3-nano-30b-a3b (trunca escrituras), gpt-oss-20b (exit 0 sin actuar).
- Límites: 20 req/min cuenta completa; 1000/día esta cuenta (≥$10 lifetime);
  429 cuenta contra la cuota.
- aider quedó pinned-only el 2026-07-15 (auto-router removido); pass rates
  propios de aider pendientes de validación.

## Estado y pendientes

- ✅ Skill creado y registrado (aparece como `/fleet-delegate`).
- ⏳ Sin probar en una tarea real — primer uso recomendado: una tarea mecánica
  pequeña (docs/boilerplate) vía opencode Tier A, con dry-run primero.
- ⏳ Crear `.agents/fleet-delegate-conventions.md` (~10 líneas) en los proyectos
  donde se vaya a delegar (mitigación M6).
- ⏳ Validación aider-específica de los 6 modelos Tier A (pendiente upstream,
  en Quorum).
