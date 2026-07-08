# 16 — Horizonte: diferidos con gate explícito (NO implementar sin su condición)

**Tipo:** documento de referencia, no es una tarea. Cada ítem se convierte en doc de tarea propio SOLO cuando su gate se cumple. Esto encarna el lema "telemetría antes que automatización" y evita que los diferidos se cuelen de contrabando en briefs futuros.

| # | Diferido | Gate para activarlo | Notas |
|---|---|---|---|
| 1 | **ADR-D: enmiendas constitucionales E1–E3** (regla 7 con telemetría; autoridad dispatcher; "telemetría antes que automatización" como regla **#11** — la #10 vigente es soberanía de datos y NO se pisa ni renumera) | Que exista la necesidad real de encadenar fases automáticamente (hoy la delegación es por fase, invocada por humano: no viola la Regla #9 y no necesita enmienda) | Cada enmienda con: métrica que mejora, invariantes de no-regresión, mecanismo de reversión |
| 2 | **Dispatcher encadenador** (`quorum task run` o equivalente) | ADR-D aceptado + telemetría de ≥N tareas delegadas sin incidentes de contrato | Existe test que hoy lo prohíbe (`task_manager_test.go:735`); cambiarlo es parte del ADR, no un side-effect |
| 3 | **`data_classification` ⊥ `risk` + `provider_trust` + data-gate en bundler** | Que la flota incorpore proveedores `external_low` (hoy codex/agy son external_standard y el gate no discrimina nada) | Exige cambio additive en `spec.schema.json` (es `additionalProperties: false`), template de spec, `q-brief`, herencia en `q-decompose`. El bundler (05) ya deja el punto de extensión |
| 4 | **Formateador L0 de notas del `04`** | Telemetría de Fase 0/operación: las notas crudas degradan `q-review` de forma medible | Presupuestado y logueado como dispatch propio — nunca una llamada LLM escondida en el componente "determinista" (tarea 08) |
| 5 | **Responder BLOCKED desde el dashboard** | Dashboard (15) estable + protocolo BLOCKED (12) rodado por CLI sin cambios durante varias tareas | La respuesta web escribe el mismo evento `blocked_answer` que el CLI: una sola semántica |
| 6 | **Convergencia `quorum serve` + visor q-report** | Idea q-report retomada | Mismo binario, ruta `/reports`; el dashboard de flota no se rediseña |
| 7 | **Routing por costo esperado** (intentos esperados × costo + overhead de diagnóstico, por modelo×fase×banda) | Historia suficiente en traces (≥30-50 tareas delegadas) + comparación A/B contra tier fijo diseñada | La política deja de ser estática porque la telemetría existe — no antes |
| 8 | **Escalación propuesta** (modelo falla verify N veces ⇒ el sistema PROPONE subir de nivel, humano confirma) | Telemetría de fallos por modelo×banda + ADR propio | Coherente con ADR 0001/0002: propuesta, jamás acción autónoma |
| 9 | **Detector de preguntas BLOCKED recurrentes** | ≥10-15 `blocked_question` acumuladas en traces | v1 del flywheel es revisión humana periódica (tarea 12); automatizar el detector solo si el volumen lo justifica |
| 10 | **Presupuestos por ventana / contadores de requests** | Que el kill-switch manual + reactivo (11) demuestre ser insuficiente en la práctica (ej.: 429s frecuentes por descuido humano) | Fue deliberadamente eliminado de la v2: no resucitar sin evidencia de dolor real |
| 11 | **Renegociación de contrato desde una respuesta BLOCKED** | ADR propio que reemplace el diferimiento de ADR 0002 | v1: decisión que exige tocar contrato ⇒ `task back` humano + re-blueprint |
| 12 | **Transporte opencode/API** (minimax, deepseek, qwen) | API keys disponibles + decisión de política sobre proveedores de bajo costo (reactivaría el ítem 3) | El slot existe en el diseño de `agents.yaml` (`quota_class: api`); solo falta la realidad. Nota: distinguir este item (transporte API directo a un modelo nombrado) de aider (editor-CLI con backend configurable, ver 17-adapter-aider.md); si el backend de aider fuera external_low, se activa la condición del item 3 (provider_trust + data-gate). |

## Regla de uso

Cuando un gate se cumple: escribir el doc de tarea (formato de esta serie), referenciar la evidencia del gate (traces, métricas, decisión humana), y pasar por `/q-brief` como cualquier otra. Si alguien propone implementar un ítem sin su gate, la respuesta está escrita acá y es no.
