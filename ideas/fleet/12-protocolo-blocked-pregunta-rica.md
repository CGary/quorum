# 12 — Protocolo BLOCKED con pregunta de contexto rico

> **PARCIALMENTE IMPLEMENTADO (2026-07-23).** Nivel 1 hecho: convención de pregunta rica en prosa en .agents/skills/q-dispatch/SKILL.md; pendiente: schema estructurado (opciones/consecuencias/recomendación) como payload validado.

**Tipo:** extensión de señal existente en core + convención de protocolo.
**Depende de:** 03 (clase `blocked` en la taxonomía). Complementa 05 (el bundle pide el formato) y 06 (el wrapper lo detecta).
**Riesgo sugerido:** medium.

## Contexto

Decisión tomada: ante ambigüedad, ni loops antieconómicos ni heurísticas — se pregunta al humano. Pero el problema detectado (y sufrido >90% de las veces): los agentes preguntan con tan poco contexto que el humano tiene que pedir más antes de poder decidir. La solución es estructural, no de buena voluntad: **la pregunta tiene schema obligatorio y una pregunta incompleta no es una pregunta válida** — compliance by construction aplicado a las preguntas, el mismo movimiento que la tarea 08 hace con el `04`. El repo ya tiene la primitiva: `ParseBlockedSignal` (`internal/core/blocked_signal.go`) convierte el BLOCKED de un skill en dato estructurado; esto la extiende.

## Objetivo

1. Schema de pregunta rica como payload del BLOCKED.
2. Extensión de `ParseBlockedSignal` (o función hermana) que valide el payload.
3. Convención de emisión para delegados (vía protocolo del bundle) y para skills.
4. Registro en trace: `blocked_question` al emitir, `blocked_answer` al responder.

## Schema de la pregunta (campos obligatorios salvo marcados)

```json
{
  "question": "una frase, decidible",
  "attempted": ["qué se intentó/analizó antes de preguntar"],
  "discarded": [{"option": "...", "why": "..."}],
  "evidence": [{"file": "path", "lines": "a-b", "excerpt": "...", "relevance": "..."}],
  "options": [{"label": "...", "consequence": "qué pasa si se elige, costo/beneficio"}],
  "recommendation": "cuál y por qué (opcional pero esperada)",
  "open_option": true
}
```

Reglas: mínimo 2 `options`, cada una CON consecuencia (opción sin consecuencia = inválida); `open_option` siempre presente — el humano puede responder algo fuera del menú; `evidence` con al menos una entrada concreta. Validación dura: payload incompleto ⇒ no es `blocked`, es `attempt_failed` con razón `malformed_question` (incentivo correcto: preguntar mal cuesta un attempt; preguntar bien es gratis).

## Flujo

1. **Delegado headless** (no puede pausar): emite el bloque BLOCKED en su salida y termina. El wrapper (06) lo detecta, valida el schema, clasifica `blocked` — no consume attempts ni reroutes — y parquea: worktree reseteado según invariantes, evento `blocked_question` en trace, tarea marcada esperando humano.
2. **Canal v1: CLI.** El skill (tarea 14) u orquestador formatea la pregunta en español, presenta opciones + abierta, y termina el turno con `ESPERANDO RESPUESTA DEL USUARIO...` según el protocolo de skills.
3. **Respuesta** → evento `blocked_answer` (decisión + quién + cuándo) → el humano relanza el dispatch con la decisión inyectada al bundle (campo `human_decisions[]` del manifest).

## La línea de cuándo preguntar (va al protocolo del bundle y al ADR-B)

Ambigüedad **dentro** del contrato y reversible → decidir, registrar en notas (el humano lo ve en `q-review`). Ambigüedad **sobre** el contrato, irreversible, o que toca semántica del spec → BLOCKED. La frontera ya existe: es el contrato.

## Flywheel (sin automatización, v1)

Las `blocked_question/answer` quedan en traces. Revisión humana periódica: pregunta recurrente = gap del template de spec o de política → se codifica vía `/q-memory` o mejora de template. Detector automático de recurrencia: horizonte (16), gated por telemetría.

## Criterios de aceptación

- [ ] Parser/validador con tests: payload completo / sin consecuencias / sin evidencia / sin open_option / JSON roto (cada uno con su clasificación correcta).
- [ ] Integración con dispatch: fake binario que emite BLOCKED válido produce clase `blocked` (cero consumo de attempts/reroutes) y el inválido produce `attempt_failed: malformed_question`.
- [ ] Eventos `blocked_question`/`blocked_answer` en trace con el formato de la tarea 03.
- [ ] Plantilla del protocolo del bundle (05) actualizada con el formato de emisión y la línea de cuándo preguntar.
- [ ] `go test ./...` verde.

## Decisiones abiertas para el brief

- ¿`blocked` expira? (tarea parqueada días: ¿status la destaca? Propuesta: `q-status` la muestra como bloqueada-en-humano; sin TTL).
- ¿La respuesta humana puede modificar el contrato (renegociación)? Propuesta: NO en v1 — si la decisión exige tocar el contrato, el camino es `task back` humano + re-blueprint (ADR 0002 difirió renegociación; respetarlo).
- Compatibilidad hacia atrás del BLOCKED simple de skills actuales (¿los skills existentes migran al schema rico o conviven ambos formatos?).
