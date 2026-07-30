# 18 — Actualización de flota: baja de codex (créditos agotados) + modelos agy Gemini 3.6

**Fecha:** 2026-07-30
**Origen:** revisión en vivo de la flota (sesión 2026-07-30): el humano reporta créditos de codex agotados; `agy models` muestra modelos nuevos no declarados en `agents.yaml`.
**Uso:** insumo de UNA tarea Quorum (`/q-brief` cierra las "Decisiones abiertas"; `/q-blueprint` la estrategia).
**Tipo de cambio:** solo datos de política/transporte (`agents.yaml`, `config.yaml`, `routing.yaml`) + contract test de agy. Cero cambios en `internal/core` (ADR 0010: los modelos nunca se hardcodean en código).

---

## 1. Evidencia verificada (2026-07-30)

### 1.1 codex sin créditos

- El humano reporta que los créditos free-tier de ChatGPT (los que justificaron la
  reactivación del 2026-07-22, `agents.yaml` transport `codex`, comentario "Reactivated
  2026-07-22 ... Credits are LIMITED and non-renewing") están **agotados**.
- La firma de fallo ya está catalogada en `agents.yaml`: `You've hit your usage limit`
  (verificada en commit d01c650).
- Hoy `codex` sigue `active: true` y aparece en
  `config.yaml policies.fleet_transport_order` (`[agy, opencode, aider, codex, claude]`),
  por lo que `quorum fleet route` puede seguir enumerándolo como candidato que fallará
  siempre.

### 1.2 agy cambió su catálogo de modelos

Salida real de `agy models` (2026-07-30):

```
gemini-3.6-flash-high
gemini-3.6-flash-medium
gemini-3.6-flash-low
gemini-3.5-flash-high
gemini-3.5-flash-medium
gemini-3.5-flash-low
gemini-3.1-pro-high
gemini-3.1-pro-low
claude-sonnet-4-6
claude-opus-4-6-thinking
gpt-oss-120b-medium
```

Diferencias contra el mapa `models` del transport `agy` en `agents.yaml`:

| Cambio | Detalle |
|---|---|
| **Agregado** | `gemini-3.6-flash` en 3 efforts (low/medium/high) — no existe en `agents.yaml` ni en el enum de `quorum fleet run --agent agy --schema` |
| **Sin cambios de effort** | `gemini-3.5-flash` sigue low/medium/high; `gemini-3.1-pro` sigue solo low/high; `gpt-oss-120b` sigue single-tier (medium) |
| **Formato de salida** | `agy models` ahora emite slugs (`gemini-3.5-flash-low`), no display names ("Gemini 3.5 Flash (Low)") como documentó Fase 0a §3 |
| **Sonnet sin sufijo thinking** | el slug es `claude-sonnet-4-6` (opus sí conserva `-thinking`); el `model_arg` actual es "Claude Sonnet 4.6 (Thinking)" — verificar si sigue siendo válido |

Compatibilidad verificada en vivo:

- `agy --model "Gemini 3.5 Flash (Low)" --print "..."` → **OK**. Los display names
  legacy siguen aceptados; los `model_arg` actuales de `agents.yaml` **no están rotos**.
- Un `--model` inválido devuelve error con la lista de display names ("Gemini 3.6 Flash
  (High)", ...), y el mensaje muestra un flag `--effort ""` separado: agy ahora expone
  `--effort (low|medium|high)` como flag independiente del modelo (ver Decisión abierta D3).

---

## 2. Cambios propuestos

### 2.1 Baja de codex

1. Desactivar el transport: la vía preferente es el kill-switch diseñado para esto —
   `quorum fleet disable codex --reason "ChatGPT free-tier credits exhausted 2026-07-30"`
   (escribe `.ai/fleet-control.json`, reversible con `quorum fleet enable`) — o bien
   `active: false` en `agents.yaml` si se decide que la baja es permanente (ver D1).
2. Si la baja es permanente: retirar `codex` de `config.yaml` (`levels[1]`, `levels[2]` y `policies.fleet_transport_order`) y de `agents.yaml` (`transports.codex`), dejando el bloque como histórico. **Corrección:** codex nunca estuvo referenciado en `routing.yaml`.
3. Fuera del repo (no es parte de la tarea, se anota para trazabilidad): actualizar la
   mención "codex free-tier" en la descripción del skill `q-orchestrate`
   (`~/.claude/skills/q-orchestrate/`) y el snapshot
   `~/.claude/skills/fleet-delegate/references/limits.md`.

### 2.2 Alta de Gemini 3.6 Flash en agy

Agregar al mapa `models` del transport `agy` en `agents.yaml` (mismo patrón que 3.5):

```yaml
google/gemini-3.6-flash-low:
  provider: google
  model_arg: <ver D2>
google/gemini-3.6-flash-medium:
  provider: google
  model_arg: <ver D2>
google/gemini-3.6-flash-high:
  provider: google
  model_arg: <ver D2>
```

- Actualizar el comentario del mapa que cita "Exact strings from `agy models` (Fase 0a §3)":
  el formato de esa salida cambió (slugs).
- Evaluar si `config.yaml.levels` debe promover 3.6-flash como celda primaria del nivel 0
  (hoy `google/gemini-3.5-flash-low`) o si 3.6 entra sin rutear hasta tener telemetría
  (lema de la serie: telemetría antes que automatización → ver D4).
- Verificar y corregir si hace falta el `model_arg` de `anthropic/claude-sonnet-4-6`
  (¿sigue aceptando "Claude Sonnet 4.6 (Thinking)"?).

### 2.3 Contract test

Extender `.agents/fleet/contract_tests/agy.yaml` para cubrir al menos una celda 3.6
(patrón barato nivel 1: validación estructural de argv, sin gastar cuota).

---

## 3. Decisiones cerradas (D1-D4)

- **D1 — Baja de codex:** **Kill-switch temporal**. Se mantiene el transport intacto en los archivos de política (`active: true`). La baja se opera mediante `quorum fleet disable codex`, permitiendo que el nivel 1 caiga limpiamente al fallback de `agy` sin mutar la configuración ratificada.
- **D2 — Formato de `model_arg` para 3.6:** **Display name** ("Gemini 3.6 Flash (Low)"). El backend de agy sigue aceptando este formato (comprobado en vivo). Mantenerlo evita desalinear las nuevas celdas de las existentes 3.5.
- **D3 — Flag `--effort` separado:** **Diferido**. No se migra el `argv_template` general de `agy` en esta tarea. El effort se sigue pasando embebido en el string del modelo para no afectar colateralmente a los demás.
- **D4 — Ruteo de 3.6-flash:** **Promovido a nivel 0 primario**. Se actualiza `levels[0].primary` a `google/gemini-3.6-flash-low`, reemplazando la única referencia a 3.5 en los niveles. Las celdas 3.5 quedan expuestas pero sin rutear.

### Rollback (Nuevas celdas 3.6)
Si los modelos 3.6 resultan inestables o rompen contratos antes de acumular telemetría suficiente, se puede usar `quorum fleet disable google/gemini-3.6-flash-low` (y las demás variantes) para sacarlos de rotación inmediatamente sin necesidad de revertir los cambios en `agents.yaml` o `config.yaml`.

---

## 4. Criterios de aceptación

1. `quorum fleet run --agent agy --schema` incluye `google/gemini-3.6-flash-{low,medium,high}`
   en el enum de `--model`.
2. Una invocación real `quorum fleet run --agent agy --model google/gemini-3.6-flash-low ...`
   con prompt trivial devuelve `ok:true` + `exit_code:0` (una sola sonda, cuota subscription).
3. `quorum fleet route` (o `quorum fleet status`, según D1) ya no ofrece celdas codex como
   candidato ejecutable.
4. `go test ./...` verde (golden master + contract tests estructurales).
5. Ningún cambio en `internal/core` (el diff toca solo `.agents/` y, según D1, `config.yaml`).
