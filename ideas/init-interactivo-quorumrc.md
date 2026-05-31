# 🧭 Propuesta Técnica: Prompt Interactivo de `.quorumrc` en `quorum init`

**Estado:** Lista para flujo SDD — alimenta `/q-brief` y `/q-blueprint`.
**Contexto:** Evolución de Quorum. Cambio acotado al propio framework (va por `go test ./...`, no por el lifecycle `/q-*`).
**Origen:** Hallazgo de que la funcionalidad descrita ("input interactivo al faltar `.quorumrc`") NO existe; hoy solo se imprime una sugerencia dentro de un mensaje de error y se aborta.

---

## 1. El Problema Real (acotado)

Cuando un usuario ejecuta `quorum init` en un proyecto sin `.quorumrc` y **sin** pasar `--project-id`/`--project-name`, el comando **aborta con un error** en lugar de capturar los datos. El usuario debe leer el error, copiar la sugerencia y **re-ejecutar** el comando a mano.

Evidencia en código (`internal/core/task_manager.go:1330-1336`):

```go
if opts.ProjectID == "" && opts.ProjectName == "" {
    if opts.NonInteractive {
        return nil, fmt.Errorf(".quorumrc is missing; provide --project-id and --project-name for non-interactive init")
    }
    suggested := SuggestProjectIdentity(projectRoot)
    return nil, fmt.Errorf(".quorumrc is missing; suggested --project-id %q --project-name %q", suggested.ProjectID, suggested.ProjectName)
}
```

El detalle clave: la rama "interactiva" (`!NonInteractive`) **también devuelve un error y muere**. Calcula una sugerencia pero nunca lee `stdin`. La infraestructura está a medias:

- `cmd/init.go:36` ya tiene `stdinIsTerminal()` que detecta TTY.
- `InitOptions.NonInteractive` ya se propaga (`task_manager.go:1143`).
- Pero **ninguna ruta lee de `os.Stdin`** — no hay `bufio.Reader`, `Scanln` ni equivalente en el flujo de init.

Sin `.quorumrc` válido, la memoria centralizada SQLite no tiene `project_id` estable y el usuario no puede operar correctamente.

---

## 2. Alcance

### Dentro de alcance
- Prompt interactivo en `quorum init` **únicamente** cuando: (a) falta `.quorumrc`, (b) no se pasaron ambos flags, y (c) `stdin` es un TTY (`!NonInteractive`).
- Captura de `project_id` y `project_name` con sugerencia como valor por defecto.
- Re-prompt en bucle ante valores inválidos.
- Confirmación final antes de escribir el archivo.

### Fuera de alcance (decisiones tomadas)
- **No** se agrega prompt lazy a otros comandos (`memory save`, etc.). Si esos comandos necesitan `.quorumrc` y falta, siguen fallando con su error actual. El prompt vive **solo en `init`**.
- **No** se cambia el comportamiento no-interactivo: si `stdin` no es TTY y faltan flags, se mantiene el error actual (necesario para CI/scripts).
- **No** se tocan los flags `--project-id`/`--project-name`: si se pasan, siguen teniendo prioridad y saltan el prompt.

---

## 3. Decisiones de Diseño (resueltas)

| # | Decisión | Elección | Justificación |
|---|----------|----------|---------------|
| D1 | Presentación de cada campo | **Sugerencia como default + Enter** | Mínimo tecleo; el usuario solo confirma o sobrescribe. |
| D2 | Input inválido (vacío tras normalizar) | **Reintentar en bucle** | No se pierde lo ya escrito; el usuario corrige el campo concreto con el motivo a la vista. |
| D3 | Ubicación del prompt | **Solo en `quorum init`** | Alcance limpio; evita mezclar I/O interactivo en más rutas y mantiene tests simples. |
| D4 | Confirmación final | **Sí, mostrar resumen y pedir `[Y/n]`** | Evita persistir basura por un error de tecleo antes de escribir `.quorumrc`. |
| D5 | `project_id` que no es slug | **Auto-slug + mostrar normalizado** | Reutiliza `SlugifyProjectID`; mínima fricción. El re-prompt (D2) solo dispara si la normalización produce cadena vacía. |
| D6 | `project_name` vacío | **Derivar del `project_id`** | Usa `humanizeProjectName` sobre el `project_id` ya capturado; cero fricción. |
| D7 | Respuesta `n` en confirmación | **Error (exit 1)** | Señal de aborto detectable por scripts que envuelvan `init`. No escribe `.quorumrc`. |
| D8 | EOF / Ctrl-D a mitad del prompt | **Cancelación con error claro** (invariante) | Nunca panic ni bucle infinito; trata `io.EOF` como aborto con mensaje que sugiere los flags. |

---

## 4. Diseño Técnico

### 4.1 Flujo del prompt (UX objetivo)

```
[*] Initializing Quorum in /home/gary/dev/quorum...
.quorumrc no encontrado. Configuremos la identidad del proyecto.

project_id [quorum]: Mi Proyecto
  → normalizado a "mi-proyecto"
project_name [Quorum]: _

Resumen:
  project_id:   mi-proyecto
  project_name: Mi Proyecto
¿Escribir .quorumrc con estos valores? [Y/n] _
  [+] Created .quorumrc for project mi-proyecto.
```

Ejemplo de re-prompt (D2) — solo cuando la normalización deja cadena vacía:

```
project_id [quorum]: !!!
[!] project_id quedó vacío tras normalizar; usa minúsculas, números o guiones.
project_id [quorum]: _
```

Auto-slug (D5): cualquier entrada no-slug se transforma con `SlugifyProjectID` y se muestra normalizada; el resumen final (D4) deja visible el valor real antes de escribir.

### 4.2 Punto de cambio

Toda la lógica nueva vive en `ensureProjectConfig(projectRoot string, opts InitOptions)` (`internal/core/task_manager.go:1308`). La rama interactiva del bloque `opts.ProjectID == "" && opts.ProjectName == ""` deja de devolver error y, en su lugar, invoca un nuevo helper que captura los valores.

Propuesta de helper aislado (testeable inyectando `io.Reader`/`io.Writer`):

```go
// promptProjectConfig captura project_id y project_name de forma interactiva,
// usando suggested como defaults. Reintenta cada campo hasta que sea válido.
func promptProjectConfig(in io.Reader, out io.Writer, suggested *QuorumConfig) (*QuorumConfig, error)
```

Esto evita acoplar la lógica a `os.Stdin`/`os.Stdout` directamente, lo cual es **requisito para los tests** (ver §6).

### 4.3 Reutilización (no reinventar)

- **Sugerencias:** `SuggestProjectIdentity(projectRoot)` ya deriva nombre del remote git o del directorio (`quorum_config.go:97`). Se usa tal cual para los defaults.
- **Normalización:** `SlugifyProjectID(value)` (`quorum_config.go:111`) para ofrecer auto-corrección del `project_id` (decisión abierta, ver §7).
- **Validación:** `ValidateQuorumConfig(config)` (`quorum_config.go:58`) es la única autoridad de validez; el bucle de re-prompt se apoya en ella, no duplica reglas (regex slug, no-vacío).
- **Persistencia:** `WriteQuorumConfigTo(config, projectRoot)` (`quorum_config.go:84`) escribe e incluso re-valida antes de escribir.

### 4.4 Reglas de validación (heredadas, no nuevas)

| Campo | Regla | Fuente |
|-------|-------|--------|
| `project_id` | No vacío y `^[a-z0-9-]+$` | `projectIDRegex`, `ValidateQuorumConfig` |
| `project_name` | No vacío | `ValidateQuorumConfig` |

El prompt **no inventa reglas**; cualquier validación adicional debe primero entrar en `ValidateQuorumConfig`.

---

## 5. Cambios Requeridos

| Archivo | Cambio |
|---------|--------|
| `internal/core/task_manager.go` | Modificar `ensureProjectConfig` para invocar `promptProjectConfig` en la rama interactiva; añadir el helper `promptProjectConfig`. |
| `internal/core/quorum_config.go` | Sin cambios funcionales esperados (reutiliza helpers existentes). Posible export de helper si el prompt se ubica aquí. |
| `cmd/init.go` | Sin cambios: `NonInteractive` y los flags ya se propagan correctamente. |
| `internal/core/task_manager_test.go` | Nuevos tests del prompt interactivo (entradas simuladas vía `io.Reader`). |

---

## 6. Criterios de Aceptación (testeables)

> "Tests are the only proof" (Constitución #8). Cada criterio debe tener un test bajo `go test ./...`.

1. **Happy path con defaults:** stdin entrega líneas vacías (Enter) para ambos campos + `y` en confirmación → se escribe `.quorumrc` con los valores sugeridos por `SuggestProjectIdentity`.
2. **Override de valores:** stdin entrega `mi-proyecto`, `Mi Proyecto` + `y` → `.quorumrc` contiene exactamente esos valores.
3. **Auto-slug (D5):** stdin entrega `Mi Proyecto` para `project_id` → se normaliza a `mi-proyecto` con `SlugifyProjectID`; el valor persistido es `mi-proyecto`, sin re-prompt.
4. **Re-prompt ante vacío tras normalizar (D2):** stdin entrega `!!!` y luego `valido` para `project_id` → el primer intento se rechaza con mensaje (normalización vacía), el segundo se acepta; resultado `valido`.
5. **`project_name` derivado (D6):** `project_id` = `mi-proyecto` y `project_name` vacío → se deriva `Mi Proyecto` vía `humanizeProjectName`.
6. **Confirmación negativa (D7):** stdin responde `n` a la confirmación → NO se escribe `.quorumrc` y se retorna **error (exit 1)** con mensaje de aborto.
7. **EOF a mitad del prompt (D8):** stdin se cierra (EOF) antes de completar → retorna error claro sugiriendo `--project-id`/`--project-name`, sin panic ni bucle.
8. **No-interactivo intacto:** `NonInteractive: true` sin flags → sigue devolviendo el error actual, NUNCA lee stdin.
9. **Flags tienen prioridad:** con `--project-id`/`--project-name` provistos → no se dispara ningún prompt aunque stdin sea TTY.
10. **`.quorumrc` existente intacto:** si ya existe, el comportamiento de merge actual (`ensureProjectConfig`) no cambia.

---

## 7. Decisiones Cerradas (sin ambigüedad para el flujo SDD)

Estas estaban abiertas y quedaron resueltas explícitamente. El agente del flujo SDD **no debe re-preguntarlas**.

1. **Auto-slug del `project_id` (D5):** cualquier entrada no-slug se normaliza con `SlugifyProjectID` y se muestra el valor resultante. El re-prompt en bucle solo dispara cuando la normalización produce cadena vacía (ej. `!!!`).
2. **Default de `project_name` (D6):** si queda vacío, se deriva con `humanizeProjectName` del `project_id` ya capturado. No se exige input adicional.
3. **Respuesta `n` en confirmación (D7):** `quorum init` sale con **error (exit 1)** y mensaje de aborto; NO escribe `.quorumrc`.
4. **EOF/Ctrl-D a mitad del prompt (D8):** invariante de robustez — se trata como aborto con error claro que sugiere `--project-id`/`--project-name`; nunca panic ni bucle infinito.

---

## 8. Riesgos y Mitigaciones

| Riesgo | Mitigación |
|--------|------------|
| I/O interactivo difícil de testear | Helper `promptProjectConfig(in io.Reader, out io.Writer, ...)` con dependencias inyectadas; nunca llamar `os.Stdin` directo en la lógica. |
| Romper CI/scripts que dependen del error actual | `NonInteractive` se respeta estrictamente; criterio de aceptación #5 lo blinda. |
| Divergencia con `ValidateQuorumConfig` | El prompt NO duplica reglas; valida exclusivamente vía `ValidateQuorumConfig`. |
| Scope creep hacia otros comandos | Decisión D3 fija el prompt solo en `init`; cualquier extensión requiere nueva propuesta. |

---

## 9. Resumen para el Brief

**Qué:** Convertir la rama interactiva de `ensureProjectConfig` en un prompt real que capture `project_id` y `project_name` cuando falta `.quorumrc`, con defaults sugeridos, re-prompt ante inválidos y confirmación final.
**Por qué:** Hoy el usuario es expulsado con un error y debe re-ejecutar a mano; bloquea el primer uso correcto de Quorum y la memoria centralizada.
**Dónde:** `internal/core/task_manager.go` (`ensureProjectConfig` + nuevo helper), reutilizando `SuggestProjectIdentity`, `ValidateQuorumConfig` y `WriteQuorumConfigTo`.
**Validación:** `go test ./internal/core` con los 7 criterios de §6.
