# Plan de fases — dispatcher multi-LLM (capa de ejecución)

**Fecha:** 2026-07-07
**Estado:** Aprobado (estructura). Las decisiones de cada fase se toman EN esa fase, no antes.
**Insumos:** `0-sistema-...-llms.md` (v2) · `*.analysis-v2.md` · `ideas/fleet/00-indice.md` (DAG y decisiones cerradas 2026-06-12) · `ideas/fleet/17-adapter-aider.md` · `ideas/aider.md`
**Autoridad:** este documento agrupa en fases el DAG de `fleet/00-indice.md`; si divergen, manda el índice. Cada tarea `NN-*.md` lleva sus propias "Decisiones abiertas para el brief" — ahí viven las decisiones, no aquí.

## Reglas del plan

1. Las decisiones llegan con recomendación escrita en el doc de su tarea; el humano **ratifica o veta, no diseña**.
2. Los gates pueden matar la serie (G0) o amputarla (G1 puede sacar a L0 de q-implement). Eso es diseño, no fracaso.
3. La constitución no se toca hasta F9, y solo con evidencia de G0/G1 (ADR-D, enmiendas E1-E3).
4. `ideas/9-enrutamiento-runtime-y-tuning.md` y `ideas/10-recuperacion-runtime-tras-fallo.md` quedan supersedidos por esta serie (referencia histórica).

## Las fases

| Fase | Construye (docs fleet) | Decisiones humanas EN esa fase | Gate |
|---|---|---|---|
| **F1 Prueba manual** | 01 — cero código: delegar a mano 2-3 tareas + mini-inventario headless de aider (flags, cwd, auto-commits, exit codes) | qué tareas usar; si agy sigue o queda fuera | **G0**: si delegar a mano no funciona, se archiva TODO |
| **F2 ADRs fundacionales** | 02 (frontera transporte/política), 03 (attempt/reroute/BLOCKED) | ~7, casi todas ratificaciones; la de diseño real: dónde vive el join modelo→agente. Incluye: ¿aider tercer transporte? | ADR-A y ADR-B aceptados |
| **F3 Piezas puras** | 04 (complexity-score), 05 (bundler), 09 (contract-checker) | ~5 locales (banda vs score, ubicación bundle, formato NOTES, semántica touch) | — |
| **F4 Motor de despacho** | 06 (dispatch + adapter codex), 07 (adapter agy + contract tests), 08 (fabricador 04), **17 (adapter aider)** | ~5 + las 6 del brief de 17 (backend/provider_trust, tope USD, API keys, kill granularity…) | — |
| **F5 Medición** | 10 — Fase 0 instrumentada | ANTES de medir: ratificar umbrales 50%/70%; origen de la muestra; ¿aider entra en la muestra? Las grandes (¿sirve L0?, bandas S/M/L) las responden los datos | **G1**: define rol de L0; el router (13) no arranca sin esto |
| **F6 Control humano** | 11 (kill-switch), 12 (protocolo BLOCKED) | ~4 (granularidad kill, expiración blocked) | — |
| **F7 Router y skill** | 13 (fleet route), 14 (/q-dispatch) | ~4 (effort en nombre de modelo, confirmación por riesgo). Aquí aterriza el routing por costo esperado (D19/Fase 4 del v2) cuando la telemetría exista | Requiere G1 |
| **F8 Visor** | 15 (quorum serve fleet status) | ~3 triviales | — |
| **F9 Constitución** | ADR-D: enmiendas E1-E3 + destino de q-orchestrate | E1 (Regla 7 con telemetría), E2 (autoridad de encadenamiento), E3 (como Regla **#11**, nunca pisar la #10), destino de q-orchestrate (migrar/absorber en q-dispatch/retirar) | Solo con evidencia G0/G1 |

## Andamiaje fuera del perímetro (ya materializado, sin ADR)

- **q-orchestrate** (global, `~/.claude/skills/`, v1.2): automatización personal del "humano o subagente" que el corpus permite en F1-F5 (decisión D40 del v2). Su destino se decide en F9.
- **Catálogo de roles** (`~/.claude/agents/`, 2026-07-07): 7 subagentes con modelo+esfuerzo fijados — ejecutor-mecanico (haiku), especificador (sonnet/medium), ejecutor (sonnet/medium), ejecutor-complejo (opus/high), revisor (sonnet/high, solo lectura), revisor-profundo (opus/xhigh, solo lectura), arquitecto (opus/xhigh). q-orchestrate rutea por rol; los precios se reajustan en los archivos de rol, nunca en el skill. Diseño: roles-ahora-celdas-cuando-duelan (se descartó la matriz modelo×esfuerzo).

## Próxima acción

**[Obligatorio para avanzar] F1**: elegir 2-3 tareas pequeñas reales y delegarlas a mano según `fleet/01-fase-0a-validacion-manual.md`, más el mini-inventario de aider. Cero código. Sus resultados (G0) alimentan todo lo demás.
