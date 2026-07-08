# 15 — `quorum serve`: dashboard de status + kill-switch

**Tipo:** servidor HTTP local + UI mínima. Tarea final de la serie (alcance decidido: SOLO status + kill-switch).
**Depende de:** 11 (estado de control y su `--json`), 06 (traces con eventos de dispatch). El router (13) y el skill (14) NO son prerequisitos — el dashboard observa y controla, no orquesta.
**Riesgo sugerido:** medium.

## Contexto

Decisión tomada: el humano monitorea el usage de los proveedores manualmente y necesita una palanca cómoda para apagar/encender modelos sin ir a la terminal, más una vista de qué está pasando con la flota. Alcance deliberadamente mínimo: NI responder BLOCKED desde la web NI visor q-report (ambos en horizonte, tarea 16 — y existe una idea previa de `quorum serve` como visor de reportes con la que esto debe converger sin chocar: mismo binario, rutas distintas).

## Objetivo

`quorum serve start [--port N]` → dashboard local con dos capacidades:

1. **Status (read-only):** estado de cada agente/modelo (verde/rojo, razón, antigüedad, por quién), últimos dispatches (de los traces: tarea, fase, modelo, outcome, duración, tokens si los hay), tareas con dispatch en vuelo o parqueadas en `blocked`.
2. **Kill-switch (única escritura permitida):** toggle enable/disable por agente/modelo con campo razón obligatorio.

## Diseño propuesto

- **Go stdlib** (`net/http` + `html/template` + algo de JS vanilla o htmx embebido con `go:embed`). **Cero frameworks pesados, cero CGO** — la prueba ácida del core (`CGO_ENABLED=0`, `go test ./...`) sigue intacta.
- Solo `localhost` por defecto. Sin auth en v1 (es local); si algún día se expone, eso es otra tarea con su ADR.
- **La escritura pasa por el MISMO código que el CLI** (`fleet disable/enable` de la tarea 11): el dashboard no tiene vía propia de mutación — una sola superficie de escritura, una sola semántica, mismos eventos. Esto es lo que mantiene al dashboard fuera del negocio de "mutar estado en silencio" que los análisis marcaron como riesgo.
- Lectura de dispatches: de los `07-trace.json` bajo `.ai/tasks/` (la verdad), no de un estado paralelo. Si resulta lento con muchas tareas, cache en memoria con invalidación simple — nunca una segunda fuente persistida.
- Refresco: polling simple (intervalo configurable). Sin websockets en v1.

## No-objetivos (horizonte, tarea 16)

Responder preguntas BLOCKED desde la web; visor de reportes q-report (convergencia futura: mismo `quorum serve`, ruta `/reports`); overrides de routing (urgencia, preferencias de modelo) — eso exigiría su propio ADR de tabla de control; métricas agregadas/burn-rate.

## Criterios de aceptación

- [ ] `quorum serve start` levanta en localhost; la vista de status refleja `.ai/fleet-control.json` y traces reales.
- [ ] Toggle desde la UI produce el mismo efecto y registro que `quorum fleet disable/enable` (test de equivalencia).
- [ ] Razón obligatoria en el disable desde UI (validación server-side).
- [ ] Tarea parqueada en `blocked` visible con su antigüedad (la pregunta se responde por CLI, pero el dashboard la hace visible para que no se pudra olvidada).
- [ ] Binario sigue compilando `CGO_ENABLED=0`; sin dependencias nuevas pesadas (revisar `go.mod` en el diff).
- [ ] `go test ./...` verde (handlers testeados con `httptest`).

## Decisiones abiertas para el brief

- ¿`serve start` o solo `serve`? ¿Puerto default?
- ¿htmx embebido o template + meta-refresh puro? (propuesta: lo más simple que tolere el toggle sin recargar toda la página).
- ¿El dashboard muestra agregados simples de tokens reportados (suma por modelo desde traces)? Es barato y read-only — propuesta: sí, con disclaimer de que es parcial (`usage laxo` por decisión).
- Si esta tarea crece, `/q-decompose`: (a) server+status read-only, (b) kill-switch UI.
