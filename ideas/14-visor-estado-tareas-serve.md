# 14. Visor read-only de estado de tareas en `quorum serve`

## Veredicto

**Valioso, pero requiere ADR antes de implementación.** La capacidad encaja naturalmente en el visor embebido de `quorum serve`, pero amplía explícitamente una frontera que los ADR existentes dejaron fuera: visualizar artefactos lifecycle (`00`-`07`) desde una interfaz web.

La implementación recomendada es agregar una pestaña **Tasks** al visor actual, respaldada por endpoints HTTP **solo lectura** que escanean `.ai/tasks/{inbox,active,done,failed}` del `root_path` registrado para cada proyecto en SQLite.

## Contexto actual

`quorum serve start` está definido en `cmd/serve.go` y arranca un proceso en background que ejecuta internamente:

```bash
quorum serve --host <host> --port <port>
```

El servidor HTTP real vive en `internal/server/server.go` y hoy expone:

- `GET /api/projects`
- `GET /api/projects/{projectID}/reports`
- `GET /api/projects/{projectID}/reports/{reportID}`
- `GET /api/projects/{projectID}/memories`
- `GET /api/projects/{projectID}/memories/{memoryID}`

La UI embebida vive en:

- `internal/server/web/index.html`
- `internal/server/web/app.js`
- `internal/server/web/style.css`
- `internal/server/web/styles.css`
- `internal/server/embed.go`

La lógica CLI textual para tareas ya existe en `internal/core/task_manager.go`:

- `ListTasks()` imprime un listado por `inbox`, `active`, `done`, `failed`.
- `ShowStatus(taskID)` imprime estado detallado de una tarea.
- `FindTaskDirIn()` resuelve IDs con las reglas constitucionales existentes.
- `DeriveParentState()` calcula `active`, `partial` o `completed` para padres con `decomposition`.

Estas funciones imprimen a stdout; para el servidor conviene extraer lógica pura que devuelva estructuras JSON en vez de parsear texto.

## Restricciones constitucionales

La nueva superficie debe respetar estas reglas:

1. **Solo lectura:** ningún handler puede mover tareas, escribir artefactos, guardar trace, crear worktrees, limpiar tareas ni modificar SQLite.
2. **Git sigue siendo la verdad de código:** el visor informa estado operacional, no emite veredictos de merge ni reemplaza validación.
3. **No nueva fase lifecycle:** no crear artefactos numerados nuevos (`03`, `08`, `09`, `10`).
4. **No bypass de contract:** mostrar archivos/touch/validación no habilita cambios fuera de `02-contract.yaml`.
5. **No exponer escritura accidental:** la UI no debe incluir botones de transición (`back`, `clean`, `retry`, etc.). Como máximo puede mostrar comandos sugeridos para ejecutar manualmente.
6. **No romper append-only trace:** si se muestra `07-trace.json`, debe leerse tal cual o resumirse; nunca reescribirse.
7. **Project isolation:** solo escanear el `root_path` del proyecto seleccionado desde `/api/projects`.
8. **Path safety:** validar IDs y rutas; no aceptar paths arbitrarios ni traversal.

## ADR necesario

ADR 0004 autorizó el visor de reportes y dejó explícitamente fuera la visualización de artefactos lifecycle como `05-validation.json`, `06-review.json` y `07-trace.json`. ADR 0006 repitió esa exclusión para el visor de memoria.

Por tanto, antes de implementar, crear un ADR nuevo, por ejemplo:

```text
docs/adr/0007-visor-estado-tareas-read-only.md
```

Decisión esperada:

- Autorizar `quorum serve` a exponer estado de `.ai/tasks` en modo read-only.
- Prohibir mutaciones de estado desde handlers/UI.
- Definir qué campos lifecycle se pueden mostrar completos y cuáles solo resumidos.
- Mantener `q-status`, `q-verify`, `q-review`, `q-accept`, etc. como rutas operacionales; el visor solo observa.

## Diseño propuesto

### 1. Extraer un query core puro para tareas

Crear un módulo nuevo:

```text
internal/core/task_query.go
internal/core/task_query_test.go
```

Responsabilidad: leer `.ai/tasks` y devolver estructuras serializables.

No debe imprimir a stdout ni mutar estado.

Tipos sugeridos:

```go
type TaskListOptions struct {
    ProjectRoot string
    Location string // optional: inbox|active|done|failed
    Query string    // optional text search in id/summary/goal
    ParentTask string // optional
    Limit int
    Offset int
}

type TaskListResponse struct {
    RootPath string `json:"root_path"`
    Counts TaskLocationCounts `json:"counts"`
    Items []TaskListItem `json:"items"`
}

type TaskLocationCounts struct {
    Inbox int `json:"inbox"`
    Active int `json:"active"`
    Done int `json:"done"`
    Failed int `json:"failed"`
}

type TaskListItem struct {
    ID string `json:"id"`
    Directory string `json:"directory"`
    Location string `json:"location"`
    Summary string `json:"summary"`
    Goal string `json:"goal,omitempty"`
    Risk string `json:"risk,omitempty"`
    ParentTask string `json:"parent_task,omitempty"`
    ParentState string `json:"parent_state,omitempty"`
    Children []TaskChildRef `json:"children,omitempty"`
    Artifacts map[string]bool `json:"artifacts"`
    WorktreePresent bool `json:"worktree_present"`
    UpdatedAt string `json:"updated_at"`
}

type TaskChildRef struct {
    ID string `json:"id"`
    Location string `json:"location"`
    Summary string `json:"summary,omitempty"`
}
```

Detalle:

```go
type TaskDetail struct {
    TaskListItem
    Spec map[string]any `json:"spec,omitempty"`
    Blueprint map[string]any `json:"blueprint,omitempty"`
    Contract TaskContractSummary `json:"contract,omitempty"`
    ImplementationLog map[string]any `json:"implementation_log,omitempty"`
    Validation map[string]any `json:"validation,omitempty"`
    Review map[string]any `json:"review,omitempty"`
    Trace TaskTraceSummary `json:"trace,omitempty"`
    Feedback map[string]any `json:"feedback,omitempty"`
}

type TaskContractSummary struct {
    Summary string `json:"summary"`
    Goal string `json:"goal"`
    Touch []string `json:"touch"`
    VerifyCommands []string `json:"verify_commands"`
}

type TaskTraceSummary struct {
    Summary string `json:"summary"`
    AttemptsCount int `json:"attempts_count"`
    LastAttempt map[string]any `json:"last_attempt,omitempty"`
    TotalCostUSD float64 `json:"total_cost_usd,omitempty"`
}
```

Recomendación: para la primera versión, el detalle puede mostrar `spec`, `blueprint`, `validation`, `review`, `feedback` completos, pero resumir `contract` y `trace` para evitar ruido y reducir riesgo de exponer contexto demasiado grande. Si se decide mostrar JSON completo, que sea detrás de secciones colapsables.

### 2. Reusar reglas existentes de resolución

No duplicar heurísticas frágiles. Reusar/adaptar:

- `readSpecTaskID(dir)` para ID canónico desde `00-spec.yaml`.
- La heurística actual de `ListTasks()` solo como fallback cuando falta spec.
- `FindTaskDirIn(projectRoot, taskID, locations)` para detalle por ID.
- `DeriveParentState(spec)` para padres con `decomposition`.
- `LoadArtifactPayload(path)` para YAML/JSON.
- `ValidateArtifact(path, payload)` solo si se quiere marcar validez de cada artifact; no bloquear el listado entero si un artifact es inválido.

Importante: `DeriveParentState()` actualmente usa `FindTaskDir()` con `ProjectRoot()` dinámico, no un root explícito. Para el servidor multi-proyecto conviene agregar una variante:

```go
DeriveParentStateIn(projectRoot string, spec map[string]any) string
```

Así se evita que el servidor calcule estado de hijos contra el repo actual en vez del `root_path` del proyecto seleccionado.

### 3. Endpoints HTTP

Modificar `internal/server/server.go` para aceptar una tercera subruta:

```go
parts[1] != "reports" && parts[1] != "memories" && parts[1] != "tasks"
```

Endpoints propuestos:

```http
GET /api/projects/{projectID}/tasks
```

Query params:

```text
location=inbox|active|done|failed
q=<texto>
parent_task=<TASK_ID>
limit=<n>
offset=<n>
```

Respuesta:

```json
{
  "root_path": "/path/to/project",
  "counts": {"inbox": 1, "active": 2, "done": 40, "failed": 1},
  "items": [
    {
      "id": "FEAT-001",
      "directory": "FEAT-001-new-spec",
      "location": "active",
      "summary": "Add task viewer",
      "goal": "Expose task lifecycle state read-only in serve UI.",
      "risk": "medium",
      "artifacts": {
        "00-spec.yaml": true,
        "01-blueprint.yaml": true,
        "02-contract.yaml": true,
        "04-implementation-log.yaml": false,
        "05-validation.json": false,
        "06-review.json": false,
        "07-trace.json": true,
        "feedback.json": false
      },
      "worktree_present": true,
      "updated_at": "2026-06-06T16:00:00Z"
    }
  ]
}
```

```http
GET /api/projects/{projectID}/tasks/{taskID}
```

Devuelve detalle enriquecido de una tarea. `taskID` debe validarse como ID de tarea, no como path. Debe usar `FindTaskDirIn(rootPath, taskID, nil)`.

Errores esperados:

- `404` si proyecto no existe o no tiene `root_path`.
- `404` si task no existe.
- `400` si `location`, `limit`, `offset` o `taskID` son inválidos.
- `405` para métodos distintos de `GET`.

### 4. Validación de IDs y paths

Agregar una función core explícita si no existe:

```go
ValidateTaskID(id string) error
```

Debe aceptar:

- `^[A-Z]+-[0-9]+$`
- `^[A-Z]+-[0-9]+-[a-z]$`

Esto coincide con padres e hijos. No aceptar `/`, `\`, `.`, `..`, URL-encoded traversal ni nombres de directorio arbitrarios.

Para rutas internas:

- construir siempre con `filepath.Join(rootPath, ".ai", "tasks", location, dirName)`.
- cuando se resuelva detalle con `FindTaskDirIn`, confirmar que el path resultante queda dentro de `<rootPath>/.ai/tasks` con `filepath.Rel`.

### 5. UI web

Modificar:

- `internal/server/web/index.html`
- `internal/server/web/app.js`
- `internal/server/web/style.css` / `styles.css`
- `internal/server/web_assets_test.go`

Cambios mínimos:

1. Agregar una tercera pestaña:

```html
<button id="tasks-tab" class="view-tab" type="button" role="tab" aria-selected="false">Tasks</button>
```

2. Agregar controles de tarea:

- filtro por ubicación: All, Inbox, Active, Done, Failed
- búsqueda textual

3. Agregar lista:

```html
<div id="task-list" class="report-list hidden">
  <div class="empty-state">Select a project to view tasks</div>
</div>
```

4. Extender `activeView` de `reports|memories` a `reports|memories|tasks`.

5. Añadir funciones:

```js
loadTasks(projectId)
loadTaskDetail(projectId, taskId)
renderTask(task)
```

6. Render de lista:

- pill por location (`inbox`, `active`, `done`, `failed`)
- ID
- summary
- artifacts completados: por ejemplo `5/7 artifacts`
- worktree present/missing
- parent/children si aplica

7. Render de detalle:

- encabezado: ID, location, directory, summary, risk
- checklist de artifacts `00`, `01`, `02`, `04`, `05`, `06`, `07`, `feedback`
- contract touch y verify commands
- validation summary
- review verdict/summary si existe
- trace attempts count y último intento
- children con ubicación si es parent
- parent/dependencies si es child

No agregar botones mutantes. Si se muestran acciones, que sean texto manual, por ejemplo:

```text
Manual CLI: quorum task status FEAT-001
Manual rollback: quorum task back FEAT-001
```

### 6. Tests recomendados

#### Core

Nuevo `internal/core/task_query_test.go`:

- lista tareas por las cuatro ubicaciones.
- cuenta por estado.
- extrae `task_id` desde `00-spec.yaml` antes que directorio.
- fallback a nombre de directorio cuando falta spec.
- parent con `decomposition` muestra children y parent_state.
- child muestra `parent_task` y `depends_on` si existen.
- artifacts map refleja archivos presentes.
- no falla todo el listado por artifact inválido; marca error/summary unreadable.
- path traversal/ID inválido rechazado.

#### Server

Extender `internal/server/server_test.go`:

- `GET /api/projects/proj1/tasks` devuelve listado.
- `GET /api/projects/proj1/tasks?location=active&q=foo` filtra.
- `GET /api/projects/proj1/tasks/FEAT-001` devuelve detalle.
- proyecto sin `root_path` da `404`.
- `location=bad` da `400`.
- `tasks/..%2Fsecret` da `400`.
- métodos no GET dan `405`.

#### UI assets

Extender `internal/server/web_assets_test.go`:

- `id="tasks-tab"`
- `id="task-list"`
- strings `/tasks`, `loadTasks`, `renderTask`.

#### CLI/help

Actualizar `cmd/serve.go` help text y tests si existen:

```text
Start a read-only server for projects, reports, memories, and task state
```

### 7. Riesgos

1. **Scope creep hacia mutación:** evitar botones de `clean`, `back`, `retry-prepare`, `artifact-save`.
2. **Contradicción con ADR previos:** resolver con ADR nuevo antes de implementación.
3. **Exposición excesiva de artifacts:** preferir resumen en primera versión; JSON completo en secciones colapsables si se aprueba.
4. **Multi-project root bug:** no usar `ProjectRoot()` dentro de queries del servidor cuando se está consultando otro proyecto; pasar `rootPath` explícitamente.
5. **Performance:** repos con muchas tareas pueden ser pesados. Incluir `limit`/`offset`, conteos y carga lazy del detalle.
6. **Estado derivado inconsistente:** si un artifact está corrupto, el visor debe mostrar error local y continuar.
7. **Read-only real:** los endpoints deben limitarse a `os.ReadDir`, `os.Stat`, `os.ReadFile`, parsing y validación. No llamar `SaveArtifact`, `MoveTask`, `CleanTask`, `EnsureTraceAppendOnly`, `EnsureMemoryProject`, etc.

## Plan de implementación sugerido

1. Crear ADR 0007 autorizando el visor read-only de `.ai/tasks`.
2. Crear `internal/core/task_query.go` con estructuras JSON y tests.
3. Refactorizar `ListTasks()`/`ShowStatus()` opcionalmente para consumir `task_query.go`, sin cambiar salida CLI.
4. Agregar handlers `/tasks` en `internal/server/server.go`.
5. Agregar tests server para lista/detalle/errores.
6. Agregar pestaña Tasks en UI embebida.
7. Actualizar assets tests.
8. Actualizar help text de `quorum serve`.
9. Ejecutar:

```bash
go test ./...
```

## Archivos que probablemente se tocarían

```text
docs/adr/0007-visor-estado-tareas-read-only.md
internal/core/task_query.go
internal/core/task_query_test.go
internal/core/task_manager.go
internal/server/server.go
internal/server/server_test.go
internal/server/web/index.html
internal/server/web/app.js
internal/server/web/style.css
internal/server/web/styles.css
internal/server/web_assets_test.go
cmd/serve.go
cmd/serve_test.go
```

## Criterio de aceptación propuesto

- Al ejecutar `quorum serve start` y abrir `http://127.0.0.1:8080`, la UI muestra una pestaña **Tasks**.
- Al seleccionar un proyecto, la pestaña lista tareas de `.ai/tasks/{inbox,active,done,failed}`.
- La lista puede filtrarse por ubicación y búsqueda textual.
- Al seleccionar una tarea, se ve su estado detallado: ubicación, artifacts presentes, worktree, resumen de spec/contract/validation/review/trace y relaciones parent/child.
- La API y UI no realizan ninguna mutación en `.ai/tasks`, worktrees ni SQLite.
- Los tests nuevos y existentes pasan con `go test ./...`.
