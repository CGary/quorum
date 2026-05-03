# Quorum ⚖️

**Constraints in. Verified diffs out. Costs bounded. Humans only where humans matter.**

Quorum es un framework **AI-first** para ejecutar funcionalidades complejas mediante contratos verificables. Convierte una intención humana en artefactos machine-first (`00` → `07`), limita el contexto que recibe cada agente y exige que el resultado se pruebe con comandos reales antes de revisión humana.

> Estado actual: **MVP de orquestación y artefactos**. Quorum ya incluye schemas, skills, CLI de tareas, worktrees aislados, políticas de riesgo/routing, scoring de riesgo y lookup de fallos relacionados. El dispatcher/runtime automático completo todavía está diferido: `task_manager.run_task()` sigue siendo stub.

---

## 🧠 Filosofía

- **Spec-Driven Contracts (SDC):** el flujo lógico es Spec → Blueprint → Contract → Verified Diff.
- **Machine-first, formato según audiencia:** YAML para planificación; JSON para captura del sistema.
- **Contexto determinista:** el agente no recibe “todo el proyecto”; recibe archivos y restricciones derivados del blueprint.
- **Ejecución aislada:** cada tarea corre en su propio Git worktree y rama `ai/<TASK_ID>`.
- **Merge humano:** Quorum puede preparar y commitear trabajo en rama, pero nunca mergea a `main`.
- **Memoria curada:** `memory/*.json` guarda conocimiento durable solo cuando `q-memory` se invoca explícitamente.

### Lo que Quorum NO es

- No es un chatbot general del repo.
- No es una herramienta para cambios triviales de 5 líneas.
- No es un generador de documentación narrativa.
- No es un sistema de merge automático.
- No depende de HSME/vector DBs: sistemas externos pueden consumir `memory/*.json`, pero Quorum es local-first.

---

## ✅ Estado actual del proyecto

### Implementado

- CLI `quorum task ...` para inicializar, listar, activar, crear worktrees, consultar estado y limpiar tareas.
- Schemas JSON para artefactos:
  - `spec`, `blueprint`, `contract`, `validation`, `review`, `trace`, `memory`.
- Skills operativos:
  - `q-brief`, `q-blueprint`, `q-analyze`, `q-implement`, `q-verify`, `q-review`, `q-accept`, `q-memory`, `q-status`.
- Worktrees por tarea en `worktrees/<TASK_ID>/`.
- Políticas de riesgo/routing en `.agents/policies/`.
- `risk_scorer.py` para sugerir riesgo desde `01-blueprint.yaml`.
- `failure_lookup.py` para consultar tareas fallidas relacionadas durante blueprint.
- `05-validation.json.error_category` opcional:
  - `logic | dependency | environment | flaky | unknown`.
- Gobernanza documentada para evitar propuestas duplicadas:
  - memoria curada,
  - routing/risk,
  - failure handling,
  - concurrency/merge-gate,
  - límite de artefactos `00`-`07`.

### Diferido / no implementado aún

- Dispatcher automático de ejecución.
- `task_manager.run_task()` real.
- Auto-retry y re-blueprint automático tras fallo.
- Renegociación automática de contrato.
- Shadow merge / pre-merge gate automático.
- Auto-rebase.
- Nuevos artefactos `08-post-mortem.json` o `09/10-impact-report.json` — rechazados por duplicar `05/06/07` y `q-memory`.

---

## 📜 Constitución: reglas inmutables

1. **Git es la verdad del código.** La memoria semántica es para patrones; Git es para código.
2. **Contexto determinista.** Los agentes reciben contexto derivado del blueprint, no el repo completo.
3. **Sin parches fuera del contrato.** Tocar archivos fuera de `02-contract.yaml.touch` rechaza la tarea.
4. **La validación es la finalidad.** Nada está terminado hasta que `verify.commands` pase.
5. **Artefactos machine-first.** YAML/JSON para operación; Markdown solo para docs/ADR.
6. **El sistema commitea, nunca mergea.** El merge a `main` es humano.
7. **El costo está limitado por política.** Routing/retries/escalaciones son política, no confianza.
8. **Los tests son la única prueba.** Specs y blueprints no prueban funcionalidad.

---

## 📂 Artefactos canónicos

Quorum usa `00` a `07` más memoria curada. No se agregan slots nuevos sin ADR, schema y consumidor determinístico.

| Archivo | Formato | Quién lo produce | Propósito |
|---|---|---|---|
| `00-spec.yaml` | YAML | Humano + `q-brief` | Qué se quiere lograr, invariantes y aceptación. |
| `01-blueprint.yaml` | YAML | `q-blueprint` | Ruta técnica: archivos, símbolos, dependencias, estrategia. |
| `02-contract.yaml` | YAML | `q-blueprint` / Gatekeeper | Qué puede tocar el agente, qué no, comandos de verificación y límites. |
| `04-implementation-log.yaml` | YAML | `q-implement` | Cambios realizados, blockers e intentos. |
| `05-validation.json` | JSON | `q-verify` | Comandos ejecutados, exit codes, output y resultado global. |
| `06-review.json` | JSON | `q-review` | Revisión del diff contra contrato y validación. |
| `07-trace.json` | JSON | Sistema/skills | Intentos, coste, fases, violaciones y resultado. |
| `memory/*.json` | JSON | `q-memory` | Decisiones, patrones y lecciones durables. |

### Boundary de artefactos

- No crear `08-post-mortem.json`: los datos del fallo viven en `05`, `06`, `07` y `memory/lessons`.
- No crear `09/10-impact-report.json`: el aprendizaje exitoso va directo a `q-memory`.
- Routing, merge-gate y eventos operativos deben registrarse en `07-trace.json` salvo ADR que justifique otra cosa.

---

## 🚀 Inicio Rápido

Quorum se instala como una herramienta global mediante `uv` para que puedas usarlo en cualquier proyecto de forma aislada.

### 1. Instalación Global

Clona el repositorio y utiliza `uv tool install`:

```bash
git clone https://github.com/usuario/quorum.git
cd quorum
uv tool install --editable .
```

Esto registrará el comando `quorum` en tu PATH. Al ser una instalación `--editable`, cualquier mejora que descargues o hagas en el código de Quorum se reflejará instantáneamente sin necesidad de re-instalar.

### 2. Inicializar un Proyecto

Ve a tu proyecto de software (ej. `hsme`) y prepara la estructura de Quorum:

```bash
cd /ruta/a/tu/proyecto
quorum init
```

Esto creará automáticamente:
- Directorios de tareas: `.ai/tasks/{inbox,active,done,failed}`.
- Directorios de memoria curada: `memory/{decisions,patterns,lessons}`.
- Configuración de `.gitignore` para proteger tus worktrees y artefactos temporales.

---

## 🛠 Flujo de Trabajo Operativo

El ciclo combina dos actores que hacen cosas distintas. Confundirlos es la causa #1 de pasos saltados.

| Actor | Responsabilidad | Herramienta |
|---|---|---|
| **Orquestador** (humano o runtime externo) | Mueve la tarea entre estados (`inbox` → `active` → `done`/`failed`), crea worktrees, hace merge a `main`, archiva la tarea | Comandos `quorum task ...` + Git |
| **Skill (AI)** | Produce artefactos (`00`–`07`, memoria) dentro de los límites de su fase | Slash commands `/q-*` |

### ⚠️ Reglas de oro tras la modularización (Regla #9 en `quorum.md`)

Los skills **ya no** ejecutan transiciones de estado por su cuenta. Cualquier comando CLI que aparezca abajo lo corre **el orquestador, no el skill**. Específicamente:

- `/q-brief` **no** corre `quorum task blueprint`. Si lo despachás sin haber hecho la transición, el skill seguiente no encontrará la tarea en `active/`.
- `/q-blueprint` **no** corre `quorum task start`. Si lo despachás sin worktree, `/q-implement` se bloqueará con `BLOCKED: worktree missing`.
- `/q-accept` **no** ejecuta `git merge`, ni la suite BDD, ni `quorum task clean`. Solo emite veredicto `ready|not_ready`. El merge y la limpieza son manuales después.
- Ningún skill activa al siguiente skill. Si querés despachar `/q-implement` después de `/q-blueprint`, lo hacés vos.

Si alguno de estos pasos se omite, la tarea queda en un estado intermedio inconsistente. Usá `quorum task status <ID>` o `/q-status <ID>` para diagnosticar dónde quedó.

### Secuencia canónica (FEAT-001 como ejemplo)

Cada fila es un dispatch independiente. **Nada salta entre filas automáticamente.**

| # | Actor | Acción | Comando o skill | Artefacto / efecto |
|---|---|---|---|---|
| 1 | Orquestador | Crear tarea en `inbox/` | `quorum task specify FEAT-001` | Directorio + `00-spec.yaml` esqueleto |
| 2 | Skill (AI) | Llenar la especificación | `/q-brief FEAT-001` | `00-spec.yaml` completo |
| 3 | Orquestador | Promover a `active/` | `quorum task blueprint FEAT-001` | Tarea movida; lista para blueprint |
| 4 | Skill (AI) | Diseñar blueprint y contrato | `/q-blueprint FEAT-001` | `01-blueprint.yaml` + `02-contract.yaml` + risk events en `07-trace.json` |
| 5 | Skill (AI) — **opcional** | Auditar consistencia entre `00`/`01`/`02` | `/q-analyze FEAT-001` | Reporte read-only (sin artefacto nuevo) |
| 6 | Orquestador | Crear worktree y rama `ai/FEAT-001` | `quorum task start FEAT-001` | `worktrees/FEAT-001/` + rama |
| 7 | Skill (AI) | Implementar dentro del contrato | `/q-implement FEAT-001` | Diff committeado en `ai/FEAT-001` + `04-implementation-log.yaml` |
| 8 | Skill (AI) | Correr `verify.commands` | `/q-verify FEAT-001` | `05-validation.json` |
| 9 | Skill (AI) | Revisar diff vs contrato | `/q-review FEAT-001` | `06-review.json` |
| 10 | Skill (AI) | Compuerta de aceptación | `/q-accept FEAT-001` | Veredicto `ready` o `not_ready` |
| 11 | Humano | Correr suite BDD (si el contrato la define) | `<acceptance.bdd_suite>` manual | Pase/fail manual |
| 12 | Humano | Inspeccionar diff y mergear a `main` | `git merge ai/FEAT-001` (manual) | Código en `main` |
| 13 | Orquestador | Archivar tarea y borrar worktree | `quorum task clean FEAT-001` | Tarea en `done/` (o `failed/`) |
| 14 | Skill (AI) | Capturar lecciones durables | `/q-memory FEAT-001` | Entradas en `memory/{decisions,patterns,lessons}/` |

### Detalle por fase

#### 1. Crear la tarea — orquestador

```bash
quorum task specify FEAT-001
```

Crea `.ai/tasks/inbox/FEAT-001-<slug>/` con plantillas. **Hacelo siempre antes** de despachar `/q-brief`; el skill no creará la tarea por vos.

#### 2. Especificar — skill `/q-brief`

```text
/q-brief FEAT-001
```

Entrevista al humano y completa `00-spec.yaml` con goal, invariantes, criterios de aceptación, no-objetivos y `risk`. Aplica el `Scope Gate`: si la solicitud es trivial, te dice que no uses Quorum.

> El skill **no** mueve la tarea a `active/`. Termina diciendo `Next phase: quorum task blueprint <TASK_ID>, then /q-blueprint <TASK_ID>`.

#### 3. Promover a active — orquestador

```bash
quorum task blueprint FEAT-001
```

Mueve `inbox/FEAT-001-*/` → `active/FEAT-001-*/`. **Si saltás este paso**, `/q-blueprint` no encontrará la tarea en `active/` y fallará.

#### 4. Blueprint + contrato — skill `/q-blueprint`

```text
/q-blueprint FEAT-001
```

El agente:

- mapea archivos, símbolos y dependencias afectadas;
- consulta tareas fallidas relacionadas vía `failure_lookup.py`;
- corre `risk_scorer.py` y registra los eventos de riesgo en `07-trace.json` (sin pisar el riesgo declarado por el humano);
- genera `01-blueprint.yaml` (estrategia) y `02-contract.yaml` (`touch`, `read`, `forbid`, `verify.commands`, `limits`, `execution`, `retry_policy`).

> El skill **no** crea el worktree. Termina con `Next phase: /q-analyze <TASK_ID> (recommended) or quorum task start <TASK_ID>`.

#### 5. Análisis de consistencia (opcional pero recomendado) — skill `/q-analyze`

```text
/q-analyze FEAT-001
```

**Read-only.** Cruza `00`, `01`, `02` contra schemas y policies. Reporta:

- aceptación sin test scenarios,
- archivos del blueprint que no están en `02-contract.yaml.touch`,
- comandos BDD lentos colocados como `verify.commands`,
- divergencias de risk no reflejadas en `07-trace.json`.

No corrige nada. Si encuentra `issues_found`, despachás `/q-blueprint` de nuevo (a un modelo capaz de razonar sobre el contrato) para arreglarlo. Si pasa, seguís al paso 6.

#### 6. Crear worktree — orquestador

```bash
quorum task start FEAT-001
```

Genera `worktrees/FEAT-001/` y la rama `ai/FEAT-001` desde `main` (o la rama base detectada). **Sin este paso**, `/q-implement` se bloquea de inmediato.

#### 7. Implementación — skill `/q-implement`

```text
/q-implement FEAT-001
```

El agente lee `02-contract.yaml` como autoridad vinculante. Toca solo archivos en `touch`, respeta `forbid`, registra cambios en `04-implementation-log.yaml` y commitea dentro del worktree en `ai/FEAT-001`. Tocar fuera del contrato rechaza la tarea.

> El skill **no** ejecuta `verify.commands` ni activa `/q-verify`. Termina con `DONE: …` o `BLOCKED: …`.

#### 8. Verificación — skill `/q-verify`

```text
/q-verify FEAT-001
```

Corre cada `verify.commands` dentro del worktree y captura exit codes, duración y un excerpt de salida. Escribe `05-validation.json` con `overall_result` (`passed | failed | blocked`) y, si falló, un `error_category` heurístico (`logic | dependency | environment | flaky | unknown`).

> El skill **no** edita código para arreglar fallos ni decide reintentos. Si falla, el orquestador decide si vuelve a `/q-implement` (logic/dependency) o reintenta `/q-verify` (environment/flaky).

#### 9. Revisión — skill `/q-review`

```text
/q-review FEAT-001
```

Compara el diff de `worktrees/FEAT-001/` contra `00`, `01`, `02` y `05`. Emite veredicto en `06-review.json`: `approve | revise | reject`. Si revise, devuelve `fix_tasks` estructurados. No edita código, no aprueba con validation no pasada, no mergea.

#### 10. Compuerta de aceptación — skill `/q-accept`

```text
/q-accept FEAT-001
```

Verifica de forma agregada: `05.overall_result == passed`, `06.verdict == approve`, `06.contract_compliance == true`, `forbidden_files_touched` vacío, sin refactors no pedidos, sin violaciones abiertas en `07-trace.json`. Emite `Acceptance: ready` o `not_ready` con bloqueantes.

> Importante: `/q-accept` **no** ejecuta el merge, **no** corre la suite BDD y **no** archiva la tarea. Solo emite el go/no-go. Los pasos 11–13 son obra del humano y del orquestador.

#### 11. Suite BDD — humano

Si `02-contract.yaml.acceptance.bdd_suite` está definido, corré ese comando manualmente fuera del agent loop. Es la única evidencia que cubre criterios de aceptación end-to-end (Política de testing).

#### 12. Merge — humano

```bash
git -C worktrees/FEAT-001 log --oneline   # inspeccionar
git checkout main
git merge ai/FEAT-001
```

Rule #6 es ley: el sistema commitea, nunca mergea. El merge es manual.

#### 13. Limpieza — orquestador

```bash
quorum task clean FEAT-001
```

Archiva la tarea en `done/` (o `failed/` si nunca pasó la compuerta) y elimina el worktree. Hacelo **después** del merge; antes deja huérfanos los commits de `ai/FEAT-001` si no fueron mergeados.

#### 14. Captura de memoria — skill `/q-memory`

```text
/q-memory FEAT-001
```

Genera entradas en `memory/decisions/`, `memory/patterns/` o `memory/lessons/`. Es la única vía de ingesta de memoria; no hay captura automática (Memory Governance: human-invoked, never automatic). También se puede invocar sobre tareas en `failed/` con lección durable; en ese caso captura un `lesson` con `anti_patterns`.

### Errores comunes y cómo detectarlos

| Síntoma | Causa probable | Diagnóstico |
|---|---|---|
| `/q-blueprint` dice "task not found in active" | Saltaste `quorum task blueprint <ID>` | `quorum task status <ID>` mostrará la tarea aún en `inbox/` |
| `/q-implement` responde `BLOCKED: worktree missing` | Saltaste `quorum task start <ID>` | `ls worktrees/` no listará la tarea |
| `/q-verify` responde `blocked` | Falta `02-contract.yaml.verify.commands` | Volver a `/q-blueprint` y completar el contrato |
| `/q-review` devuelve `revise` con `fix_tasks` | Validation pasó pero el diff sale del contrato | Despachar `/q-implement` con los `fix_tasks` |
| `/q-accept` queda en `not_ready` por trace | Hay violaciones sin resolver en `07-trace.json` | Inspeccionar `07-trace.json.violations` |
| Tarea sigue apareciendo en `active/` después del merge | Saltaste `quorum task clean <ID>` | Correr `quorum task clean <ID>` ahora |
| Lecciones no aparecen en `memory/` | `/q-memory` nunca fue invocado | La memoria es manual; no hay auto-captura |

### Utilidades transversales (read-only)

```bash
quorum task list              # resumen de todas las tareas y su estado
quorum task status FEAT-001   # estado, artefactos presentes y próximo paso
```

```text
/q-status            # vista global con próxima acción recomendada
/q-status FEAT-001   # diagnóstico por tarea
```

`/q-status` es el equivalente conversacional de `quorum task list/status`: nunca modifica artefactos, solo lee `.ai/tasks/` y reporta qué falta y qué skill o comando viene a continuación. Útil al volver a una tarea después de un rato o al diagnosticar un paso saltado.

### Sobre `quorum task run`

El comando existe en el CLI, pero `task_manager.run_task()` es stub: el dispatcher automático está diferido. **Hoy el flujo real lo conducís vos** alternando entre los comandos del orquestador y los despachos de skills, como en la tabla de arriba.

---

## 🧪 Política de testing

```text
Agent loop:  unit tests + lint       objetivo: <60s
Human gate:  BDD / acceptance suite  objetivo: <10min
```

- El agente ejecuta solo `verify.commands` definidos en el contrato.
- El humano ejecuta la suite de aceptación completa antes del merge.
- Ningún reporte reemplaza la validación determinística.

---

Para validar el propio framework:

```bash
uv run pytest -v
```

---

## 📂 Estructura del sistema

```bash
project/
├── agents                 # wrapper CLI; configura PYTHONPATH=.agents
├── .agents/
│   ├── cli/               # CLI y helpers core
│   │   └── core/
│   │       ├── task_manager.py
│   │       ├── risk_scorer.py
│   │       └── failure_lookup.py
│   ├── schemas/           # JSON Schemas para YAML/JSON artifacts
│   ├── policies/          # risk.yaml y routing.yaml
│   ├── retrievers/        # import graph / AST neighbors
│   ├── config.yaml        # niveles de modelos y límites
│   └── skills/            # skills q-* y spec-kitty.*
├── .ai/tasks/
│   ├── inbox/
│   ├── active/
│   ├── done/
│   └── failed/
├── docs/adr/              # decisiones arquitectónicas
├── memory/
│   ├── decisions/
│   ├── patterns/
│   └── lessons/
└── worktrees/             # worktrees gitignored por tarea
```

---

## 🧭 Roadmap resumido

Prioridad antes de automatización avanzada:

1. Convertir `task_manager.run_task()` en runtime real.
2. Consolidar escritura consistente de `07-trace.json` durante ejecución.
3. Añadir flujo explícito de review/pre-merge en CLI.
4. Implementar merge-gate determinístico mediante shadow merge + `verify.commands`.
5. Solo con telemetría: evaluar auto-retry, renegociación de contrato o re-blueprint automático.

Rechazado por arquitectura actual:

- post-mortem dedicado `08`;
- impact report `09/10`;
- agente integrador LLM para resolver Git;
- memoria automática sin `q-memory`;
- merge automático a `main`.

---

## ⚖️ Licencia

MIT
