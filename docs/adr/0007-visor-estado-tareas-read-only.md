# ADR 0007: Visor read-only de estado de tareas (`quorum serve`)

**Date:** 2026-06-06
**Status:** Accepted

## Contexto

ADR 0004 sancionó una capacidad acotada de visualización read-only en `quorum serve` (reportes autorados por `q-report`) y ADR 0006 la extendió a la memoria curada. Ambos ADR dejaron **explícitamente fuera de alcance** la visualización de artefactos del ciclo de vida `05-validation.json`, `06-review.json` y `07-trace.json`:

- ADR 0004, condición 6: *"La visualización de artefactos del ciclo (`07-trace.json`, `05-validation.json`, `06-review.json`) queda **fuera** de este ADR."*
- ADR 0006, sección "Prohibido": *"Visualizar artefactos lifecycle `05-validation.json`, `06-review.json` o `07-trace.json` desde esta superficie."*

El manifiesto (`quorum.md`, "Bounded exception (ADR 0004)") replica esa frontera: el visor *"does not visualize lifecycle artifacts (`05`/`06`/`07`)"*. La autoridad canónica es el manifiesto: cuando código y manifiesto discrepan, gana el manifiesto y el código es el bug.

Existe un problema operativo real: hoy el estado de una tarea solo es inspeccionable por CLI (`quorum task list`, `quorum task status`, `/q-status`), que imprime texto denso a stdout. Revisar el estado del ciclo de N tareas y N proyectos genera la misma fatiga cognitiva que ADR 0004 buscaba reducir para reportes. La capacidad encaja en el visor embebido existente, pero **cruza deliberadamente** la frontera que 0004 y 0006 cerraron. Construirla sin registrar la decisión la convertiría en una violación del manifiesto, no en un refactor.

## Decisión

Se amplía la misión para **sancionar la visualización read-only del estado de tareas** de `.ai/tasks/{inbox,active,done,failed}` en `quorum serve`, bajo las siguientes condiciones normativas vinculantes.

### 1. Solo lectura, sin excepciones

Los handlers de tareas se limitan a `os.ReadDir`, `os.Stat`, `os.ReadFile`, parsing (YAML/JSON) y validación read-only. Está **prohibido** que cualquier handler o la UI invoque `SaveArtifact`, `MoveTask`, `CleanTask`, `EnsureTraceAppendOnly`, `PrepareFailedChildRetry`, `EnsureMemoryProject`, escritura SQLite, creación/borrado de worktrees, o cualquier mutación de `.ai/tasks`. La UI **no** incluye botones de transición (`back`, `clean`, `retry`, `artifact-save`); como máximo muestra comandos CLI sugeridos como texto para que el humano los ejecute manualmente.

### 2. La Constitución queda intacta

El visor **no emite veredictos ni tiene autoridad de merge**: Regla #4 (Validation is Finality) y Regla #6 (The System Commits, Never Merges) se mantienen — mostrar `05-validation.json` o `06-review.json` es observación, no decisión. Regla #1 (Git es la verdad) intacta: el estado de la tarea sigue siendo el directorio de artefactos; el visor lo lee, no lo reemplaza por una BD. Regla #8 (los tests son la única prueba) intacta: el visor no fabrica evidencia. Regla #9 intacta: el visor no es un skill ni auto-activa fases. `q-status`, `q-verify`, `q-review`, `q-accept` siguen siendo las rutas operacionales; el visor solo observa.

### 3. No nueva fase ni artefacto numerado

El visor **solo lee** artefactos `00`–`07` existentes y `feedback.json`. No crea slots `03`, `08`, `09`, `10` ni ningún artefacto numerado nuevo. La frontera de "no nuevo artefacto numerado" no se relaja.

### 4. `07-trace.json` se preserva append-only

El trace solo se lee o se resume; **nunca** se reescribe desde esta superficie. No se invoca ningún path de escritura de trace. La evidencia append-only del ciclo queda intacta.

### 5. Aislamiento por proyecto y seguridad de paths

Los handlers escanean únicamente el `root_path` del proyecto seleccionado, leído de la tabla `projects` de la SQLite central (poblada por `EnsureMemoryProject`). Los proyectos sin `root_path` se omiten de `/api/projects` y sus subrutas directas responden no disponible (paridad con ADR 0006). El estado multi-proyecto se calcula contra el `root_path` consultado, **nunca** contra `ProjectRoot()` dinámico del cwd donde corre el servidor (variantes `...In(projectRoot)`). Los `taskID` se validan como ID de tarea (`^[A-Z]+-[0-9]+$` o `^[A-Z]+-[0-9]+-[a-z]$`), nunca como path; se rechaza `/`, `\`, `.`, `..` y traversal URL-encoded. Toda ruta resuelta se confirma dentro de `<root_path>/.ai/tasks` (`filepath.Rel`).

### 6. Política de exposición de campos lifecycle (completo vs. resumido)

Esta es la condición que esta decisión añade respecto a ADR 0004/0006. Para acotar la superficie y evitar fuga de contexto grande, los campos se clasifican así:

| Artefacto | Exposición | Justificación |
|-----------|------------|---------------|
| `00-spec.yaml` | **Completo** (colapsable) | Intención humana; bajo riesgo |
| `01-blueprint.yaml` | **Completo** (colapsable) | Estrategia técnica; bajo riesgo |
| `02-contract.yaml` | **Resumido**: `summary`, `goal`, `touch`, `verify_commands` | El `context_bundle` puede ser grande y ruidoso; no se vuelca completo en v1 |
| `04-implementation-log.yaml` | **Completo** (colapsable) | Registro de lo hecho; bajo riesgo |
| `05-validation.json` | **Completo** (colapsable) | Evidencia de validación; observación, no veredicto (cond. 2) |
| `06-review.json` | **Completo** (colapsable) | Resultado de review; observación, no veredicto (cond. 2) |
| `07-trace.json` | **Resumido**: `summary`, `attempts_count`, `last_attempt`, `total_cost_usd` | Append-only y potencialmente extenso; nunca se reescribe (cond. 4) |
| `feedback.json` | **Completo** (colapsable) | Ticket de reparación efímero; bajo riesgo |

"Completo (colapsable)" significa que el JSON/YAML íntegro puede mostrarse, pero detrás de una sección colapsada por defecto. El listado nunca falla entero por un artefacto corrupto: se marca ese artefacto como ilegible/ inválido y se continúa.

### 7. Capacidad acotada

El alcance es **observar** el estado del ciclo. Quedan **fuera** y requieren un ADR posterior: edición de artefactos, disparo de transiciones, export (PDF/CSV), constructor de vistas, temas, o cualquier mutación de estado, worktrees o SQLite.

### 8. Enmienda del manifiesto

El párrafo "Bounded exception" de `quorum.md` se enmienda para referenciar ADR 0006 y este ADR 0007, y para reemplazar la afirmación *"does not visualize lifecycle artifacts (`05`/`06`/`07`)"* por la frontera vigente: el visor **puede** mostrar artefactos del ciclo en modo read-only y resumido según la condición 6, sin emitir veredicto ni autoridad de merge y sin mutar estado. Hasta que esa enmienda se aplique, este ADR es el registro de la intención.

## Consecuencias

- **Positivas:** Reduce la fatiga cognitiva al inspeccionar el estado del ciclo entre tareas y proyectos; reusa el patrón handler→core→JSON y la maquinaria del visor (reports/memories) sin infraestructura nueva; extraer un query core puro (`internal/core/task_query.go`) y hacer que `ListTasks()`/`ShowStatus()` lo consuman separa dominio de presentación y reduce deuda existente; la ampliación de misión queda explícita y auditable.
- **Negativas:** Expande la superficie del visor más allá de reportes/memoria hacia el estado del ciclo, acercándolo a una "Human-Centric UI"; el riesgo de scope creep (botones mutantes, edición) se acota con las condiciones 1, 2 y 7 y con el requisito de ADR posterior.
- **Neutrales:** No se introduce artefacto numerado nuevo; la gobernanza de la SQLite no cambia (solo lectura de `projects`); el trace permanece append-only. Este ADR **se relaciona con 0004 y 0006** (misma capacidad de visor read-only) y **enmienda explícitamente** la exclusión de artefactos lifecycle declarada en ADR 0004 (condición 6) y ADR 0006 (sección "Prohibido"): esa exclusión queda superada por la condición 6 de este ADR. No supersede el resto de 0004 ni 0006.
