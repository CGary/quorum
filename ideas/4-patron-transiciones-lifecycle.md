# Patrón propuesto: transiciones explícitas del lifecycle

**Estado:** Idea de refactorización.
**Prioridad:** Alta.
**Tipo:** State pattern funcional / Transition objects.
**Alcance inicial:** comandos `quorum task blueprint`, `start`, `clean`, `back`, `retry-prepare`.

---

## 1. Problema actual

Quorum ya funciona como una máquina de estados:

```text
inbox -> active -> done
              \-> failed
failed child -> active  // retry autorizado
```

Pero esa máquina está implementada de forma implícita dentro de funciones grandes como:

- `PrepareBlueprint()`;
- `StartTask()`;
- `CleanTask()`;
- `BackTask()`;
- `PrepareFailedChildRetry()`;
- `AutoArchiveParentIfComplete()`.

Cada función contiene una mezcla de:

- guards de estado;
- validación de artefactos;
- efectos sobre filesystem;
- efectos sobre Git/worktrees;
- mensajes al usuario;
- reglas constitucionales.

---

## 2. Patrón sugerido

Modelar cada transición como una operación explícita con guards y efectos:

```go
type TransitionContext struct {
    TaskID string
    Store  TaskStore
    Git    GitRunner
}

type TaskTransition struct {
    Name   string
    From   []string
    To     string
    Guard  func(TransitionContext) error
    Effect func(TransitionContext) error
}
```

Ejemplos conceptuales:

| Transición | From | To | Guard clave |
|---|---|---|---|
| `blueprint` | `inbox` | `active` | task existe en inbox |
| `start` | `active/inbox` | `active` | existe `02-contract.yaml` válido |
| `clean` | `active` | `done` | worktree limpio o `--force/--stash`; children done si es parent |
| `auto-archive-parent` | `active` | `done` | parent con todos los children en `done/`; **única transición iniciada por el sistema** (la dispara el `clean` del último hijo — `AutoArchiveParentIfComplete`) |
| `retry-prepare` | `failed` | `active` | solo child con `parent_task` y `07-trace.json` válido |

### 2.1 `back` queda FUERA de la tabla: es un dispatcher inverso

`BackTask()` (`task_manager.go:843`) no es una transición declarable: no tiene `From`/`To` fijos. Infiere qué revertir observando el estado actual — si existe `worktrees/<ID>/` revierte `start`; si la tarea está en `done/` o `failed/` revierte `clean`. Forzarlo dentro de `TaskTransition{From []string, To string}` exigiría enumerar combinaciones de estado o degradar el struct hasta vaciarlo de significado.

Decisión: `back` se modela como **operación de reversión** aparte, que consulta la tabla de transiciones para localizar la última transición aplicable y ejecutar su inverso. Sigue siendo humano-only (la tabla no lo expone a skills) y conserva su semántica actual de inspección de estado.

---

## 3. Beneficios post-refactorización

### 3.1 Lifecycle auditable en código

Las transiciones autorizadas quedarían visibles en una tabla o conjunto pequeño de funciones. Esto ayuda a verificar que el código sigue lo definido en `quorum.md`.

### 3.2 Menos riesgo de transiciones ilegales

Reglas como “retry solo para failed children” o “rollback es humano” se pueden expresar como guards reutilizables y testeables.

### 3.3 Mejor manejo de errores

Cada transición puede distinguir claramente:

- error de precondición;
- error de validación;
- error de Git/worktree;
- error de persistencia.

Esto produce mensajes CLI más consistentes.

### 3.4 Refactorización sin nuevos artefactos

El patrón no agrega `08`, `09`, ni ningún archivo nuevo de lifecycle. Solo hace explícita la máquina de estados que ya existe.

### 3.5 Facilita futuras extensiones seguras

Si más adelante se agrega una validación pre-merge o un dispatcher real, podrá invocar transiciones declaradas en lugar de duplicar lógica de movimiento de tareas.

---

## 4. Riesgos y límites

- No introducir una framework interna compleja.
- No hacer que las transiciones auto-activen skills; eso violaría la regla de single-phase.
- No convertir el lifecycle en configuración editable por usuarios. La constitución manda.

---

## 5. Criterio de éxito

La refactorización es exitosa si las reglas de estado se pueden leer sin recorrer 1600 líneas de `task_manager.go`, y si `go test ./...` confirma que el CLI conserva el comportamiento actual.
