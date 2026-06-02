# Propuesta Técnica SDD: `TaskStore` para artefactos y directorios de tarea

**Estado:** Lista para flujo SDD — alimentar `/q-brief` y `/q-blueprint`.
**Prioridad:** Alta.
**Tipo:** Refactorización incremental / Facade ligera.
**Contexto:** Evolución interna de Quorum. Cambio acotado al framework Go; validación final con `go test ./...`.
**Objetivo:** Implementar una fachada `TaskStore` que centralice resolución de tareas, paths de artefactos, carga validada, guardado validado y movimiento de directorios de tarea, extrayendo `MoveTask` como primitivo y migrando **los 8 sitios** de `os.Rename` de movimiento de tarea sin cambiar comportamiento CLI observable.

---

## 1. Problema real

Quorum ya tiene reglas críticas para artefactos:

- `SaveArtifact()` valida antes de escribir.
- `ValidateArtifact()` aplica schemas desde `.agents/schemas`.
- `07-trace.json` debe conservar el invariante append-only mediante `EnsureTraceAppendOnly()`.
- `FindTaskDir()` implementa resolución no trivial de tareas por prioridad:
  1. `task_id` dentro de `00-spec.yaml`;
  2. nombre exacto del directorio;
  3. prefijo `<ID>-`, evitando colisiones parent/child.

El problema es que muchas rutas combinan manualmente:

- `ProjectRoot()`;
- `FindTaskDir()` / `FindTaskDirIn()`;
- `filepath.Join(taskDir.Path, artifactName)`;
- `LoadArtifactPayload()`;
- `ValidateArtifact()`;
- `SaveArtifact()`;
- `os.Rename()` para mover tareas entre `.ai/tasks/{inbox,active,done,failed}`.

Esto hace que cada función de lifecycle tenga que recordar detalles de filesystem y validación que deberían estar encapsulados.

---

## 2. Alcance decidido

### Dentro de alcance

1. Crear `TaskStore` como fachada explícita para tareas.
2. Crear constructores:

   ```go
   func NewTaskStore(projectRoot string) TaskStore
   func DefaultTaskStore() (TaskStore, error)
   ```

3. Implementar métodos:

   ```go
   type TaskStore struct {
       ProjectRoot string
   }

   func (s TaskStore) FindTask(id string, locations ...string) (*TaskDirMatch, error)
   func (s TaskStore) TaskArtifactPath(task *TaskDirMatch, name string) (string, error)
   func (s TaskStore) LoadArtifact(task *TaskDirMatch, name string) (any, error)
   func (s TaskStore) SaveArtifact(task *TaskDirMatch, name string, payload any) (string, error)
   func (s TaskStore) MoveTask(task *TaskDirMatch, targetLocation string) (*TaskDirMatch, error)
   ```

4. Mover helpers de artefactos fuera de `task_manager.go` a:

   ```text
   internal/core/artifact.go
   ```

   Helpers a mover sin cambiar firmas ni comportamiento:

   ```go
   EnsureTraceAppendOnly
   LoadArtifactPayload
   DumpArtifactPayload
   SaveArtifact
   ```

4.1. Extraer un helper nuevo de parseo desde bytes en `internal/core/artifact.go`:

   ```go
   // ParseArtifactPayload decide YAML/JSON por la extensión de path y parsea raw.
   func ParseArtifactPayload(path string, raw []byte) (any, error)
   ```

   Motivo: hoy `LoadArtifactPayload(path)` acopla lectura de disco + parseo por `filepath.Ext(path)` (`task_manager.go:157-173`). Esa unión es la razón por la que `cmd/task_artifact_save.go` escribe un `.tmp` solo para poder parsear stdin con una extensión válida. Tras extraer `ParseArtifactPayload`, redefinir `LoadArtifactPayload` como un delgado wrapper que conserva su firma y comportamiento:

   ```go
   func LoadArtifactPayload(path string) (any, error) {
       raw, err := os.ReadFile(path)
       if err != nil {
           return nil, err
       }
       return ParseArtifactPayload(path, raw)
   }
   ```

   Con esto, `cmd/task_artifact_save.go` parsea stdin sin tocar disco: `ParseArtifactPayload(artifactName, raw)` y luego `store.SaveArtifact(task, artifactName, payload)`. El nombre del artefacto (`artifactName`) aporta la extensión que decide YAML/JSON, eliminando el `.tmp`.

5. Crear:

   ```text
   internal/core/task_store.go
   internal/core/task_store_test.go
   ```

6. Mantener funciones públicas actuales como wrappers compatibles:

   ```go
   FindTaskDir(...)
   FindTaskDirIn(...)
   LoadArtifactPayload(...)
   SaveArtifact(...)
   ValidateArtifact(...)
   ```

   No eliminar ni cambiar sus firmas.

7. Migrar **los 8 sitios de `os.Rename` de movimiento de tarea** en `task_manager.go` para usar `store.MoveTask(...)`. Es el alcance obligatorio completo, no una muestra:

   | # | Sitio | Función | Movimiento |
   |---|-------|---------|------------|
   | 1 | `task_manager.go:288` | `PrepareBlueprint` | inbox → active |
   | 2 | `task_manager.go:601` | `StartTask` | inbox → active |
   | 3 | `task_manager.go:862` | `CleanTask` | active → done |
   | 4 | `task_manager.go:906` | `AutoArchiveParentIfComplete` | active → done (parent) |
   | 5 | `task_manager.go:986` | `BackTask` | active → … |
   | 6 | `task_manager.go:994` | `BackTask` | … → inbox |
   | 7 | `task_manager.go:1483` | `restoreParentForChildRetry` | … → active (parent) |
   | 8 | `task_manager.go:1557` | `PrepareFailedChildRetry` | failed → active |

   **Regla de migración:** en cada sitio se reemplaza **únicamente** el par `filepath.Join(root, ".ai", "tasks", <loc>, filepath.Base(...))` + `os.Rename(...)` por una sola llamada a `store.MoveTask(task, <loc>)`. Los guards de estado, la lógica de worktree/branch, el append de trace de cleanup y los mensajes CLI permanecen INTACTOS en su función. La extracción es del movimiento, no de las reglas que lo rodean.

   Adicionalmente, `cmd/task_artifact_save.go` se migra para usar `DefaultTaskStore()` + `store.FindTask(...)` + `store.SaveArtifact(...)`, eliminando la concatenación cruda de path y la danza manual de `.tmp`.

### Fuera de alcance

- No implementar `ArtifactStore`; solo `TaskStore`.
- No modificar `cmd/report.go`; reportes quedan para la idea `5-patron-report-service.md`.
- No modificar schemas.
- No cambiar lifecycle `00`→`07`.
- No agregar artefactos nuevos.
- No cambiar firmas públicas estilo CLI como:

  ```go
  StartTask(taskID string)
  CleanTask(taskID string, force, stash bool)
  BackTask(taskID string, opts ...bool)
  ```

- No convertir funciones que hoy imprimen a funciones que devuelven error.
- No cambiar comportamiento CLI observable, mensajes esperados o idempotencia existente salvo errores nuevos estrictamente internos ante inputs inválidos del nuevo `TaskStore`.

---

## 3. Diseño detallado

### 3.1 `TaskStore`

`TaskStore` es una fachada local-first sobre filesystem. No es ORM, no es backend intercambiable y no introduce persistencia nueva.

```go
type TaskStore struct {
    ProjectRoot string
}
```

`ProjectRoot` debe ser el root del repositorio/proyecto donde vive `.ai/tasks`.

### 3.2 Constructores

```go
func NewTaskStore(projectRoot string) TaskStore
```

- No debe llamar a `ProjectRoot()`.
- Debe ser ideal para tests con directorios temporales.

```go
func DefaultTaskStore() (TaskStore, error)
```

- Debe llamar a `ProjectRoot()`.
- Si `ProjectRoot()` falla, propaga el error.
- Lo usan rutas productivas que antes llamaban manualmente a `ProjectRoot()`.

### 3.3 `FindTask`

```go
func (s TaskStore) FindTask(id string, locations ...string) (*TaskDirMatch, error)
```

Debe conservar exactamente la semántica de `FindTaskDirIn(s.ProjectRoot, id, locations)`:

- Si `locations` está vacío, buscar en:

  ```text
  inbox, active, done, failed
  ```

- Si no encuentra tarea, devolver:

  ```go
  nil, nil
  ```

- Si hay ambigüedad, devolver el mismo estilo de error `AMBIGUITY ERROR`.
- Debe conservar la prioridad de resolución:
  1. `task_id` dentro de `00-spec.yaml`;
  2. nombre exacto del directorio;
  3. prefijo `<ID>-`.
- Debe conservar protección parent/child:
  - buscar `FEAT-001` no debe matchear `FEAT-001-a` ni `FEAT-001-a-slug` por prefijo.

`FindTaskDir(...)` y `FindTaskDirIn(...)` deben seguir existiendo. Pueden delegar al store, pero no deben cambiar comportamiento.

### 3.4 `TaskArtifactPath`

```go
func (s TaskStore) TaskArtifactPath(task *TaskDirMatch, name string) (string, error)
```

Debe construir el path de un artefacto dentro del directorio de tarea.

Reglas obligatorias:

- `task` no puede ser `nil`.
- `task.Path` no puede estar vacío.
- `name` debe ser un nombre base simple.
- Rechazar:

  ```text
  ""
  "../00-spec.yaml"
  "subdir/file.yaml"
  "subdir\\file.yaml"
  "/tmp/file.yaml"
  "C:\\tmp\\file.yaml"        // en plataformas donde aplique
  "."
  ".."
  ```

- No permitir slash `/`, backslash `\`, path absoluto ni path traversal.
- No restringir a lista cerrada de artefactos conocidos; cualquier nombre base simple es aceptable a nivel de path.
- La validación semántica/schema ocurre en `LoadArtifact`, `SaveArtifact` y `ValidateArtifact`.

Ejemplo válido:

```go
path, err := store.TaskArtifactPath(task, "02-contract.yaml")
```

### 3.5 `LoadArtifact`

```go
func (s TaskStore) LoadArtifact(task *TaskDirMatch, name string) (any, error)
```

Debe:

1. Resolver path con `TaskArtifactPath`.
2. Leer y parsear con `LoadArtifactPayload(path)`.
3. Validar con `ValidateArtifact(path, payload)`.
4. Devolver payload validado.

No se implementa `LoadArtifactRaw` en esta tarea.

Consecuencias:

- Si el archivo no existe, propaga error de lectura.
- Si YAML/JSON es inválido, propaga error de parseo.
- Si no hay schema para ese artefacto, debe fallar mediante el error actual de `ValidateArtifact`, por ejemplo:

  ```text
  artifact=<path>; field=$; reason=unsupported artifact path
  ```

- Si el payload viola schema, debe conservar el formato actual de `ArtifactValidationError`.
- **Regla de Observabilidad vs Transición:** Esta función es exclusiva para lecturas de lifecycle/transición donde necesitamos garantías fuertes del contrato. Las rutas read-only/observabilidad (`ListTasks`, `ShowStatus`, etc.) deben seguir usando `LoadArtifactPayload` para evitar fallar (fail-fast) ante artefactos corruptos o legacy.

### 3.6 `SaveArtifact`

```go
func (s TaskStore) SaveArtifact(task *TaskDirMatch, name string, payload any) (string, error)
```

Debe:

1. Resolver path con `TaskArtifactPath`.
2. Delegar en el helper actual:

   ```go
   SaveArtifact(path, payload)
   ```

3. Devolver la ruta escrita y el error, igual que `SaveArtifact`.

Regla crítica:

- `TaskStore.SaveArtifact` **no debe reimplementar** validación, serialización ni append-only trace.
- `SaveArtifact(...)` sigue siendo la autoridad del write-point validado.

Debe conservar:

- validación antes de escribir;
- no sobrescribir un artefacto válido con payload inválido;
- append-only de `07-trace.json`;
- formato de errores de validación.

### 3.7 `MoveTask`

```go
func (s TaskStore) MoveTask(task *TaskDirMatch, targetLocation string) (*TaskDirMatch, error)
```

Debe mover el directorio completo de una tarea entre ubicaciones lifecycle.

Ubicaciones válidas:

```text
inbox
active
done
failed
```

Cualquier otro valor debe fallar con error explícito.

Reglas obligatorias:

- `task` no puede ser `nil`.
- `task.Path` no puede estar vacío.
- `task.Location` no puede estar vacío.
- `targetLocation` debe ser una de las cuatro ubicaciones válidas.
- Si `task.Location == targetLocation`, es no-op idempotente:

  ```go
  return &TaskDirMatch{Path: task.Path, Location: task.Location}, nil
  ```

- El no-op no debe mutar el `task` original.
- Si el destino ya existe, fallar **antes** de llamar a `os.Rename`.
- No sobrescribir ni eliminar el destino.
- Debe crear el directorio padre del destino si hace falta.
- Debe usar el mismo basename del directorio original.
- Debe devolver un **nuevo** `TaskDirMatch` actualizado, sin mutar el puntero original.

Ejemplo:

```go
moved, err := store.MoveTask(task, "done")
// moved.Path == <root>/.ai/tasks/done/<basename>
// moved.Location == "done"
// task.Location conserva su valor anterior
```

#### `MoveTask` es un primitivo TONTO (contrato no negociable)

`MoveTask` se limita a las reglas de arriba: resolver root, construir destino, crear el padre, renombrar y devolver un match refrescado. **NO** debe absorber guards de estado, efectos de worktree/branch, append de trace de cleanup ni reglas constitucionales. Esa lógica vive en las funciones que lo invocan y, a futuro, en las transiciones explícitas de `4-patron-transiciones-lifecycle.md`, que consumen `TaskStore` desde su `TransitionContext`.

Si `MoveTask` tragara guards: (1) violaría SRP y se volvería un cajón de sastre; (2) colisionaría con la idea #4, que se quedaría sin un ladrillo limpio sobre el que montar `Guard`/`Effect`. **Por eso el orden #2 → #4 es obligatorio:** esta tarea entrega el primitivo de movimiento sin guards; #4 construye las transiciones guardadas encima.

> **Nota de comportamiento:** los 8 sitios actuales mezclan de forma inconsistente la construcción del destino, `MkdirAll`, chequeos de existencia y `os.Rename`. Los 8 ya hacen `MkdirAll` del padre; solo `restoreParentForChildRetry` (`:1483`) chequea explícitamente que el destino no exista (`os.Stat`) antes de renombrar. `MoveTask` centraliza esos detalles: **siempre** crea el directorio padre y **siempre** falla explícitamente si el destino ya existe antes de renombrar. El único cambio de comportamiento es que los 7 sitios que hoy NO chequean destino pasan a fallar de forma explícita y temprana en vez de depender del error de `os.Rename`; el chequeo manual de `:1483` queda redundante y se elimina al migrar.

---

## 4. Migración requerida (los 8 sitios)

La implementación debe reemplazar los 8 `os.Rename` de movimiento de tarea (tabla de §2, item 7) por `store.MoveTask(...)`, más migrar `cmd/task_artifact_save.go`. En cada sitio se sustituye solo el snippet de movimiento; los guards y efectos circundantes no se tocan. A continuación, el detalle de los sitios con lógica circundante relevante.

#### 4.1 `PrepareBlueprint`

Situación actual típica:

- busca tarea en `inbox`;
- si no existe, busca en `active`;
- calcula `activePath` manualmente;
- mueve con `os.Rename`.

Objetivo:

- usar `DefaultTaskStore()`;
- usar `store.FindTask(...)`;
- usar `store.MoveTask(task, "active")`.

Debe conservar comportamiento:

- si ya está en `active`, imprime mensaje actual y retorna path active;
- si no está en `inbox`, error `Task <ID> not found in inbox.`;
- no cambiar mensajes CLI salvo que el error interno de store sea propagado en casos nuevos.

#### 4.2 `StartTask`

Situación actual típica:

- busca tarea en `active` o `inbox`;
- construye path de `02-contract.yaml`;
- carga y valida contrato manualmente;
- si está en `inbox`, mueve a `active` con `os.Rename`;
- crea `04-implementation-log.yaml` y `07-trace.json` con `SaveArtifact`.

Objetivo:

- usar `store.FindTask(...)`;
- usar `store.TaskArtifactPath(...)` para checks de existencia;
- usar `store.LoadArtifact(...)` para cargar contrato validado;
- usar `store.MoveTask(..., "active")` cuando aplique;
- usar `store.SaveArtifact(...)` para log y trace cuando aplique.

Debe conservar comportamiento CLI observable.

#### 4.3 `PrepareFailedChildRetry`

Situación actual típica:

- busca en `active` o `failed`;
- construye paths de `00-spec.yaml` y `07-trace.json`;
- carga y valida manualmente;
- calcula `activeTarget`;
- mueve con `os.Rename`.

Objetivo:

- usar `store.FindTask(...)`;
- usar `store.LoadArtifact(...)` para `00-spec.yaml` y `07-trace.json`;
- usar `store.MoveTask(..., "active")` para restaurar.

Debe conservar guards:

- retry solo para child con `parent_task`;
- requiere `07-trace.json` válido;
- no clobber de copia activa existente;
- clear de artifacts stale sigue funcionando.

#### 4.4 `InitializeSpecify`

Situación actual típica:

- busca tarea existente;
- calcula path inbox manualmente;
- guarda `00-spec.yaml`.

Objetivo:

- usar `store.FindTask(...)`;
- puede seguir creando el directorio inicial manualmente porque todavía no hay `TaskDirMatch` previo;
- una vez creado el `TaskDirMatch`, usar `store.SaveArtifact(...)` para `00-spec.yaml` si resulta más claro.

#### 4.5 `cmd/task_artifact_save.go`

Situación actual típica:

- llama `core.FindTaskDir(taskID, nil)`;
- construye `destPath` con concatenación cruda de string;
- escribe un `.tmp` solo para que `LoadArtifactPayload(path)` decida YAML/JSON por extensión;
- llama `core.SaveArtifact(destPath, payload)`.

Objetivo:

- usar `core.DefaultTaskStore()`;
- usar `store.FindTask(...)`;
- parsear stdin con `core.ParseArtifactPayload(artifactName, raw)` (helper de §2, item 4.1), **eliminando el `.tmp`**;
- persistir con `store.SaveArtifact(task, artifactName, payload)`;
- rechazar relpaths peligrosos mediante `TaskArtifactPath(...)`;
- conservar validación-before-write.

#### 4.6 `CleanTask` (active → done)

Mueve la tarea a `done` tras limpiar worktree/branch. Objetivo: reemplazar solo el `os.Rename` de `:862` por `store.MoveTask(task, "done")`.

Debe conservar INTACTO: el guard `taskDir.Location == "active"`, la remoción de worktree, el borrado de branch si aplica, el chequeo de children done para parents y los mensajes CLI. `MoveTask` NO absorbe nada de esto.

#### 4.7 `AutoArchiveParentIfComplete` (active → done)

Archiva el parent cuando todos sus children están en `done`. Objetivo: reemplazar el `os.Rename` de `:906` por `store.MoveTask(parentDir, "done")`.

Debe conservar el guard de "todos los children en done" y la verificación de que el parent esté en `active`.

#### 4.8 `BackTask` (rollback humano)

Tiene **dos** sitios de movimiento (`:986` y `:994`) según el estado previo. Objetivo: reemplazar ambos `os.Rename` por `store.MoveTask(task, <loc>)` con el destino correspondiente.

Debe conservar INTACTO: que es comando exclusivamente humano, la remoción de worktree, el manejo de branch y la lógica que decide el estado anterior. `MoveTask` solo ejecuta el movimiento ya decidido.

#### 4.9 `restoreParentForChildRetry` (… → active)

Restaura el parent a `active` durante el retry de un child fallido. Objetivo: reemplazar el `os.Rename` de `:1483` por `store.MoveTask(parentDir, "active")`.

Debe conservar la condición de que solo aplica durante el retry autorizado de un child.

### Ruta de artefacto (no es sitio de movimiento)

`InitializeSpecify` (§4.4) no contiene `os.Rename`: crea el directorio inicial en `inbox` y guarda `00-spec.yaml`. No cuenta entre los 8, pero se migra su `SaveArtifact` a `store.SaveArtifact(...)` una vez creado el `TaskDirMatch`, por coherencia.

**Límite:** la migración toca exclusivamente el snippet de movimiento y el de guardado en estos sitios. No se debe refactorizar la lógica de Git/worktrees ni convertir esta tarea en el rediseño de transiciones (eso es la idea #4).

---

## 5. Edge cases obligatorios

### 5.1 Path safety

Tests obligatorios para `TaskArtifactPath`:

Debe aceptar:

```text
00-spec.yaml
feedback.json
custom.yaml
```

Debe rechazar:

```text
""
"."
".."
"../00-spec.yaml"
"subdir/file.yaml"
"subdir\\file.yaml"
"/tmp/file.yaml"
```

En Windows o lógica portable, también debe rechazar paths absolutos con volumen si aplica.

### 5.2 Carga validada

Tests obligatorios para `LoadArtifact`:

- Carga un `00-spec.yaml` válido y devuelve payload.
- Falla con artifact inválido según schema.
- Falla con nombre simple sin schema conocido, por ejemplo `notes.yaml`, usando el error actual `unsupported artifact path`.
- Falla si el archivo no existe.

Tests obligatorios para `ParseArtifactPayload` (helper nuevo):

- Parsea bytes con extensión `.json` como JSON y con `.yaml`/otra como YAML, igual que `LoadArtifactPayload` decidía por path.
- Propaga error de parseo ante JSON/YAML inválido.
- Regresión: `LoadArtifactPayload(path)` reescrito como wrapper conserva exactamente su firma y comportamiento (lee disco y delega en `ParseArtifactPayload`).

### 5.3 Guardado validado

Tests obligatorios para `TaskStore.SaveArtifact`:

- Guarda artefacto válido.
- Rechaza payload inválido.
- No sobrescribe un artefacto válido existente con payload inválido.
- Conserva append-only de `07-trace.json`:
  - permite append de intento nuevo;
  - rechaza eliminación de intentos;
  - rechaza mutación/reordenamiento de intentos existentes.

### 5.4 Movimiento de tareas

Tests obligatorios para `MoveTask`:

- Mueve `inbox -> active` y devuelve nuevo `TaskDirMatch` con path/location actualizados.
- No muta el `TaskDirMatch` original.
- `active -> active` es no-op idempotente.
- Falla si `targetLocation` no es `inbox|active|done|failed`.
- Falla si el destino ya existe antes de llamar a `os.Rename`.
- Crea el directorio padre del destino si no existe.

### 5.5 Resolución de tareas

Tests obligatorios para `FindTask` o wrappers compatibles:

- Conserva prioridad por `task_id` en `00-spec.yaml` sobre nombre de directorio.
- Conserva match por nombre exacto.
- Conserva match por prefijo `<ID>-`.
- Conserva error de ambigüedad cuando múltiples tareas matchean en la misma prioridad.
- Conserva protección parent/child:
  - `FEAT-001` no matchea `FEAT-001-a` ni `FEAT-001-a-slug` por prefijo.
- No encontrado devuelve `nil, nil`.

### 5.6 Wrappers existentes

Tests existentes de `FindTaskDirIn`, `SaveArtifact`, `ValidateArtifact` y append-only trace deben seguir pasando sin cambios de expectativa.

---

## 6. Invariantes y restricciones

1. **No cambiar schemas.**
2. **No cambiar lifecycle `00`→`07`.**
3. **No agregar numbered artifacts.**
4. **No modificar `cmd/report.go`.**
5. **No implementar `ArtifactStore`.**
6. **No eliminar helpers públicos existentes.**
7. **No cambiar firmas públicas existentes salvo añadir nuevas APIs.**
8. **No cambiar comportamiento CLI observable.**
9. **No reimplementar validación dentro de `TaskStore.SaveArtifact`.**
10. **No relajar append-only trace.**
11. **No permitir path traversal en artefactos.**
12. **No permitir `MoveTask` fuera de `inbox|active|done|failed`.**
13. **No sobrescribir directorios destino al mover tareas.**
14. **No convertir esta tarea en refactor completo de Git/worktrees.** Esa preocupación pertenece a `3-patron-gitrunner-filesystem-ports.md`.
15. **Rutas de observabilidad vs transición:** Las rutas read-only/observabilidad (`ListTasks`, `ShowStatus`, etc.) DEBEN seguir usando el primitivo `LoadArtifactPayload` (no-validante). Un comando de observabilidad nunca debe abortar (crash) al leer un artefacto legacy o corrupto; su trabajo es reportarlo.

---

## 7. Archivos esperados

### Crear

```text
internal/core/artifact.go
internal/core/task_store.go
internal/core/task_store_test.go
```

### Modificar

```text
internal/core/task_manager.go
internal/core/task_manager_test.go        // si conviene mover/ajustar tests existentes
cmd/task_artifact_save.go                 // dentro de alcance
```

### No modificar

```text
cmd/report.go
.agents/schemas/*
quorum.md
lifecycle artifact schemas
```

---

## 8. Criterios de aceptación

La implementación se acepta si cumple todo lo siguiente:

1. Existe `TaskStore` con los métodos y constructores definidos.
2. No existe un nuevo tipo `ArtifactStore`.
3. Helpers de artifact storage viven fuera de `task_manager.go`, preferentemente en `internal/core/artifact.go`.
4. Las funciones públicas existentes siguen disponibles con las mismas firmas.
5. `TaskStore.LoadArtifact` lee y valida; no existe `LoadArtifactRaw` en esta tarea.
6. `TaskStore.SaveArtifact` delega en `SaveArtifact` y devuelve `(string, error)`.
7. `TaskArtifactPath` rechaza path traversal y solo acepta nombres base.
8. `MoveTask` acepta solo `inbox|active|done|failed`, no clobber, no-op idempotente en mismo estado y devuelve nuevo match sin mutar original.
9. Los 8 sitios de `os.Rename` de movimiento de tarea (tabla de §2, item 7) usan `store.MoveTask(...)`; no queda ningún `os.Rename` de movimiento de directorio de tarea en `task_manager.go`.
10. `cmd/task_artifact_save.go` usa `TaskStore` (sin concatenación cruda de path ni archivo `.tmp` manual).
11. No cambia comportamiento CLI observable.
12. No cambia formato de errores de validación.
13. No cambia append-only trace.
14. Todos los edge cases de §5 tienen tests.
15. `MoveTask` no contiene guards de estado ni efectos de worktree/branch; esa lógica permanece en las funciones llamadoras.
16. Pasa:

    ```bash
    go test ./...
    ```

---

## 9. Notas para `/q-brief`

**Qué:** Introducir `TaskStore` como fachada ligera para resolver tareas, construir paths seguros de artefactos, cargar artefactos validados, guardar artefactos mediante el write-point validado existente y mover tareas entre ubicaciones lifecycle.

**Por qué:** Reducir dispersión operacional de `ProjectRoot`, `FindTaskDir`, `filepath.Join`, `LoadArtifactPayload`, `ValidateArtifact`, `SaveArtifact` y `os.Rename`, disminuyendo riesgo de saltarse validación o romper invariantes de tarea.

**Dónde:** `internal/core/task_store.go`, `internal/core/artifact.go`, `internal/core/task_manager.go` (los 8 sitios de `os.Rename`), tests en `internal/core/task_store_test.go`, y `cmd/task_artifact_save.go`.

**Validación:** `go test ./...` más tests específicos de path safety, carga validada, guardado validado, append-only trace, movimiento seguro y compatibilidad de resolución de tareas.

**Riesgo:** Medio. Es refactor interno sobre rutas de lifecycle. Mitigación: mantener wrappers existentes, no cambiar CLI, no cambiar schemas, reemplazar solo el snippet de movimiento en los 8 sitios dejando los guards intactos, mantener `MoveTask` como primitivo sin guards (contrato con la idea #4) y conservar tests existentes.
