# 💡 Propuesta Técnica: Protocolo de Re-negociación de Contrato (SDC-Renegotiate)

**Estado:** Diferida — implementar cuando se cumplan las precondiciones (ver §7).
**Contexto:** Evolución de Quorum v1.2 (no v1.1).
**Referencia:** Basado en la limitación de `owned_files` observada en Spec Kitty.
**Veredicto de factibilidad:** Conceptualmente sólida, compatible con la Constitución, pero NO prioritaria hasta que exista bucle de ejecución funcional y telemetría de fallos.

---

## 1. El Problema: El "Callejón sin Salida" Contractual
En el framework Quorum, el agente ejecutor está obligado por el `02-contract.yaml` (Regla #3 de la Constitución).

### Síntomas en Spec Kitty (Antecedente):
- El agente descubre una dependencia oculta (ej. un archivo de constantes o un entrypoint no mapeado).
- El agente tiene prohibido tocarlo por su `owned_files`.
- Resultado: El agente alucina (ignora la restricción), falla la tarea silenciosamente, o se detiene requiriendo intervención humana manual y costosa.

### Punto Ciego en Quorum:
La rigidez determinista es una virtud, pero sin un mecanismo de salida, se convierte en un punto único de falla ante la incertidumbre arquitectónica.

---

## 2. Solución Propuesta: Protocolo de Re-negociación Activa
Implementar un flujo de "Halt & Request" donde el agente ejecutor puede solicitar una expansión de su contrato de forma estructurada.

### Flujo de Trabajo (The Loop):
1. **Detección**: Durante `q-implement`, el agente identifica que el éxito de la tarea requiere modificar `file_X`, el cual NO está en el contrato.
2. **Inhibición Proactiva**: El agente se detiene inmediatamente. No realiza cambios parciales.
3. **Solicitud de Renegociación**: El agente genera una solicitud estructurada (ubicación a definir según §6.1; no reservar slot 09 por defecto) con:
   - `requested_files`: Lista de archivos adicionales.
   - `rationale`: Por qué son necesarios (símbolos, dependencias, efectos secundarios).
   - `risk_impact`: Evaluación inicial del agente sobre si esto cambia la complejidad de la tarea.
4. **Disparo de Re-Blueprint**: El orquestador detecta el archivo y lanza `q-blueprint --mode renegotiate`.
   - El **Surgical Cartographer** revisa la solicitud.
   - Si es válida, actualiza `01-blueprint.yaml` y regenera `02-contract.yaml`.
5. **Re-Resume**: El agente recibe el contrato actualizado y continúa la ejecución.

---

## 3. Especificaciones Técnicas para Evitar Ambigüedades

### A. Estructura de la Solicitud de Renegociación
```json
{
  "task_id": "FEAT-001",
  "requester": "executor-l0",
  "timestamp": "ISO-8601",
  "missing_context": [
    {
      "path": "src/core/utils.py",
      "reason": "Necesito exportar un nuevo tipo para evitar dependencias circulares en el modelo.",
      "symbols": ["AuthType", "ValidationResult"]
    }
  ],
  "current_implementation_state": "dirty|clean",
  "blocker_severity": "critical|minor"
}
```

### B. Guardrail: La Regla de los Tres Intentos
Para evitar que un agente entre en un bucle infinito de "pedir un archivo más" debido a una mala planificación:
- Máximo de **2 re-negociaciones** por tarea.
- Si se requiere una 3ª, la tarea se mueve automáticamente a `failed/` y se escala a un arquitecto humano.

### C. Evaluación de Riesgo Dinámica
La propuesta original usaba el umbral de "20% de archivos nuevos" como gatillo. Este umbral es arbitrario y debe sustituirse por una **señal compuesta** que considere:

- Variación proyectada en `limits.max_diff_lines`.
- Aparición de archivos que matchean `forbid.files` o paths con tests críticos.
- Costo acumulado del intento vs. `max_cost_per_call_usd` del nivel actual.
- Cambio en la profundidad de dependencias respecto al blueprint original.

Si dos o más señales se disparan, el dispatcher debe:
- Re-evaluar `policies/risk.yaml`.
- Considerar el cambio de modelo (ej. de L0 a L2) para manejar la nueva complejidad.

---

## 4. Puntos Ciegos Identificados y Mitigación

| Punto Ciego | Mitigación |
| :--- | :--- |
| **Bucle de Consumo**: El agente pide archivos para "limpiar" código no relacionado. | El `q-blueprint` en modo re-negociación debe validar que la petición esté estrictamente ligada a los invariantes de `00-spec.yaml`. |
| **Estado de Git Sucio**: ¿Qué pasa con los cambios ya realizados antes de la re-negociación? | El protocolo exige que la re-negociación ocurra ANTES de escribir código. Esto requiere añadir una **fase explícita de exploración** en `q-implement`, hoy inexistente. `git stash` dentro de un worktree es frágil y queda como último recurso. |
| **Inconsistencia de Esquema**: ¿Qué pasa si el nuevo archivo rompe la validación de otros WPs? | El Re-Blueprint debe ser "Global-Aware" dentro de la tarea, revisando colisiones con otros contratos activos. |
| **Erosión de la Disciplina de Blueprint**: Si re-negociar es barato, los Cartógrafos pueden generar blueprints laxos. | Trackear `renegotiation_count` por tarea en `07-trace.json` y reportar el ratio por Cartógrafo. Un ratio elevado indica que la inversión real está en mejorar `q-blueprint`, no en el fallback. |

---

## 5. Análisis de Compatibilidad con la Constitución

| Regla | Compatibilidad | Observación |
| :--- | :--- | :--- |
| **#2** Contexto Determinista | ✅ Se preserva | El agente no expande contexto unilateralmente; solicita y `q-blueprint` decide. |
| **#3** Sin parches fuera del contrato | ✅ Se refuerza | Convierte la violación silenciosa en un evento estructurado. |
| **#4** Validación es finalidad | ⚠️ Neutra | `verify.commands` debe re-ejecutarse íntegramente tras la re-negociación. |
| **#7** Costo limitado por política | ⚠️ Riesgo | Cada re-negociación es una llamada L2 (Opus, hasta `$3.00`). Dos re-negociaciones pueden multiplicar el costo de la tarea. |
| **#8** Tests son la única prueba | ✅ Se preserva | El re-blueprint regenera el contrato; la validación sigue siendo la prueba final. |

**Conclusión:** No viola ninguna regla inmutable. El agente sigue siendo "ejecutor quirúrgico"; solo gana un canal estructurado para señalar incompletitud del blueprint.

---

## 6. Estado actual del sistema (verificado contra el código)

- `skills/q-implement/SKILL.md` ya implementa una versión manual: instruye `BLOCKED` cuando el contrato es insuficiente. Esto es el protocolo propuesto, sin automatización.
- `.agents/cli/core/task_manager.py:174` (`run_task`) es un stub. **El bucle de ejecución todavía no existe.**
- `.agents/schemas/contract.schema.json` usa `additionalProperties: false`. No se puede parchear un contrato; hay que regenerarlo. Compatible con la propuesta.
- No hay dispatcher, no hay enrutamiento de modelos, ni captura de eventos en `07-trace.json` durante ejecución.

**Implicación:** Construir re-negociación sobre un ejecutor inexistente es prematuro.

### 6.1 Boundary de artefactos

La propuesta original usaba `09-renegotiation-request.json`, pero la gobernanza actual de Quorum rechaza la expansión automática de slots 08+ (ver `quorum.md` §Artifact lifecycle boundary).

Si la renegociación se implementa en el futuro, la solicitud debe resolverse en este orden de preferencia:

1. **Evento en `07-trace.json`** si solo se necesita auditoría.
2. **Entrada estructurada en `04-implementation-log.yaml`** si es parte del bloqueo del ejecutor.
3. **Artefacto transitorio nuevo** solo si existe consumidor determinístico, schema oficial y ADR previa.

No coordinar esta propuesta con `08-post-mortem.json` ni `10-impact-report.json`: ambos quedaron rechazados por duplicar artefactos existentes y `q-memory`.

---

## 7. Precondiciones para Implementación

**No implementar hasta que TODAS estas condiciones se cumplan:**

1. **Bucle de ejecución real**: `task_manager.run_task` funcional (no stub), capaz de invocar el ejecutor y capturar resultados.
2. **Dispatcher operativo**: Enrutamiento real a modelos según `policies/risk.yaml`.
3. **Trace en tiempo real**: `07-trace.json` con eventos por intento (no solo inicialización), incluyendo costo, retries y violaciones.
4. **Telemetría de fallos**: Contador en `07-trace.json` de eventos `blocked_by_contract` durante al menos 4-6 semanas de operación real.
5. **Evidencia cuantitativa**: ≥15-20% de tareas terminando en `BLOCKED` por contrato incompleto. Por debajo de ese umbral, la inversión correcta es mejorar `q-blueprint`, no añadir el fallback.

---

## 8. Plan de Implementación Diferida

| Fase | Acción | Justificación |
| :--- | :--- | :--- |
| **v1.1 (ahora)** | Formalizar `BLOCKED: missing_file=<path>` como protocolo manual en `q-implement/SKILL.md`. Estandarizar el mensaje para que sea parseable. | Cero costo. Recolecta datos. |
| **v1.1.x** | Añadir contador `blocked_by_contract` en `07-trace.json`. Crear ADR en `docs/adr/` registrando la decisión de diferir y el racional. | Telemetría barata; auditoría de la decisión. |
| **v1.2** | Si la telemetría justifica el ROI, implementar el protocolo con los ajustes de §3.C, §4 y §6.1. | Decisión basada en evidencia. |

---

## 9. Próximos Pasos para el Agente Revisor (cuando se desbloquee)

1. Confirmar si la solicitud de renegociación vive como evento en `07-trace.json`, entrada en `04-implementation-log.yaml`, o artefacto transitorio con ADR y schema propios.
2. Definir la lógica de "Merge de Contratos" en `task_manager.py`, incluyendo cómo regenerar `02-contract.yaml` sin invalidar el trace existente.
3. Validar empíricamente si este protocolo reduce o aumenta el costo operativo (Token Cost vs Human Intervention Cost) usando los datos recolectados en la fase de telemetría.
4. Definir el formato exacto del mensaje `BLOCKED` manual durante v1.1 para que sea retrocompatible con la estructura JSON automatizada de v1.2.
