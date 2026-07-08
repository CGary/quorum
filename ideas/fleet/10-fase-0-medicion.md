# 10 — Fase 0: medición instrumentada de calidad de delegación (gate G1)

**Tipo:** experimento instrumentado + reporte. Poco código nuevo (scripts de tabulación como mucho).
**Depende de:** 05, 06, 07 (o 06 solo, si agy se difirió), 08, 09.
**Gate que produce:** G1 — decide el rol de la celda barata y calibra las bandas de complejidad. La tarea 13 (router) NO inicia sin G1 resuelto.
**Riesgo sugerido:** low (no toca core; produce datos y una decisión).

## Contexto

La pregunta existencial de la serie: ¿delegar a modelos baratos sale más barato, o *barato + reintentos + diagnóstico frontier* pierde contra *mid directo*? La tesis no se defiende, se mide. Gracias al fabricador del 04 (tarea 08), la métrica quedó limpia: el éxito de delegación es **pasar `q-verify`** (regla #4 elevada a métrica única), sin ruido de compliance.

## Objetivo

Sobre **5–10 hijas reales representativas** (mezcla S/M de complejidad, riesgo low/medium), correr dos celdas con el pipeline completo (bundle → dispatch → 04 → verify):

- **Celda barata:** Gemini 3.5 Flash Low o GPT-OSS 120B vía agy (elegir una con el dato de Fase 0a; si agy se difirió, la celda barata no existe y esta tarea lo registra como resultado).
- **Celda mid:** `gpt-5-medium` vía codex (alternativa: Gemini 3.1 Pro Low).

## Métricas

| Métrica | Qué responde |
|---|---|
| `pass@1` | % que pasa `q-verify` sin intervención humana |
| `pass@≤1-reroute` | ídem admitiendo un reroute |
| Tasa de no-op por modelo | incapacidad silenciosa |
| % de attempts con violación de contrato (tarea 09) | respeto de boundaries |
| Minutos/tokens de diagnóstico del arquitecto por fallo | el costo oculto de la celda barata |
| Notas crudas que degradaron el review (juicio humano) | ¿hace falta el formateador L0? (alimenta tarea 16) |

Todo queda en `07-trace.json` de cada hija (events + attempts) — la tabulación lee traces, no estado paralelo.

## Criterio de decisión (umbral propuesto, el brief lo ratifica)

- Celda barata con `pass@1 < 50%` o `pass@≤1-reroute < 70%` ⇒ **L0 queda fuera de `q-implement`** (se conserva para tareas mecánicas: formateo, parsing) y los docs 13/16 se retocan antes de sus briefs.
- Celda mid por debajo de esos umbrales ⇒ alarma mayor: la delegación de implement se replantea entera (¿solo bandas S?) antes de seguir.
- Subproducto siempre: primeros cortes empíricos para `complexity.yaml` (tarea 04) — ¿qué tamaño de tarea empieza a degradar el pass rate?

## Criterios de aceptación

- [ ] ≥5 hijas reales por celda ejecutadas con el pipeline completo, trazas completas.
- [ ] Reporte en `ideas/fleet/resultados-fase-0.md` con la tabla de métricas y la **decisión G1 explícita**.
- [ ] Decisión y lecciones capturadas en memoria curada vía `/q-memory` (tipo `decision`/`lesson`).
- [ ] `complexity.yaml` recalibrado o ratificado con nota de evidencia.
- [ ] Índice de la serie (00) actualizado con el estado del gate.

## Decisiones abiertas para el brief

- Ratificar umbrales (50%/70%) o ajustarlos.
- Las 5–10 hijas: ¿backlog real de un proyecto consumidor, hijas de este repo, o mezcla? (representatividad vs riesgo de quemar tareas reales con modelos flojos — mitigado porque el fallo no mergea nada).
- ¿Se mide también una celda `claude haiku` si el transporte opcional entró en la tarea 02?
