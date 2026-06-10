# 🚦 Propuesta Técnica: Enrutamiento Dinámico en Runtime y Tuning de Umbrales

**Estado:** Diferida — implementar cuando se cumplan las precondiciones (ver §4).
**Contexto:** Evolución de Quorum v1.2+.
**Origen:** Subconjunto refinado de la propuesta original "Matriz de Enrutamiento y Riesgo Automática", filtrado por análisis de factibilidad contra el código real.
**Veredicto:** El scoring estático basado en señales se implementa **ahora** vía plan `RISK-001`. Lo que queda diferido es la aplicación en tiempo de ejecución y el tuning empírico de umbrales.

---

## 1. El Problema Real (acotado)

`RISK-001` introduce una función pura `assign_risk_level(blueprint, risk_policy)` que sugiere un nivel basado en señales binarias:
- ¿Toca un `sensitive_path`? → `high`.
- ¿Más de 5 archivos afectados o más de 2 símbolos exportados? → `medium`.
- Caso contrario → `low`.

Esa versión cubre la asignación **al momento del blueprint**. Quedan tres modos de fallo sin mitigar:

1. **No hay aplicación automática en runtime.** El nivel sugerido se registra en `07-trace.json` pero ningún consumidor (dispatcher) lo usa para enrutar el modelo.
2. **Umbrales no validados con datos.** Los valores `>5 archivos` y `>2 símbolos exportados` son guesses iniciales. Sin telemetría, no sabemos si producen falsos negativos (tareas riesgosas marcadas como `low`) o falsos positivos (tareas triviales marcadas como `medium`).
3. **Profundidad de dependencias no se considera.** Una tarea que toca 3 archivos cuya transitividad alcanza 50 archivos es de mayor riesgo real que una que toca 6 archivos aislados. Hoy se ignora.

---

## 2. Componentes a Implementar (a futuro)

### 2.1 Aplicación automática del nivel calculado en el dispatcher

**Qué hace:** cuando el dispatcher invoca el ejecutor, consulta el último `assign_risk_level()` registrado en `07-trace.json` y lo combina con `routing.yaml` para elegir el `executor_level` y aplicar `max_attempts`/`human_gate_required` automáticamente.

**Diseño técnico:**

| Aspecto | Decisión propuesta |
|---|---|
| Punto de aplicación | `task_manager.run_task` (cuando deje de ser stub). |
| Fuente de verdad | El nivel `risk` del `00-spec.yaml` mantiene autoridad humana; el calculado es un override opcional gobernado por `policies/routing.yaml.auto_apply: true|false` (default `false` hasta validar). |
| Resolución de conflictos | Si `00-spec.yaml.risk` y el calculado divergen, registrar el evento en `07-trace.json` con `divergence: {declared, calculated, reasons}` y aplicar el **más alto de los dos** (fail-safe). |
| Reversibilidad | El humano puede sobrescribir el nivel aplicado vía un campo nuevo `02-contract.yaml.execution.risk_override`. |

**Cambios requeridos:**
- Modificación en `task_manager.run_task` (que primero debe existir como bucle real).
- Campo opcional `auto_apply: bool` en `routing.yaml`.
- Campo opcional `risk_override: string` en `contract.schema.json`.

### 2.2 Tuning empírico de umbrales

**Qué hace:** los umbrales de `RISK-001` (`>5 affected_files`, `>2 exported_symbols`) son hipótesis. Esta fase los valida con datos:

| Acción | Métrica |
|---|---|
| Calcular ratio de divergencia entre `risk` declarado por humano y calculado por scorer | Si >30%, los umbrales están mal calibrados. |
| Cruzar `level: low` con tareas que terminaron en `failed/` | Si >10% de fallos vienen de tareas marcadas `low`, los umbrales son demasiado permisivos. |
| Cruzar `level: high` con tareas que pasaron en primer intento sin retries | Si >40% pasaron sin fricción, los umbrales son demasiado conservadores. |

**Cambios requeridos:**
- Script de análisis en `.agents/cli/commands/audit.py` (nuevo) que lea `07-trace.json` de `done/` y `failed/`.
- Reporte CSV o markdown con divergencias y propuestas de ajuste.
- ADR en `docs/adr/` documentando cada cambio de umbral con su evidencia.

### 2.3 Profundidad de dependencias como señal opcional

**Qué hace:** invoca `import_graph.py` desde `assign_risk_level()` con `max_hops=2` y suma una señal cuando la transitividad alcanza más de N archivos.

**Diseño técnico:**

| Aspecto | Decisión propuesta |
|---|---|
| Activación | Detrás de un flag `risk_policy.use_dependency_depth: bool` (default `false`). |
| Costo | Lectura de archivos del filesystem; sin LLM. Aceptable si `max_hops ≤ 2`. |
| Umbral inicial | Si `len(reachable) > 15` con `max_hops=2`, añadir señal `high_transitive_impact`. |
| Caché | Cachear el resultado en `07-trace.json` para evitar re-cómputo. |

**Por qué no se incluye en `RISK-001`:** ejecutar el retriever en cada blueprint añade tiempo y complejidad. Si los otros tres factores (sensitive_paths, file_count, exported_symbols) ya capturan >90% de los casos riesgosos, el ROI es marginal. Solo implementar si la fase 2.2 muestra falsos negativos atribuibles a transitividad oculta.

---

## 3. Lo que NO se Implementará (rechazado del original)

Estos componentes de la propuesta original quedan formalmente descartados. Ya están documentados en `quorum.md` §🚦 Routing & Risk Governance / "What is NOT needed":

| Componente rechazado | Razón |
|---|---|
| Sistema de pesos numéricos (+1, +2, +5, +10 puntos) | Magic numbers sin telemetría. Reemplazado por señales binarias en `RISK-001`. |
| Artefacto dedicado `routing_decision.json` | Duplica `07-trace.json` y `02-contract.yaml.execution`. Sin justificación para un slot nuevo. |
| Hardcoding de modelos (`gemini-1.5-flash`, `o1-preview`) en el scoring | Los modelos cambian; el scorer emite niveles 0/1/2 y `config.yaml` resuelve nombres. |
| Auto-override silencioso del `risk` declarado por humano | Viola Regla #7 (autoridad de política, no de agente). Sustituido por modelo "fail-safe: max(declared, calculated)". |
| Re-implementación de detección de símbolos o profundidad de imports | Los retrievers existen (`ast_neighbors.py`, `import_graph.py`). Reusarlos, no re-escribirlos. |

---

## 4. Precondiciones para Desbloquear

**No implementar §2 hasta que TODAS estas condiciones se cumplan:**

| # | Precondición | Justificación |
|---|---|---|
| 1 | Plan `RISK-001` (scorer basado en señales) está implementado y en uso. | Es la pieza estructural sobre la que se aplica runtime y tuning. |
| 2 | `task_manager.run_task` existe como bucle real, no stub. | Sin dispatcher consumidor, la aplicación automática es código muerto. |
| 3 | ≥20 tareas completadas con `assign_risk_level()` registrando resultado en `07-trace.json`. | Sin masa crítica, el tuning empírico es ruido. |
| 4 | Telemetría agregada disponible (script de audit funcional o consulta manual sobre `done/` + `failed/`). | El tuning se justifica con datos, no con intuición. |
| 5 | Al menos un caso documentado de divergencia significativa (declarado vs. calculado, o falso negativo en producción). | Evidencia de que el problema existe en este proyecto. |

Si la condición 3 no se cumple en 6 meses, **revisar si Quorum está siendo usado para tareas suficientemente diversas como para justificar el tuning**. Es posible que el scorer estático sea suficiente.

---

## 5. Trazabilidad de la Decisión

- **Propuesta original:** scoring numérico + 4 factores ponderados + nuevo artefacto + mapeo a modelos por nombre.
- **Análisis de factibilidad:** 60% de la infraestructura ya existía (políticas + retrievers + niveles + schema). El 40% restante se dividió en estática (scorer) y dinámica (runtime + tuning).
- **Acción inmediata:** plan `3-exec/RISK-001-scorer-senales-blueprint.md` — función pura, señales binarias, sin pesos arbitrarios.
- **Acción diferida:** este documento — esperar dispatcher y datos antes de invertir en runtime y tuning.
- **Acción rechazada:** §3 — documentada en `quorum.md` para evitar reintroducción.

---

## 6. Próximos Pasos para el Agente Revisor (cuando se desbloquee)

1. Validar que las 5 precondiciones de §4 se cumplen con datos verificables.
2. Implementar §2.1 antes que §2.2 y §2.3 (la aplicación automática es prerrequisito de la telemetría útil).
3. Generar reporte de divergencia (§2.2) tras 20+ tareas y proponer ajustes de umbral con evidencia, no con opinión.
4. Implementar §2.3 (profundidad de dependencias) solo si §2.2 demuestra falsos negativos atribuibles a transitividad.
5. Escribir ADR en `docs/adr/` por cada cambio de umbral, citando el ratio de divergencia que lo motivó.
6. Confirmar que la implementación NO reintroduce ninguno de los componentes rechazados en §3.
