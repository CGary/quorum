# 🛟 Propuesta Técnica: Recuperación Automática en Runtime tras Fallo

**Estado:** Diferida — implementar cuando se cumplan las precondiciones (ver §4).
**Contexto:** Evolución de Quorum v1.2+.
**Origen:** Subconjunto refinado de la propuesta original "Recuperación de Errores y Self-Healing", filtrado por análisis de factibilidad contra el código real.
**Veredicto:** El 80% de la propuesta original duplicaba artefactos y skills existentes. Lo único que queda diferido (tras implementar `FAIL-001`) es la automatización del bucle de retry, que requiere un dispatcher activo.

---

## 1. El Problema Real (acotado)

`FAIL-001` introduce dos mejoras que cierran el bucle de aprendizaje **manual**:
- Clasificación de errores (`logic|dependency|environment|flaky|unknown`) en `05-validation.json`.
- Consulta de tareas fallidas relacionadas durante el blueprint.

Pero ambas requieren **invocación humana o de skill explícita**. El bucle de retry automatizado, donde un fallo dispara una corrección dirigida sin intervención humana, queda fuera de alcance hasta que exista un dispatcher activo.

Modos de fallo aún sin mitigar:
1. Una tarea que falla por `error_category ∈ {environment, flaky}` debería reintentarse automáticamente sin generar trabajo de blueprint ni costo de LLM.
2. Una tarea que falla por `error_category ∈ {logic, dependency}` debería disparar un re-blueprint con el contexto del fallo.

Hoy, ambas requieren intervención humana para reiniciar el flujo.

---

## 2. Componentes a Implementar (a futuro)

### 2.1 Retry automático para errores transitorios

**Qué hace:** cuando `05-validation.json.error_category ∈ {environment, flaky}` y `len(07-trace.json.attempts)` está bajo `02-contract.yaml.retry_policy.max_attempts`, el dispatcher reintenta sin re-blueprint, sin re-implementación, sin costo de LLM. Solo re-ejecuta `verify.commands`.

**Diseño técnico:**

| Aspecto | Decisión propuesta |
|---|---|
| Trigger | Lectura de `05-validation.json` post-`q-verify`. |
| Condiciones acumulativas | `overall_result == failed`, `error_category ∈ {environment, flaky}`, `attempts.count < retry_policy.max_attempts`. |
| Backoff | Lineal (5s, 10s, 20s) por intento. Configurable en `routing.yaml`. |
| Costo | Cero LLM. Solo re-ejecución de tests/lint. |
| Telemetría | Cada retry añade un `attempts[]` con `phase: verify` y `notes: "auto-retry: {category}"`. |

**Cambios requeridos:**
- Modificación en `task_manager.run_task` (que primero debe existir como bucle real).
- Campo opcional `auto_retry_on: [string]` en `routing.yaml` (lista de categorías que disparan retry).

### 2.2 Re-blueprint automático con contexto del fallo

**Qué hace:** cuando `05-validation.json.error_category ∈ {logic, dependency}` y los retries automáticos no aplican, el dispatcher invoca `q-blueprint --mode recover` pasándole los artefactos de fallo como contexto adicional. El Cartógrafo regenera `01-blueprint.yaml` y `02-contract.yaml` incorporando el aprendizaje.

**Diseño técnico:**

| Aspecto | Decisión propuesta |
|---|---|
| Trigger | `error_category ∈ {logic, dependency}` y `attempts.count < retry_policy.max_attempts`. |
| Inputs adicionales al blueprint | Excerpts de `05-validation.json.commands[].output_excerpt`, lista de `06-review.json.fix_tasks`, notas de `04-implementation-log.yaml`. |
| Salida | Nuevo `01-blueprint.yaml` y `02-contract.yaml` que pueden añadir entradas a `forbid.behaviors` derivadas del fallo. |
| Acotación | Máximo 1 re-blueprint automático por tarea. Tras eso, escalación humana. |

**Cambios requeridos:**
- Modo `--mode recover` en `q-blueprint/SKILL.md`.
- Lógica de invocación en `task_manager.run_task`.
- **Coordinación con el protocolo de renegociación** (`2-ideas/protocolo-renegociacion-contrato.md`) — son flujos hermanos que reentran al blueprint por triggers distintos (fallo de verify vs. archivos faltantes en contrato). Necesitan política unificada para evitar bucles cruzados.

---

## 3. Lo que NO se Implementará (rechazado del original)

Ya documentado en `quorum.md` §🛟 Failure Handling / "What is NOT needed":

| Componente rechazado | Razón |
|---|---|
| Artefacto `08-post-mortem.json` | Duplica `05-validation.json`, `06-review.json` y `07-trace.json`. Slot innecesario. |
| Agente "Diagnostic-L0" dedicado | `q-review` y `q-memory` ya cubren las responsabilidades. |
| "Negative constraints" como mecanismo nuevo | `02-contract.yaml.forbid.behaviors` y `memory/lessons/anti_patterns` ya existen. |
| "Promoción a Memoria" como flujo nuevo | `q-memory` ya lo hace; basta documentar que también opera sobre `failed/`. |
| Auto-perdón de fallos de test | Viola Regla #4 (Validation is Finality). |

---

## 4. Precondiciones para Desbloquear

**No implementar §2 hasta que TODAS estas condiciones se cumplan:**

| # | Precondición | Justificación |
|---|---|---|
| 1 | Plan `FAIL-001` está implementado y en uso. | `error_category` es prerrequisito de §2.1 y §2.2. |
| 2 | `task_manager.run_task` existe como bucle real, no stub. | Sin dispatcher consumidor, la automatización es código muerto. |
| 3 | ≥10 tareas con `error_category` registrado en `05-validation.json`. | Sin datos, no se puede calibrar qué categorías son seguras para auto-retry vs. requieren re-blueprint. |
| 4 | Coordinación documentada con el protocolo de renegociación de contrato. | §2.2 y `protocolo-renegociacion-contrato.md` son flujos hermanos; uno reentra al blueprint por fallo, el otro por archivos faltantes. Necesitan política unificada. |
| 5 | Ratio observado de fallos `environment|flaky` ≥ 20% del total. | Si los fallos transitorios son raros, §2.1 es sobre-ingeniería. |

---

## 5. Trazabilidad de la Decisión

- **Propuesta original:** post-mortem dedicado + agente diagnóstico + negative-constraint loop + promoción a memoria + retry automático.
- **Análisis de factibilidad:** 80% duplicaba artefactos/skills existentes. 20% (clasificación + consulta de fallos) se extrajo como acción inmediata. El bucle de retry quedó diferido por dependencia del dispatcher.
- **Acción inmediata:** plan `3-exec/FAIL-001-clasificacion-error-y-fallos-relacionados.md` — campo `error_category` + función de consulta sobre `failed/`.
- **Acción diferida:** este documento — esperar dispatcher y datos.
- **Acción rechazada:** §3 — documentada en `quorum.md` para evitar reintroducción.

---

## 6. Próximos Pasos para el Agente Revisor (cuando se desbloquee)

1. Validar que las 5 precondiciones de §4 se cumplen con datos verificables.
2. Implementar §2.1 antes que §2.2 (el retry sin LLM es más barato y más seguro de validar primero).
3. Coordinar §2.2 con `protocolo-renegociacion-contrato.md` — definir un único modo de re-blueprint con dos triggers (fallo de verify, archivos faltantes en contrato).
4. Escribir ADR en `docs/adr/` por cada cambio de política de auto-retry, citando la métrica que lo motivó.
5. Confirmar que la implementación NO reintroduce ninguno de los componentes rechazados en §3.
