# 22 — Medición del ahorro real: campos de coste en el ledger + agregador determinista

**Fecha:** 2026-08-02
**Origen:** auditoría de la sesión `hexcell-2` (2026-07-27→31) sobre el skill `/q-orchestrate` y el flujo Quorum. De las mejoras detectadas, tres se implementaron en el momento (nivel 1 del router, gate de flota como precondición computada, fases mecánicas externas por defecto). Ésta se DIFIRIÓ explícitamente por decisión del usuario: "no lo debemos hacer ahora, implementarlo después con más calma y detalle".
**Estado:** 🚧 diferida — propuesta viva, con gate de entrada (ver "Gate").

## Problema

Tras la v1.7 de `/q-orchestrate` no se puede responder con datos la única pregunta que justifica todo el esfuerzo de delegación externa: **¿cuántos tokens dejaron de ser de Anthropic?**

El orquestador ya escribe una línea por fase en `~/.claude/quorum-agent-ledger.jsonl`:

```json
{"ts":"...","phase":"<phase>","executor":"internal|fleet","gate_reason":"<why not fleet, or null>","agent":"...","model":"...","effort":"...","outcome":"...","retries":0,"tokens":<total-or-null>,"notes":"<short>"}
```

Lo que falta:

1. **No hay campo de pagador.** `model` dice *qué* corrió, no *quién lo pagó*. Un `gemini-3.6-flash-medium` por suscripción Google y un `sonnet/high` de Anthropic aparecen como dos strings equivalentes. Sin distinguirlos, sumar `tokens` mezcla peras con manzanas y el ahorro es inmedible.
2. **No hay clave de join con la telemetría de dispatch.** `quorum fleet stats` agrega los `result.json` terminales por celda/nivel/banda, pero esos registros y las filas del ledger no comparten identificador, así que no se puede cruzar "fase del orquestador" con "dispatch real".
3. **Las filas externas traen `tokens: null`.** Verificado: el envelope de `quorum fleet run` expone `{agent, cwd, exit_code, killed, model, output, output_parse_ok, quota_matched}` — **no devuelve uso de tokens**. Las tres filas `implement-fleet` medidas (2026-07-31, todas `outcome: fail`) tienen `tokens: null`.

### Línea base medida (para comparar después)

Proyecto `hexcell`, 94 filas de ledger (HEX-001..HEX-011), **2.52M tokens, 100% Anthropic**:

| Fase | Tokens | % |
|---|---:|---:|
| implement | 682k | 27.1% |
| blueprint | 563k | 22.4% |
| review | 351k | 13.9% |
| brief | 254k | 10.1% |
| analyze | 250k | 9.9% |
| verify | 159k | 6.3% |
| accept | 151k | 6.0% |
| memory | 108k | 4.3% |

Ésta es la foto *anterior* a las tres mejoras. Cualquier medición futura se compara contra ella.

## Restricción de diseño (la que hace que esta tarea sea delicada)

**Medir no puede costar lo que ahorra.** Hay que separar dos operaciones que se confunden:

- **Escribir** el dato: el orquestador ya emite la línea; añadirle 3 campos cuesta decenas de tokens por tarea. Ruido, aceptable.
- **Leer** el dato: si el orquestador carga filas del ledger en su contexto para sacar cuentas, se come el ahorro. En la propia auditoría de `hexcell-2` esto se resolvió con scripts deterministas (`stats.py`) precisamente para no pagar la lectura.

**Regla que debe quedar escrita en el skill:** la línea la ESCRIBE el orquestador; la suma la hace SIEMPRE un script o un comando. Nunca entran filas del ledger al contexto de la conversación. Precedente vigente: `quorum fleet stats` hace exactamente eso con los dispatches.

## Propuesta

### 1. Tres campos nuevos en la línea del ledger

| Campo | Valores | Para qué |
|---|---|---|
| `transport` | `agy_edit` \| `agy` \| `opencode` \| `aider` \| `codex` \| `internal` | Distinguir el camino, no solo el modelo |
| `cell` | `<agent>/<model>` | Identidad estable de celda; permite "esta celda falla siempre" |
| `usd_class` | `anthropic` \| `subscription` \| `free` | **El campo clave**: responde cuántos tokens salieron de Anthropic |

`usd_class` no se inventa por fila: se deriva de `quota_class` del transporte en `.agents/fleet/agents.yaml` (`api` / `subscription`) más el caso `internal` → `anthropic`. Conviene que el skill lo lea de ahí en lugar de hardcodearlo, misma razón por la que `core.Route` no hardcodea modelos.

### 2. Clave de join con dispatch

Cuando `executor: "fleet"`, añadir `dispatch_id` (el que ya genera `quorum fleet bundle` / `dispatch`). Con eso una fila del ledger se cruza con el `result.json` que `quorum fleet stats` ya sabe leer, sin duplicar telemetría.

### 3. Agregador

Dos opciones, con tradeoff real:

| Opción | A favor | En contra |
|---|---|---|
| **(a) Script en `~/.claude/skills/q-orchestrate/scripts/`** | El ledger vive fuera del repo (global, multiproyecto) → no es asunto de Quorum; iteración barata | Queda fuera de `go test ./...`; sin schema ni validación |
| (b) Extender el CLI (`quorum stats phases` o `fleet stats --ledger`) | Testeable, versionado, coherente con `fleet stats` | Mete un artefacto global de `~/.claude/` dentro de un binario de proyecto; roza el límite de responsabilidades |

**Recomendación: empezar por (a)**, y promover a (b) solo si el reporte demuestra ser útil de forma recurrente. No al revés.

### 4. Forma del reporte

Por proyecto × fase: total de tokens, desglose por `usd_class`, y el número que importa — **% de tokens Anthropic evitados** contra la línea base de arriba. Más una tabla por celda con `outcome` para detectar celdas que fallan sistemáticamente.

## Qué NO cubre

- **Coste real en USD.** Se cuentan tokens y clase de pagador, no precios. Convertir a dinero exige tabla de precios por modelo, que envejece; queda fuera.
- **Uso de tokens de los proveedores externos.** Verificado que el envelope de `fleet run` no lo devuelve. Las filas externas seguirán con `tokens: null` salvo que se instrumente el adapter — trabajo aparte, probablemente no rentable.
- **Auto-captura a memoria curada.** Prohibido por la constitución (Regla: la memoria es curada, `q-memory` es la única vía de ingesta). Este reporte es telemetría operativa, no conocimiento.
- **Backfill de filas viejas.** No se hace: se marca una fecha de corte y se compara contra la línea base ya medida arriba.

## Coherencia con las decisiones de la serie

- Mismo patrón que `fleet stats` (tarea 10, gate G1): agregación determinista fuera del contexto del modelo.
- Mismo principio que ADR-A: la política (aquí, la clase de cuota) es DATO en `agents.yaml`, no constante en código ni en el skill.
- No añade slot de artefacto del ciclo `00`→`07` (el manifiesto los rechaza): el ledger es un archivo global de orquestación, no un artefacto de tarea.

## Gate (condición de entrada)

**No implementar hasta que las tres mejoras de 2026-07-31/08-01 hayan generado filas reales** — nivel 1 del router reconstruido, gate de flota como precondición computada, fases mecánicas externas. Medir antes significaría medir la forma vieja del flujo. Referencia práctica: ~10 tareas completas con la v1.7 del skill.

## Esfuerzo estimado

Pequeño: los 3 campos son edición de una línea de schema en el skill + el `dispatch_id`; el agregador es un script de ~80 líneas sobre un JSONL. El coste real no es escribirlo, es **esperar a tener muestra**.

## Decisiones abiertas

1. ¿(a) script o (b) CLI? — recomendación (a) primero; decisión del usuario en el brief.
2. ¿`tokens` en filas externas queda `null` o se intenta best-effort? — recomendación `null` explícito, y que el reporte lo declare como "no medido" en vez de asumir 0 (asumir 0 inflaría el ahorro).
3. ¿El reporte se expone en el dashboard de `quorum serve`? — solo si se elige la opción (b).

## Seguimiento

Al implementarse, actualizar `00-indice.md` y archivar según la convención de la serie (`git mv` a `docs/archive/fleet/`).
