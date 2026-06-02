# Patrón propuesto: `GitRunner` y puertos de filesystem

**Estado:** Idea de refactorización.
**Prioridad:** Media-alta.
**Tipo:** Ports & Adapters mínimo.
**Alcance inicial:** operaciones Git/worktree y filesystem imperativo en `internal/core/task_manager.go`.

---

## 1. Problema actual

Varias funciones llaman directamente a comandos externos y operaciones de filesystem:

- `exec.Command("git", ...)`;
- `os.Rename()`;
- `os.Remove()`;
- `os.Stat()`;
- `os.MkdirAll()`;
- `os.WriteFile()`.

Esto es normal en una CLI, pero genera acoplamiento fuerte entre reglas SDC y efectos externos.

Las zonas más sensibles son:

- creación/remoción de worktrees;
- detección de ramas;
- limpieza de worktrees sucios;
- stash de cambios como patch;
- rollback humano con `quorum task back`;
- retry de failed children.

---

## 2. Patrón sugerido

Crear puertos pequeños para las operaciones repetidas. Por ejemplo:

```go
type GitRunner interface {
    BaseBranch() string
    BranchExists(branch string) bool
    WorktreeAdd(path, branch, base string) error
    WorktreeRemove(path string, force bool) error
    DirtyPaths(worktreePath string) ([]string, error)
    SavePatch(worktreePath, taskID string) (string, error)
    DeleteBranchIfMerged(branch, base string) bool
}
```

Y, si hace falta, un filesystem port mínimo:

```go
type FileSystem interface {
    Exists(path string) bool
    Rename(src, dst string) error
    MkdirAll(path string) error
    Remove(path string) error
}
```

No es necesario abstraer todo el sistema de archivos desde el primer día. El primer puerto valioso es Git.

---

## 3. Beneficios post-refactorización

### 3.1 Tests más rápidos y deterministas

Los tests de guards y transiciones podrían usar un `FakeGitRunner` sin crear repositorios Git reales ni worktrees temporales.

### 3.2 Menos lógica shell mezclada con dominio

Funciones como `CleanTask()` y `BackTask()` podrían concentrarse en decidir qué debe pasar, delegando cómo ejecutar Git al adapter.

### 3.3 Mejor control de errores

Un adapter puede normalizar errores de Git en mensajes estructurados:

- branch inexistente;
- worktree sucio;
- merge-base no resoluble;
- patch vacío;
- comando Git fallido.

### 3.4 Mayor seguridad al tocar rollback y cleanup

Las operaciones destructivas (`worktree remove --force`, borrado de branch, stash de patch) quedan detrás de métodos explícitos, fáciles de auditar.

### 3.5 Base para merge-gate futuro

La idea de validación pre-merge necesitará operaciones Git deterministas. Un `GitRunner` reduce el costo de implementarla sin duplicar comandos shell.

---

## 4. Riesgos y límites

- No simular Git completamente; solo envolver operaciones usadas por Quorum.
- No ocultar decisiones de seguridad. Por ejemplo, `force` debe seguir siendo visible en la firma.
- No introducir mocks globales. Preferir inyección explícita en servicios/transiciones.

---

## 5. Criterio de éxito

La refactorización vale la pena si reduce llamadas directas a `exec.Command("git", ...)` en `task_manager.go` y permite probar `clean`, `back`, `start` y `retry-prepare` con dobles de prueba pequeños.
