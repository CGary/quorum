# 📦 Propuesta Técnica: versionar `.ai/tasks/done/` y retirar el archivado manual en `kitty-specs/`

**Estado:** Propuesta — lista para implementar, con **una decisión de producto pendiente** (§5.1).
**Contexto:** Quorum v1.2+, comando `quorum init` y su emisión de `.gitignore`.
**Origen:** Auditoría del repositorio `hexcell` (2026-08-14). Se detectó que los artefactos SDD se guardaban **dos veces** y que la copia versionada era **incompleta**.
**Veredicto:** El archivado manual en `kitty-specs/` es una compensación artesanal de una decisión de `quorum init`. Se elimina la causa (la línea `.ai/tasks/done/*` en `.gitignore`) y el síntoma desaparece solo. Coste en código: ~2 líneas borradas + una migración guardada. Coste real: decidir qué se versiona y auditar lo que entra al historial.

---

## 1. El problema

`quorum init` escribe en `.gitignore` (`internal/core/task_manager.go:886-897`):

```go
ignoreEntries := []string{
    "\n# Quorum", "worktrees/",
    ".ai/tasks/active/*", ".ai/tasks/done/*",
    ".ai/tasks/failed/*", ".ai/tasks/inbox/*",
    "!.ai/tasks/active/.gitkeep", "!.ai/tasks/done/.gitkeep", ...
}
```

Consecuencia: **los artefactos de las tareas terminadas nunca entran a git**. `00-spec.yaml`, `01-blueprint.yaml`, `02-contract.yaml`, `05-validation.json`, `06-review.json`, `07-trace.json`, `08-acceptance-report.json` — todo el rastro SDD — vive solo en el disco del desarrollador.

En `hexcell` esto se compensó a mano: 23 commits `chore: archivar artefactos de la tarea HEX-0NN en kitty-specs`, copiando cada tarea a una carpeta paralela versionada. El resultado medido:

| Almacén | Tareas | Contenido | Git | Peso |
|---|---|---|---|---|
| `.ai/tasks/done/` | 26 | completo | ignorado | 6,4 M |
| `kitty-specs/` | 26 | **incompleto** | versionado | 3,8 M |

La copia pierde datos de forma sistemática (verificado en HEX-001, HEX-012, HEX-023):

```
Only in .ai/tasks/done/HEX-001-new-spec: 08-acceptance-report.json
Only in .ai/tasks/done/HEX-012-new-spec: dispatch
```

Es el peor de los dos mundos: se paga el coste de duplicar y el historial **igual** no contiene la verdad completa. Además `kitty-specs/` no está declarada en ninguna parte — cero referencias fuera de sí misma en todo `hexcell`, y **nada en el código de Quorum la menciona ni la escribe**. Es una convención oral.

---

## 2. Alcance A — proyectos nuevos

### 2.1 Cambio en `ignoreEntries`

Borrar dos entradas de `internal/core/task_manager.go:886-897`:

```diff
     ".ai/tasks/active/*",
-    ".ai/tasks/done/*",
     ".ai/tasks/failed/*",
     ".ai/tasks/inbox/*",
     "!.ai/tasks/active/.gitkeep",
-    "!.ai/tasks/done/.gitkeep",
```

`!.ai/tasks/done/.gitkeep` se va con ella: una negación sin exclusión previa es ruido. El fichero `.gitkeep` **se sigue creando** (mantiene el directorio en git cuando aún no hay tareas); solo desaparece la regla que lo rescataba.

`active/`, `failed/` e `inbox/` **siguen ignorados**: son estado de trabajo en curso, no historial.

### 2.2 `kitty-specs/` en proyectos nuevos

**No hay nada que implementar.** Ningún código de Quorum crea, lee ni escribe esa carpeta. Con el cambio de §2.1 desaparece la razón por la que un humano la creaba. La carpeta no existirá porque nadie la inventará.

> **Rechazado explícitamente:** añadir a `quorum init` un borrado de `kitty-specs/`. Ver §6.

### 2.3 Test de no-regresión

`internal/core/task_manager_test.go:1269` hoy sólo comprueba `# Quorum`, `worktrees/` y `.ai/tasks/active/*` — **ningún test asserta `.ai/tasks/done/*`**, así que el cambio no rompe la suite. Precisamente por eso hay que **añadir** una aserción negativa; si no, la línea vuelve callada en el próximo refactor:

```go
if strings.Contains(string(gb), ".ai/tasks/done/") {
    t.Fatalf(".gitignore no debe ignorar tasks/done: %s", gb)
}
```

---

## 3. Alcance B — proyectos ya funcionando (migración)

### 3.1 Por qué hace falta código

`appendGitExcludeEntries` (`task_manager.go:~921`) **solo añade lo que falta; nunca quita**. Un proyecto existente conserva su línea `.ai/tasks/done/*` para siempre aunque `ignoreEntries` cambie. Editar el `.gitignore` a mano funciona, pero hay que recordarlo en cada repo.

Hay precedente exacto para migrar proyectos existentes desde `init`: `RunInitMemoryMigration` (`internal/core/memory_init_migration.go:50-183`), invocada en `task_manager.go:874`, que migra y **borra** `.ai/memory/*.md` legado. La migración propuesta aquí es estrictamente menos invasiva: no borra datos, solo retira una línea de configuración.

### 3.2 `RunDoneTrackingMigration` — contrato

Invocarla desde `InitializeProject`, **antes** de `appendGitExcludeEntries`.

1. **Guarda de una sola ejecución.** Marcador persistente (campo en `.quorumrc` o registro en `memory.db`, según el patrón que ya usa la migración de memoria). Si el marcador existe → retorno inmediato. Esto respeta al usuario que decida re-añadir la línea a propósito después: la migración no se la volverá a quitar.
2. **Ámbito doble.** Retirar la línea de `.gitignore` **y** de `.git/info/exclude`. Los proyectos con `GitHideRuntime: true` (`QuorumConfig`, `internal/core/quorum_config.go:13-18`) escriben las reglas en `exclude`, no en `.gitignore`. Una migración que solo toque `.gitignore` los deja rotos y en silencio.
3. **Cirugía, no reescritura.** Borrar exclusivamente las líneas cuyo contenido, ya recortado, sea `.ai/tasks/done/*` o `!.ai/tasks/done/.gitkeep`. Todo lo demás del fichero se preserva byte a byte: ese fichero es del usuario, no de Quorum.
4. **No tocar el índice de git.** La migración **jamás** ejecuta `git add` ni `git commit`. Al retirar la regla aparecerán N directorios sin rastrear; qué hacer con ellos es decisión del humano. Un scaffolder que commitea por su cuenta es un scaffolder en el que no se puede confiar.
5. **Degradación silenciosa.** Sin `.gitignore`, sin permisos de escritura o sin la línea presente → no-op, sin error.
6. **Aviso al terminar.** Una línea en stdout: qué se retiró y que quedan artefactos sin rastrear pendientes de decisión.

### 3.3 Procedimiento manual para `hexcell` (una vez)

En este orden, **en commits separados** para que cada paso sea reversible por sí solo:

1. **Auditar antes de añadir nada** (§5.2). Bloqueante.
2. `.gitignore`: retirar `.ai/tasks/done/*` y `!.ai/tasks/done/.gitkeep`; añadir la regla de `dispatch/` si se elige la opción recomendada (§5.1).
3. `git add .ai/tasks/done` → commit `chore: versionar artefactos SDD de tareas terminadas`.
4. `git rm -r kitty-specs` → commit `chore: retirar kitty-specs, sustituido por .ai/tasks/done versionado`.
5. Documentar la convención en el `CLAUDE.md` del proyecto. Hoy **nada** de esto está escrito; ese fue el hallazgo original de la auditoría y este cambio lo agrava si no se registra.

> Si se ejecuta la migración de §3.2 sobre `hexcell`, sustituye al paso 2 pero **no** a los pasos 1, 3, 4 y 5.

---

## 4. Lo que NO cambia

- `active/`, `failed/`, `inbox/` y `worktrees/` siguen ignorados.
- `.gitkeep` se sigue creando en los cuatro directorios.
- `~/.quorum/memory.db` no se toca: guarda memoria curada, no artefactos, y es global al usuario — sigue siendo un almacén distinto, no un duplicado. (Aparte: sigue viviendo fuera del repo, así que un disco perdido se la lleva. Fuera del alcance de esta propuesta, pero conviene no olvidarlo.)
- `quorum init` sigue siendo idempotente y no destructivo sobre ficheros del usuario.

---

## 5. Lo que hay que tener en cuenta

### 5.1 DECISIÓN PENDIENTE — `dispatch/` es dos tercios del peso

Medido en `hexcell`: **4,3 M de los 6,4 M** son 17 carpetas `dispatch/`. Los artefactos SDD reales son 2,1 M.

Un bundle contiene `prompt.md` (el código y docs empaquetados y enviados al delegado externo), `manifest.json`, `notes.txt` (el log crudo del delegado — 238 K en una sola tarea) y `result.json`. Se crean en `cmd/fleet_bundle.go:97-98` e `internal/core/fleet_dispatch.go:88`. **No existe hoy ninguna opción para omitirlos o podarlos.**

| Opción | Qué entra a git | Coste por tarea | A 100 tareas |
|---|---|---|---|
| Todo | artefactos + bundles | ~250 K | ~25 M |
| **Sin `dispatch/`** (recomendada) | los 8 artefactos numerados | ~80 K | ~8 M |

Recomendación: ignorar `dispatch/` añadiendo a `ignoreEntries` la entrada `.ai/tasks/*/*/dispatch/`. Un bundle es un artefacto de ejecución — cómo se le habló a un modelo un martes concreto —, no una decisión de diseño. `prompt.md` es además una **copia de código que ya está versionado**: duplicarlo dentro del mismo repo es redundancia pura. El rastro que importa ya vive en `07-trace.json`.

Quien quiera conservar los bundles siempre puede retirar esa línea. Lo contrario —sacar 25 M del historial una vez dentro— no tiene vuelta atrás barata.

### 5.2 RIESGO PRINCIPAL — auditar secretos antes del primer `git add`

`prompt.md` empaqueta ficheros arbitrarios del repositorio. El `manifest.json` inspeccionado lista specs y docs, y trae una lista `dropped` por presupuesto de bytes, pero **el conjunto lo decide el bundler, no una lista blanca de seguridad**. Un `grep` sobre los bundles de `hexcell` con el patrón `api_key|secret|password|BEGIN … PRIVATE KEY` devuelve 4 ficheros con coincidencias; una revisión superficial sugiere que son menciones legítimas en specs sobre credenciales y emparejamiento, **no** secretos reales, pero eso hay que confirmarlo fichero a fichero antes de commitear.

**Lo que entra al historial de git no sale barato.** La auditoría es bloqueante y va antes del paso 3 de §3.3. Es, además, el argumento más fuerte para la opción recomendada en §5.1: sin `dispatch/`, la superficie de exposición se reduce a artefactos que el humano ya revisó tarea por tarea.

### 5.3 `.gitignore` es inerte sobre ficheros ya rastreados

Una vez que los 26 directorios están en el índice, si alguien re-ejecuta un binario viejo de Quorum y la línea vuelve, **los ficheros ya rastreados siguen rastreados**. La regresión no se ve: solo dejarían de aparecer las tareas **nuevas**. Es un fallo silencioso, y por eso el test de §2.3 no es opcional.

### 5.4 Ruido en `git status`

`quorum task clean` mueve `active/<ID>` → `done/<ID>` (`internal/core/task_transition.go:427-487`). Con `done/` versionado, cada cierre de tarea ensucia el árbol de trabajo. Es el comportamiento deseado, pero es un cambio de hábito: cerrar una tarea pasa a exigir un commit.

### 5.5 Sobre añadir un flag de configuración

`QuorumConfig` tiene hoy cuatro campos (`ProjectID`, `ProjectName`, `GitHideRuntime`, `GitHideAgents`). Sería fácil añadir un quinto tipo `VersionDoneTasks`.

**Recomendación: no añadirlo.** Versionar el rastro SDD es una convención del producto, no una preferencia; cada flag es una rama más que mantener, testear y documentar. Quien no lo quiera re-añade la línea a mano una vez, y la guarda de §3.2.1 hace que Quorum la respete. Si más adelante aparecen usuarios reales con esa necesidad, se reabre.

### 5.6 Ruido de nombres

`ideas/hsme-cli-mkcli/` menciona un `semantic/kitty-specs/` como archivo legado congelado, en un contexto distinto y sin relación con esta carpeta. Nadie debería confundirlos al buscar por nombre.

---

## 6. Rechazado: que `quorum init` borre `kitty-specs/`

Se evaluó y se descarta, por tres motivos:

1. **Acoplamiento invertido.** `kitty-specs` es una convención de UN proyecto. Quorum es la herramienta genérica. Que el scaffolder conozca el nombre de una carpeta de un repositorio concreto es exactamente la dependencia que después no se puede sacar — y el día que se use en otro repo donde ese nombre signifique otra cosa, se la come.
2. **Un comando de inicialización que borra ficheros del usuario es una mina.** `init` se re-ejecuta a ciegas sobre proyectos existentes, precisamente para reparar el scaffold. El día que alguien lo lance en el directorio equivocado no pierde configuración: pierde trabajo. La única eliminación que hace hoy (`RunInitMemoryMigration`) borra ficheros que Quorum mismo creó, tras verificar que su contenido está a salvo en la base de datos. No es el mismo caso.
3. **Es automatizar algo que ocurre una vez.** `git rm -r kitty-specs` resuelve lo mismo en diez segundos y deja el rastro donde debe estar: en el historial del repositorio afectado.

---

## 7. Resumen de implementación

| # | Cambio | Fichero | Tamaño |
|---|---|---|---|
| 1 | Quitar `.ai/tasks/done/*` y su negación de `ignoreEntries` | `internal/core/task_manager.go:886-897` | −2 líneas |
| 2 | Añadir `.ai/tasks/*/*/dispatch/` (si se acepta §5.1) | mismo | +1 línea |
| 3 | Aserción negativa en el test de `.gitignore` | `internal/core/task_manager_test.go:~1269` | +3 líneas |
| 4 | `RunDoneTrackingMigration` con guarda, doble ámbito y cirugía por línea | `internal/core/` (fichero nuevo) + `task_manager.go:874` | ~80 líneas |
| 5 | Documentar la convención en el README de Quorum | `README.md` | párrafo |

Bloqueantes previos: decidir §5.1 y ejecutar la auditoría de §5.2.
