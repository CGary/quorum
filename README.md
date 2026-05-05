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

El ciclo combina dos actores. Saber qué hace cada uno es lo que evita pasos saltados.

| Actor | Responsabilidad | Herramienta |
|---|---|---|
| **Orquestador** (humano o runtime externo) | Crea tareas iniciales, decide qué skill despachar, hace `git merge` a `main`, ejecuta rollbacks (`quorum task back`) | Comandos `quorum task ...` + Git |
| **Skill (AI)** | Produce artefactos (`00`–`07`, memoria) dentro de su fase, y auto-ejecuta UNA transición de estado forward al terminar con éxito (cuando aplica) | Slash commands `/q-*` |

### ⚠️ Auto-transición forward + indicador de espera + idioma (Regla #9 en `quorum.md`)

Para reducir fricción operativa, los skills auto-ejecutan **una sola transición CLI hacia adelante** al cerrar su fase con éxito. Las únicas tres autorizadas:

| Skill | Auto-ejecuta | Efecto |
|---|---|---|
| `/q-brief` | `quorum task blueprint <ID>` | Mueve la tarea de `inbox/` a `active/` |
| `/q-decompose` | `quorum task split <PARENT_ID>` | Materializa hijos en `inbox/` desde el campo `decomposition` |
| `/q-blueprint` | `quorum task start <ID>` | Crea el worktree y la rama `ai/<ID>` |

Los demás skills (`/q-analyze`, `/q-implement`, `/q-verify`, `/q-review`, `/q-accept`, `/q-memory`, `/q-status`) **no** ejecutan transiciones de estado.

Reglas adicionales que cumple cada skill (impuestas en `SKILL.md`):

- **Idioma**: cualquier output al usuario va en **español**, sin importar el idioma del input o de la documentación interna.
- **Indicador de espera**: cada turno que termina esperando una respuesta cierra exactamente con `ESPERANDO RESPUESTA DEL USUARIO...` (mayúsculas, tres puntos) como última línea.
- **Sin fence final**: los ejemplos pueden estar documentados en bloques Markdown, pero el output real del agente no debe dejar un cierre ``` después del indicador; la última línea visible tiene que ser `ESPERANDO RESPUESTA DEL USUARIO...`.
- **Handoff explícito**: el cierre de cada skill enumera los próximos pasos, marcando cada uno como `[Obligatorio]` u `[Opcional]`, e incluye `quorum task back <ID>` como vía de rollback humana.
- **Sin auto-encadenado de skills**: ningún skill activa al siguiente. La activación del próximo skill la decide el orquestador.

Si querés volver a un estado anterior porque algo no quedó bien:

```bash
quorum task back <TASK_ID>
```

Reversa la transición forward más reciente: si hay worktree, lo borra (y borra la rama si está vacía); si no hay worktree y la tarea está en `active/`, la devuelve a `inbox/`; si está en `done/` o `failed/`, la devuelve a `active/`. Es siempre humano: ningún skill ejecuta `back` por su cuenta.

### Secuencia canónica para una tarea simple o no decompuesta (FEAT-001)

| # | Actor | Acción | Comando o skill | Auto-transición | Artefacto / efecto |
|---|---|---|---|---|---|
| 1 | Orquestador | Crear tarea en `inbox/` | `quorum task specify FEAT-001` | — | Directorio + `00-spec.yaml` esqueleto |
| 2 | Skill | Llenar la spec | `/q-brief FEAT-001` | `quorum task blueprint` ✓ | Spec lista; tarea en `active/` |
| 3 | Skill — **opcional** | Decomposition | `/q-decompose FEAT-001` | `quorum task split` (solo si confirma) | Hijos en `inbox/` o continúa single-task |
| 4 | Skill | Blueprint + contrato | `/q-blueprint FEAT-001` | `quorum task start` ✓ | `01`, `02`, `07-trace.json`; worktree creado |
| 5 | Skill — **opcional** | Auditoría | `/q-analyze FEAT-001` | — | Reporte read-only |
| 6 | Skill | Implementar | `/q-implement FEAT-001` | — | Diff committeado + `04-implementation-log.yaml` |
| 7 | Skill | Verificar | `/q-verify FEAT-001` | — | `05-validation.json` |
| 8 | Skill | Revisar | `/q-review FEAT-001` | — | `06-review.json` |
| 9 | Skill | Compuerta de aceptación | `/q-accept FEAT-001` | — | Veredicto `ready` / `not_ready` |
| 10 | Humano | Suite BDD (si el contrato la define) | `<acceptance.bdd_suite>` | — | Pase/fail manual |
| 11 | Humano | Merge | `git checkout main && git merge ai/FEAT-001` | — | Código en `main` |
| 12 | Orquestador | Archivar | `quorum task clean FEAT-001` | — | Tarea en `done/`, worktree borrado |
| 13 | Skill — **opcional** | Memoria | `/q-memory FEAT-001` | — | `memory/{decisions,patterns,lessons}/` |

Comparado con el modelo anterior, `quorum task blueprint` y `quorum task start` **ya no los corre normalmente el orquestador**: `/q-brief` auto-ejecuta el primero y `/q-blueprint` auto-ejecuta el segundo. El orquestador solo los corre manualmente para reparación o recuperación.

### Variante con decomposition (feature grande → N hijos)

Cuando la feature es lo bastante grande como para que un LLM modesto no pueda implementarla en una sola pasada, el orquestador despacha `/q-decompose` después de `/q-brief`. El padre queda como umbrella en `active/` y cada hijo recorre su propio ciclo completo.

| # | Actor | Acción | Resultado |
|---|---|---|---|
| 1 | Orquestador | `quorum task specify FEAT-001` | Padre en `inbox/` |
| 2 | Skill | `/q-brief FEAT-001` | Padre con spec; auto-mueve a `active/` |
| 3 | Skill | `/q-decompose FEAT-001` | Aplica heurística de `.agents/policies/decomposition.yaml`, propone N hijos, espera confirmación, persiste `decomposition` en el spec del padre, auto-corre `quorum task split FEAT-001` → crea `FEAT-001-a`, `FEAT-001-b`, ... en `inbox/` con `parent_task: FEAT-001` y `depends_on` |
| 4 | Orquestador | Para cada hijo, en orden topológico de `depends_on`: | |
| 4a | Skill | `/q-brief FEAT-001-a` | Refina el spec del hijo (heredó invariantes y aceptación del padre); auto-mueve a `active/` |
| 4b | Skill | `/q-blueprint FEAT-001-a` | Genera 01/02 del hijo y auto-crea worktree `worktrees/FEAT-001-a/` con rama `ai/FEAT-001-a` |
| 4c..h | Skill / Humano | `/q-implement` → `/q-verify` → `/q-review` → `/q-accept` → BDD → merge `ai/FEAT-001-a` a `main` → `quorum task clean FEAT-001-a` → `/q-memory FEAT-001-a` (opcional) | Hijo cerrado |
| 5 | (repetir 4 para `FEAT-001-b`, etc.) | | |
| 6 | Orquestador | Cuando todos los hijos están en `done/`: `quorum task clean FEAT-001` | Padre archivado |

Cada hijo mergea a `main` independientemente cuando está `ready`. No hay rama integradora; las dependencias se respetan sólo a nivel de orden de despacho (`depends_on` lo marca el spec del hijo). El CLI protege al padre: `quorum task clean <PARENT_ID>` falla si algún hijo de `decomposition` no está todavía en `done/`.

### Detalle por fase

#### 1. Crear la tarea — orquestador

```bash
quorum task specify FEAT-001
```

Crea `.ai/tasks/inbox/FEAT-001-<slug>/` con un `00-spec.yaml` esqueleto. **Hacelo antes** de despachar `/q-brief`; el skill no crea la tarea por vos.

#### 2. Especificar — skill `/q-brief`

```text
/q-brief FEAT-001
```

Entrevista al humano y completa `00-spec.yaml` con goal, invariantes, aceptación, no-objetivos y `risk`. Aplica el `Scope Gate`: si la solicitud es trivial, redirige fuera de Quorum.

> Auto-transición: al terminar con éxito, el skill ejecuta `quorum task blueprint FEAT-001` y mueve la tarea a `active/`. Si falló o quedó incompleto, no corre la transición.

#### 3. Decomposition (opcional) — skill `/q-decompose`

```text
/q-decompose FEAT-001
```

Lee el spec del padre, aplica los signals de `.agents/policies/decomposition.yaml` y propone una decomposition concreta en hijos `FEAT-001-a`, `FEAT-001-b`, ... cada uno con `summary`, `depends_on` y herencia de invariantes/aceptación. Pide confirmación humana **antes** de persistir.

> Auto-transición: solo si el humano confirma. Persiste el campo `decomposition` en el spec del padre y corre `quorum task split FEAT-001`. Si el humano dice "no decomponer", no se ejecuta nada y se sigue el flujo single-task.

#### 4. Blueprint + contrato — skill `/q-blueprint`

```text
/q-blueprint FEAT-001
```

Mapea archivos, símbolos y dependencias afectadas; consulta tareas fallidas relacionadas vía `failure_lookup.py`; corre `risk_scorer.py` y registra los eventos en `07-trace.json` sin pisar el riesgo declarado por el humano; genera `01-blueprint.yaml` y `02-contract.yaml`.

> Auto-transición: al terminar con éxito, ejecuta `quorum task start FEAT-001` y crea el worktree + rama.

#### 5. Análisis de consistencia (opcional) — skill `/q-analyze`

```text
/q-analyze FEAT-001
```

**Read-only.** Cruza `00`, `01`, `02` contra schemas y policies. Reporta gaps (aceptación sin test scenarios, archivos del blueprint que no están en `touch`, BDD lento como `verify.commands`, divergencias de risk no reflejadas). No corrige.

#### 6. Implementación — skill `/q-implement`

```text
/q-implement FEAT-001
```

Lee el contrato como autoridad vinculante. Toca solo archivos en `touch`, respeta `forbid`, registra cambios en `04-implementation-log.yaml` y commitea dentro del worktree en `ai/FEAT-001`. Tocar fuera del contrato rechaza la tarea.

> Sin auto-transición. Termina con `DONE` o `BLOCKED`.

#### 7. Verificación — skill `/q-verify`

```text
/q-verify FEAT-001
```

Corre `verify.commands` del contrato dentro del worktree, captura exit codes y output, escribe `05-validation.json` con `overall_result` y `error_category` opcional.

#### 8. Revisión — skill `/q-review`

```text
/q-review FEAT-001
```

Compara el diff contra `00`, `01`, `02`, `05`. Veredicto en `06-review.json`: `approve | revise | reject`. Si `revise`, devuelve `fix_tasks` estructurados.

#### 9. Compuerta de aceptación — skill `/q-accept`

```text
/q-accept FEAT-001
```

Verifica de forma agregada que validation pasó, review aprobó, contrato se cumplió, no hay refactors no pedidos ni violaciones abiertas en trace. Emite `Acceptance: ready` o `not_ready`.

> No corre merge, ni BDD, ni clean. Reporta lo que tiene que hacer el humano.

#### 10–12. BDD + merge + clean — humano + orquestador

```bash
# 10. (si el contrato define bdd_suite, humano corre el comando manualmente)

# 11. Merge humano
git -C worktrees/FEAT-001 log --oneline
git checkout main
git merge ai/FEAT-001

# 12. Archivar
quorum task clean FEAT-001
```

Regla #6: el sistema commitea, nunca mergea. El merge es manual.

#### 13. Memoria (opcional) — skill `/q-memory`

```text
/q-memory FEAT-001
```

Genera entradas en `memory/{decisions,patterns,lessons}/`. La memoria es exclusivamente human-invoked; no hay captura automática.

### Rollback humano: `quorum task back`

```bash
quorum task back FEAT-001
```

Revierte la transición forward más reciente:

| Estado actual de la tarea | Resultado de `back` |
|---|---|
| Worktree existe | Borra worktree + rama (si está vacía). Tarea queda en `active/`. |
| En `active/` sin worktree | Mueve a `inbox/`. |
| En `done/` o `failed/` | Mueve a `active/`. |
| En `inbox/` | Rechaza con mensaje (no hay estado anterior). |

Útil cuando un skill cerró una fase con auto-transición pero el resultado no convence: corres `back`, refinás lo que sea, y volvés a despachar el skill correspondiente.

### Comandos CLI que mutan estado

Estos comandos existen aunque el flujo normal delegue algunas transiciones a los skills:

```bash
quorum task specify FEAT-001      # crea la tarea inicial en inbox/
quorum task blueprint FEAT-001    # mueve inbox/ -> active/ (normalmente lo auto-ejecuta /q-brief)
quorum task split FEAT-001        # materializa hijos desde 00-spec.yaml.decomposition (lo auto-ejecuta /q-decompose)
quorum task start FEAT-001        # crea worktree + rama ai/FEAT-001 (normalmente lo auto-ejecuta /q-blueprint)
quorum task clean FEAT-001        # archiva en done/ y borra worktree; en padres exige todos los hijos en done/
quorum task back FEAT-001         # rollback humano de la última transición forward
```

`quorum task split` es idempotente: si un hijo ya existe, lo salta y crea sólo los faltantes. También valida que el padre esté en `active/`, que no sea ya una tarea hija, que los `child_id` pertenezcan al padre (`FEAT-001-a`), que `depends_on` apunte a hermanos existentes y que no haya ciclos.

### Campos nuevos en `00-spec.yaml`

La decomposition agrega metadata explícita al spec:

```yaml
# En tareas padre (umbrella):
decomposition:
  - child_id: FEAT-001-a
    summary: Slice implementable de forma independiente.
    depends_on: []

# En tareas hijas:
parent_task: FEAT-001
depends_on:
  - FEAT-001-a
```

Los IDs hijos usan un sufijo de una letra (`FEAT-001-a`, `FEAT-001-b`, ...). Los schemas de `00`–`07` aceptan estos IDs para que cada hijo tenga sus propios artefactos, worktree, branch, verify, review y accept.

### Errores comunes y cómo detectarlos

| Síntoma | Causa probable | Diagnóstico |
|---|---|---|
| `/q-brief` cierra sin auto-correr `task blueprint` | El skill detectó la spec incompleta o falló validación | Leé la última respuesta del skill: si terminó en `BLOCKED` o sigue haciendo preguntas, no es un bug. Completá la entrevista. |
| `/q-blueprint` no creó worktree | El skill cerró con `BLOCKED` (contrato inválido o spec inconsistente) | `quorum task status <ID>` muestra worktree Missing y artefactos parciales. Re-despachá `/q-blueprint` después de corregir. |
| `quorum task back` borra commits no mergeados | Comportamiento esperado cuando la rama no se mergeó | Antes de correr `back`, mergeá o guardá los cambios manualmente |
| `/q-decompose` propuso decomposition pero no creó hijos | El humano respondió "no decomponer" o hubo error de schema en el spec del padre | Revisá la última respuesta del skill |
| Hijos en `inbox/` con specs derivados del padre | Lo esperado tras `quorum task split` | Despachá `/q-brief <child>` para refinar cada uno |
| Padre en `active/` con todos los hijos en `done/` | Falta archivar el padre | `quorum task clean <PARENT>` |
| Lecciones no aparecen en `memory/` | `/q-memory` nunca fue invocado | La memoria es manual; no hay auto-captura |

### Utilidades transversales (read-only)

```bash
quorum task list              # resumen de todas las tareas y su estado
quorum task status FEAT-001   # estado, artefactos, worktree, parent_task / decomposition / depends_on
```

```text
/q-status            # vista global con próxima acción recomendada
/q-status FEAT-001   # diagnóstico por tarea
```

### Sobre `quorum task run`

El comando existe en el CLI, pero `task_manager.run_task()` es stub: el dispatcher automático está diferido. Hoy el flujo lo conducís vos despachando skills; las transiciones forward las hacen los skills mismos. La reversión sigue siendo humana vía `quorum task back`.

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
