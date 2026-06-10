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
    WorktreeAdd(path, branch, base string) error  // crea branch nuevo: worktree add -b (StartTask)
    WorktreeAttach(path, branch string) error     // re-adjunta branch existente, sin -b (PrepareFailedChildRetry)
    WorktreeRemove(path string, force bool) error
    DirtyPaths(worktreePath string) ([]string, error)
    SavePatch(worktreePath, taskID string) (string, error)
    DeleteBranchIfMerged(branch, base string) bool     // merge-base --is-ancestor + branch -d (CleanTask)
    ForceDeleteBranchIfEmpty(branch, base string) bool // rev-list --count == 0 + branch -D (BackTask)
}
```

### 2.1 Operaciones reales que la interfaz DEBE distinguir

Verificado contra las ~20 llamadas a `exec.Command("git", ...)` de `task_manager.go`; colapsarlas en menos métodos ocultaría decisiones de seguridad:

- **Dos modos de worktree add.** `StartTask` crea branch nuevo (`worktree add -b`, línea 526); `PrepareFailedChildRetry` re-adjunta uno existente sin `-b` (líneas 1385-1388). Una sola firma `WorktreeAdd(path, branch, base)` no puede expresar ambos: de ahí `WorktreeAttach`.
- **Dos semánticas de borrado de branch.** `CleanTask` borra solo si está mergeado (`merge-base --is-ancestor` + `branch -d`, líneas 669-671); `BackTask` fuerza el borrado solo si la branch no tiene commits propios (`rev-list --count` + `branch -D`, líneas 903-906). Son políticas distintas con riesgos distintos; cada una merece su método explícito.
- **`SavePatch` debe preservar el intent-to-add.** La implementación actual hace `git add -N .` antes de `diff --binary HEAD` (líneas 627-628); sin eso, los archivos nuevos sin trackear desaparecen del patch. Es comportamiento, no estilo: el adapter lo conserva y un test lo fija.

**Fuera del alcance del puerto** (se quedan como llamadas directas): `ProjectRoot()` (`rev-parse --show-toplevel`, es bootstrap previo a cualquier inyección de dependencias) y la lectura de `remote.origin.url` en `quorum_config.go`.

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
