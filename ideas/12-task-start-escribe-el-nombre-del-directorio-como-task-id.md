# 🐞 Defecto: `quorum task start` escribe el nombre del directorio como `task_id`

**Estado:** Diagnosticado con evidencia, sin implementar.
**Contexto:** Quorum, `internal/core/task_transition.go` y `internal/core/task_manager.go`.
**Origen:** Detectado el 2026-09-02 en el repositorio `hexcell`, tarea HEX-060, durante la fase de blueprint.
**Veredicto:** `start` hace eco del argumento del CLI en un campo que debe llevar el identificador canónico de la tarea. El resultado es que **la inicialización de `04-implementation-log.yaml` y `07-trace.json` se salta en silencio** en toda tarea creada por `quorum task specify`.

---

## 1. El síntoma observado

```
$ quorum task specify HEX-060
[+] Task directory created: .ai/tasks/inbox/HEX-060-new-spec

$ quorum task start HEX-060-new-spec
# crea el worktree y la rama ai/HEX-060-new-spec, y despues falla:
# el task_id "HEX-060-new-spec" no cumple el patron del esquema
```

El worktree y la rama SÍ se crean. Lo que no se crea es `04-implementation-log.yaml` ni `07-trace.json`. El fallo se reporta, pero la transición ya dejó el estado a medias: hay rama, hay worktree, y faltan dos de los cuatro artefactos que el resto del pipeline da por hechos.

## 2. La causa raíz

Tres hechos que se contradicen entre sí:

1. `internal/core/task_manager.go:155` — `specify` crea el directorio con sufijo, **a propósito**:
   ```go
   dirPath := filepath.Join(store.ProjectRoot, ".ai", "tasks", "inbox", taskID+"-new-spec")
   ```

2. `internal/core/task_query.go:14-15` — el esquema exige que `task_id` sea el identificador canónico:
   ```go
   taskIDParentRE = regexp.MustCompile(`^[A-Z]+-[0-9]+$`)
   taskIDChildRE  = regexp.MustCompile(`^[A-Z]+-[0-9]+-[a-z]$`)
   ```

3. `internal/core/task_transition.go:234-245` — `initializeImplementationLog` escribe en ese campo **el argumento del CLI**, que es el nombre del directorio:
   ```go
   log := map[string]any{
       "task_id": ctx.TaskID,   // <- "HEX-060-new-spec", no "HEX-060"
       ...
   }
   ```
   `initializeTrace` (línea 258 en adelante) tiene la misma forma.

El nombre del directorio y el `task_id` son deliberadamente distintos, y el propio código lo sabe: `internal/core/task_store_test.go:223` construye el caso `mkSpec(t, root, "inbox", "FEAT-003-a-new-spec", "FEAT-003-a")` y afirma que el store resuelve la pareja correctamente. Es decir, **la capa de almacenamiento ya soporta directorio ≠ identificador; la capa de transición lo ignora.**

## 3. Por qué importa más de lo que parece

* Afecta a **toda** tarea creada con `quorum task specify`, porque el sufijo `-new-spec` no es una excepción sino la regla del comando.
* Falla **en silencio para el pipeline**: el mensaje de error se emite, pero el worktree y la rama ya existen, así que un orquestador que solo comprueba "¿se creó el worktree?" continúa con dos artefactos ausentes.
* Rompe el contrato del rastro append-only antes de que exista: si `07-trace.json` no se inicializa, el primer evento que alguien quiera anexar no tiene dónde ir, y la reconstrucción manual del archivo elude por completo la garantía de append-only que `task_store.go` defiende con cuatro comprobaciones (`task_store_test.go:189-202`).

## 4. La corrección propuesta

`ctx.TaskID` es un **localizador** (cómo encuentro la tarea), no una **identidad** (cómo se llama la tarea). No deben ser el mismo valor.

En `initializeImplementationLog` e `initializeTrace`, tomar el identificador del `00-spec.yaml` ya cargado en el contexto, en lugar del argumento del CLI. El spec es la única fuente de verdad del `task_id`: es lo que el humano escribió y lo que el esquema valida.

Coste estimado: dos líneas, más una prueba que cubra el caso `directorio ≠ task_id` en la transición `start` — el mismo caso que `task_store_test.go` ya cubre para el store y que la transición no hereda.

## 5. Alternativa descartada

*Que `specify` cree el directorio sin sufijo.* Eliminaría la discrepancia de raíz, pero el sufijo `-new-spec` está deliberadamente puesto y probado en tres tests (`task_manager_test.go:153,193`, `task_store_test.go:223`), lo que sugiere que distingue una tarea recién especificada de una ya trabajada. Quitarlo cambia una convención observable del CLI para arreglar un defecto que vive en otra capa. Se corrige donde está el error, no donde es más cómodo.

## 6. Mitigación temporal en curso

En `hexcell` los dos artefactos se crean a mano con el `task_id` canónico cuando la transición falla. Funciona, pero es exactamente el tipo de compensación artesanal que la idea 11 documenta para otro defecto de este mismo CLI: mientras exista, el pipeline depende de que un humano recuerde hacerla.
