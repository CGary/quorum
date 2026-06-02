# Patrón propuesto: `ArtifactStore` / `TaskStore`

**Estado:** Idea de refactorización.
**Prioridad:** Alta.
**Tipo:** Facade / Repository ligero.
**Alcance inicial:** `internal/core/task_manager.go`, `internal/core/schema.go`, comandos que leen o escriben artefactos.

---

## 1. Problema actual

Quorum ya tiene una regla crítica: todo artefacto persistido debe pasar por `SaveArtifact()` para validar schema y preservar invariantes especiales como el carácter append-only de `07-trace.json`.

El problema no es la regla, sino su dispersión operacional. Muchas rutas combinan manualmente:

- resolución de raíz de proyecto;
- búsqueda de directorios de tarea;
- construcción de rutas con `filepath.Join`;
- lectura con `LoadArtifactPayload()`;
- validación con `ValidateArtifact()`;
- escritura con `SaveArtifact()`;
- movimientos entre `.ai/tasks/{inbox,active,done,failed}`.

Esto hace que cada nueva función de lifecycle tenga que recordar demasiados detalles de infraestructura.

---

## 2. Patrón sugerido

Introducir una fachada pequeña, no una capa de persistencia compleja:

```go
type ArtifactStore struct {
    ProjectRoot string
}

type TaskStore struct {
    Artifacts ArtifactStore
}
```

Responsabilidades deseadas:

```go
func (s TaskStore) FindTask(id string, locations ...string) (*TaskDirMatch, error)
func (s TaskStore) TaskArtifactPath(task *TaskDirMatch, name string) string
func (s TaskStore) LoadArtifact(task *TaskDirMatch, name string) (any, error)
func (s TaskStore) SaveArtifact(task *TaskDirMatch, name string, payload any) error
func (s TaskStore) MoveTask(task *TaskDirMatch, targetLocation string) (*TaskDirMatch, error)
```

La fachada **no** debe reemplazar `SaveArtifact()`. Debe envolverlo para que el write-point validado siga siendo la autoridad.

---

## 3. Beneficios post-refactorización

### 3.1 Menos riesgo de saltarse validación

Al tener métodos de escritura únicos para artefactos de tarea, disminuye la probabilidad de introducir una ruta futura que escriba YAML/JSON directamente con `os.WriteFile()`.

### 3.2 Código de lifecycle más legible

Una transición podría leerse como:

```go
task, _ := store.FindTask(taskID, "inbox")
spec, _ := store.LoadArtifact(task, "00-spec.yaml")
store.MoveTask(task, "active")
```

En vez de mezclar reglas de negocio con detalles de paths.

### 3.3 Tests más simples

Los tests pueden inicializar un `TaskStore` apuntando a un root temporal y verificar operaciones de alto nivel sin repetir setup de rutas internas.

### 3.4 Mejor separación entre regla SDC y filesystem

La regla “un artefacto debe validarse antes de escribirse” queda encapsulada como comportamiento de dominio, no como convención que cada comando debe recordar.

### 3.5 Refactorización incremental segura

Puede introducirse sin cambiar schemas, CLI, ni lifecycle. Primero se crea la fachada y luego se migran funciones una por una.

---

## 4. Riesgos y límites

- No convertirlo en ORM ni abstracción genérica de storage.
- No permitir múltiples backends; Quorum es local-first sobre filesystem.
- No ocultar demasiado los nombres `00-spec.yaml`, `01-blueprint.yaml`, etc.; son parte del contrato constitucional.

---

## 5. Criterio de éxito

La refactorización vale la pena si reduce llamadas directas a `ProjectRoot()`, `FindTaskDir()`, `LoadArtifactPayload()` y `SaveArtifact()` dentro de funciones grandes de `task_manager.go`, sin cambiar el comportamiento observable de `go test ./...` ni del CLI.
