# 🧠 Propuesta Técnica: Captura de Memoria por Sesión (`q-session`)

**Slug / task_id:** `F-06`
**Estado:** Propuesta lista para ingesta directa al flujo SDD (sin entrevista).
**Contexto:** Evolución de Quorum v1.2+. Hermano de `q-memory`: misma taxonomía (decision/pattern/lesson), distinta **unidad de captura** (sesión de trabajo en vez de tarea del lifecycle 00→07).
**Origen:** Destilado de un skill externo de *session handoff* (`/home/gary/tug/handoffs`) que persiste conocimiento de la sesión de trabajo. Se conserva el OBJETIVO (exprimir decisiones/patrones/lecciones del diálogo humano↔agente para una BDD de conocimiento reutilizable); se adapta el MECANISMO a las invariantes de Quorum (memoria curada, tipada, en SQLite, vía única ruta de escritura).

> **Este documento es la ingesta completa.** Contiene goal, invariantes, acceptance, non-goals, constraints, edge cases y componentes con rutas. El flujo SDD (`/q-brief` → `/q-blueprint` → `/q-implement` → `/q-verify` → `/q-review` → `/q-accept`) debe ejecutarse **sin formular preguntas**: toda decisión abierta ya está cerrada en §4 y §7.

---

## 1. El Problema Real (acotado)

En cada sesión de trabajo, humano y agente toman **decisiones**, aplican **patrones** y aprenden **lecciones** que hoy se pierden cuando termina la sesión. Quorum ya tiene una memoria curada excelente (`q-memory`), pero está **acoplada a una tarea del lifecycle**: lee `00-spec.yaml`…`07-trace.json` desde `.ai/tasks/{done,failed}/<ID>/` y solo captura conocimiento atribuible a *esa* tarea aceptada.

Mucho conocimiento valioso nace **fuera de una tarea**: en exploraciones, diseños descartados, discusiones de arquitectura, depuraciones ad-hoc. No hay superficie que lo capture.

| Superficie actual | Qué captura | Lo que NO captura |
|---|---|---|
| `q-memory` | decision/pattern/lesson de **una tarea aceptada** (fuente: artefactos 00→07) | conocimiento de una **sesión** sin tarea asociada |
| `07-trace.json` | log de actividad **por tarea** | nada transversal a la sesión |
| `quorum memory search` / visor (ADR 0006) | **lectura** de memoria ya curada | no es ruta de captura |

**Conclusión:** la pieza faltante es un **segundo punto de ingesta curada, human-invoked, por sesión**, que reuse el pipeline probado de `q-memory` sin duplicar infraestructura ni debilitar invariantes.

---

## 2. La Solución (decisión de arquitectura: reuso total, cero código Go de producción)

Un skill nuevo **`q-session`** que:

1. Revisa el **diálogo de la sesión actual** (no artefactos de tarea) como fuente de evidencia.
2. Identifica memorias de alta señal tipadas `decision` / `pattern` / `lesson`.
3. **Propone candidatos al humano y espera confirmación** antes de persistir (curación humana en el loop → satisface "curated, never automatic").
4. Persiste cada memoria confirmada vía el comando existente `quorum memory save`, con un **centinela** en `source_task`.

**Reuso verificado (no se reinventa nada):**

| Pieza existente | Reuso |
|---|---|
| Tipos `decision`/`pattern`/`lesson` | Encajan exactos con "decisiones, patrones, lecciones". Sin tipo nuevo. |
| `quorum memory save` (transaccional, idempotente por hash) | Única ruta de escritura. Reusado tal cual. |
| `memory.schema.json` | **Sin cambios.** `source_task` es `{"type":"string"}` libre (sin patrón), así que admite el centinela. |
| Tablas SQLite (`memory_entries`, satélites, FTS) | **Sin cambios.** `source_task` es `NOT NULL` libre. |
| `quorum memory search` + visor (ADR 0006) | Las memorias de sesión aparecen automáticamente; `source_task=SESSION-*` es filtrable por LIKE. Sin trabajo adicional. |
| IDs `DEC/PAT/LES-YYYY-MM-DD-HHmmssSSS`, `supersedes`, `anti_patterns` | Heredados de `q-memory`. |

**Centinela de sesión** (resuelve la fricción `source_task` required):

```text
source_task: "SESSION-YYYY-MM-DD"        # caso normal
source_task: "SESSION-YYYY-MM-DD-NN"     # 2.ª+ sesión conceptual del mismo día (sufijo decidido por el humano)
```

Distingue memoria-de-sesión (`SESSION-2026-06-06`) de memoria-de-tarea (`FEAT-001`) por prefijo, **sin nuevo enum ni tabla**. El hash canónico incluye `source_task`, por lo que dos sesiones distintas nunca colisionan y la misma memoria recapturada en la misma sesión colapsa a `unchanged`.

---

## 3. Lo que NO se Implementará (límites explícitos)

| Componente rechazado | Razón |
|---|---|
| Skill de **lectura** (`q-recall`) | La lectura ya la cubren `quorum memory search` y el visor read-only (ADR 0006). Duplicarla es alcance separado (otro ADR). |
| **Markdown narrativo** de handoff de sesión | Violaría la Regla #5 (markdown solo en `docs/adr/` y docs externos) y el modelo de memoria tipada. Solo memorias tipadas en SQLite. |
| Nuevo **tipo enum** (`handoff`) o **tabla** nueva | Innecesario: los tres tipos ya cubren el caso. Forzaría migración SQLite, tocar el `CHECK` y todos los `switch type`. Alto costo, cero beneficio. |
| Cambios al **schema** `memory.schema.json` o a las **tablas** SQLite | El centinela `source_task` libre lo hace innecesario. Cero impacto en idempotencia/hash. |
| **Auto-captura** por tiempo/volumen/background | Prohibida por la gobernanza de memoria (ADR 0003, manifiesto). La captura es exclusivamente human-invoked. |
| **Auto-transición** o auto-chain a otro `/q-*` | Violaría la Regla #9. `q-session` es single-phase y terminal; NO entra en la tabla de auto-transiciones autorizadas. |
| Persistir **sin confirmación humana** | Rompería "curated, never automatic". El humano confirma los candidatos antes del save. |
| Ruta de ingesta paralela a `quorum memory save` | `quorum memory save` sigue siendo la **única** ruta de escritura (ADR 0006). |
| Push a **sistemas externos** (HSME, vector DBs) | Subordinados a Git/lifecycle; consumen vía export, no reciben push del skill. |

---

## 4. Decisiones Cerradas (NO preguntar)

Estas decisiones ya fueron tomadas. El flujo SDD las toma como dadas.

| # | Decisión | Valor cerrado |
|---|---|---|
| D1 | Nombre del skill | **`q-session`** (prefijo `q-` obligatorio: lo exige `skill_protocol_test.go`). |
| D2 | Alcance | **Solo escritor.** Lectura ya cubierta por search + visor. |
| D3 | Formato de salida | **Solo memorias tipadas** (decision/pattern/lesson). Sin markdown narrativo. |
| D4 | Centinela `source_task` | **`SESSION-YYYY-MM-DD`** (sufijo `-NN` opcional decidido por el humano). |
| D5 | Cambios de schema/SQLite | **Ninguno.** Reuso del schema y tablas vigentes. |
| D6 | Ruta de escritura | **`quorum memory save`** (única, sin alternativas). |
| D7 | Curación | **Humana en el loop**: proponer candidatos → esperar confirmación → persistir. |
| D8 | Máximo por sesión | **Hasta 5 memorias de alta señal** (una sesión cubre más terreno que una tarea, donde `q-memory` usa 3; aun así se prioriza señal sobre volumen). |
| D9 | Idioma | Chat en **español**; valores persistidos en **inglés conciso** (protocolo de skills). |

---

## 5. Invariantes (deben permanecer verdaderas tras la implementación)

1. `memory.schema.json` y las tablas SQLite (`memory_entries` y satélites) **no cambian**.
2. `quorum memory save` sigue siendo la **única ruta de escritura** de memoria curada.
3. La idempotencia por hash sigue intacta: misma memoria + misma sesión → `unchanged`; sesiones distintas → no colisionan (`source_task` participa del hash).
4. `q-session` es **single-phase y terminal**: no auto-transiciona ni auto-encadena (Regla #9); **no** aparece en la tabla de auto-transiciones autorizadas.
5. La captura sigue **human-invoked**: sin auto-captura por tiempo/volumen/background.
6. La memoria se persiste **solo tras confirmación humana** de los candidatos propuestos.
7. Toda memoria de sesión usa `source_task` con prefijo **`SESSION-`**; los tipos siguen siendo `decision`/`pattern`/`lesson` y los IDs `DEC/PAT/LES-YYYY-MM-DD-HHmmssSSS`.
8. Los valores persistidos en SQLite están en **inglés conciso**, aunque el chat sea en español.
9. `q-session` **no edita** código fuente, artefactos lifecycle (`00`→`07`) ni `07-trace.json`, ni hace push a sistemas externos.
10. `go test ./...` sigue verde; en particular, el nuevo `SKILL.md` pasa todos los invariantes de `internal/core/skill_protocol_test.go` (ver §8).

---

## 6. Edge Cases (manejo obligatorio, sin preguntar)

| Caso | Comportamiento esperado |
|---|---|
| `.quorumrc` ausente o setup de memoria no disponible | Reportar `BLOCKED` con explicación concisa en español y sugerir `quorum init`. **Nunca** ejecutar `quorum init` desde el skill. |
| `quorum memory save` falla validación de schema | Corregir solo si el error es **mecánico** (typo, comilla faltante, campo mal formado) sin cambiar el significado; en caso contrario `BLOCKED` para decisión humana. |
| Error de SQLite / lock / permisos | `BLOCKED`. **Nunca** escribir un fallback durable bajo `memory/` ni en otro directorio local. |
| La sesión no tuvo conocimiento de alta señal | No persistir nada. Cerrar con turno **informativo** (sin indicador de espera) explicando que no hubo memorias que capturar. |
| Varias sesiones conceptuales el mismo día | El humano puede pedir sufijo `-NN` en el centinela (`SESSION-2026-06-06-02`). Por defecto, `SESSION-YYYY-MM-DD`. |
| Colisión de ID | Improbable por el sufijo `HHmmssSSS`. No sobrescribir IDs existentes. |
| Recaptura de una memoria ya guardada | La idempotencia por hash devuelve `unchanged`; el skill lo reporta sin error. |
| Una memoria de sesión **supersede** a una previa | Permitido vía campo `supersedes` (mismo protocolo que `q-memory`): la vieja permanece en DB, el enlace preserva la traza causal. |

---

## 7. Componentes a Implementar (para `q-blueprint` → contrato `touch`)

> Cambio aditivo, de bajo riesgo, **reuso total del pipeline existente**. Producción nueva = 1 archivo de skill + 1 ADR. El resto es un test y notas de documentación.

### 7.1 `CREAR` — `.agents/skills/q-session/SKILL.md`

Modelado sobre `.agents/skills/q-memory/SKILL.md`, adaptado a captura por sesión. Secciones obligatorias:

- **Frontmatter**: `name: q-session`, `description:` (mencionar "session", "decisions/patterns/lessons", "SQLite", "single-phase"), `user-invocable: true`.
- **🌐 Communication Protocol** (copiar literal el bloque de `q-memory/SKILL.md`): debe contener la frase exacta `ALWAYS respond in Spanish`, el indicador de espera **condicional** (`only when …`), la regla de no-fence final y el prefijo de contexto CLI.
- **Rol**: "Eres el **Curador de Sesión**: destilás conocimiento durable del diálogo de esta sesión (no de una tarea del lifecycle)."
- **Source Inputs**: el **diálogo de la sesión actual**. Explícito: NO lee artefactos `00`→`07`; NO inventa contenido.
- **What to Capture / Do not capture / Anti-patterns**: reusar la guía de `q-memory`.
- **Centinela `source_task`**: instruir `SESSION-YYYY-MM-DD` (+`-NN` opcional) y explicar el porqué.
- **IDs / JSON Shape / Supersession Protocol**: idénticos a `q-memory`, con `source_task` centinela.
- **Output Location**: persistir vía `cat <payload>.json | quorum memory save` o `quorum memory save --file <payload>.json`; archivos temporales solo bajo `.tmp/`. Prohibido escribir bajo `memory/`.
- **Workflow**: (1) revisar el diálogo; (2) proponer ≤5 candidatos (tipo + título + 1 línea); (3) **esperar confirmación** del humano (turno de espera → cierra con `ESPERANDO RESPUESTA DEL USUARIO...`); (4) generar IDs; (5) persistir; (6) reportar IDs SQLite devueltos; (7) manejo de fallos según §6.
- **Rules**: compacto; valores en inglés conciso; no editar código; no sobrescribir IDs; prohibición explícita de auto-captura y auto-chain (incluir la frase `Auto-chaining violates Rule #9`).
- **🛑 Handoff (single-phase boundary)**: bloque de cierre terminal en español (con `Artefactos producidos:`), sin transición de estado, sin indicador de espera en el cierre exitoso. Cualquier línea de comando dentro de bloques ```` ```text ```` debe llevar prefijo `[ROOT]` o `[WORKTREE:...]`.

### 7.2 `CREAR` — `docs/adr/0007-captura-de-memoria-por-sesion.md`

Siguiente número ADR (último es 0006). Patrón en español de ADR 0006 (Estado / Contexto / Decisión / Consecuencias):

- **Decisión**: se permite un skill `q-session` human-invoked que persiste `decision`/`pattern`/`lesson` originadas en el **diálogo de una sesión** (no en una tarea), reusando `quorum memory save` y el schema vigente **sin cambios**, marcando `source_task = "SESSION-YYYY-MM-DD"`. Reafirma: única ruta de escritura = `quorum memory save`; sin auto-captura; single-phase; sin tipo/tabla nuevos; curación humana obligatoria.
- **Consecuencias**: la memoria deja de estar acoplada 1:1 a una tarea; `SESSION-*` es un **valor de `source_task`**, no un nuevo namespace de tarea (no entra a `FindTaskDir`/lifecycle). El visor (ADR 0006) las muestra sin trabajo extra. Riesgo de ruido mitigado por la confirmación humana.

### 7.3 `MODIFICAR` — `internal/core/memory_service_test.go`

Añadir un caso (molde de `TestSaveMemoryEntryPersistsEntryAndSatellites`) que blinda el centinela:

- Persistir payload con `source_task = "SESSION-2026-06-06"` → `Status == "inserted"`; fila guardada con ese `source_task`.
- Re-guardar el mismo payload → `Status == "unchanged"` (idempotencia con centinela).
- `SearchMemoryEntries` lo recupera.
- Sin cambios al schema ni a SQLite.

### 7.4 `MODIFICAR` (opcional, defensivo) — `CLAUDE.md`

Una línea en "Memory is curated, never automatic": aclarar que `q-session` es una **segunda ruta human-invoked** sobre `quorum memory save`, con `source_task=SESSION-*`, y que **no** es auto-captura.

### 7.5 Nota sobre `internal/core/skill_protocol_test.go`

Los tests genéricos barren **todo** directorio `q-*` automáticamente (vía `ReadDir`), así que recogerán `q-session` sin tocar el archivo. **No** es obligatorio añadir `q-session` al mapa `producers` (ese test solo valida los listados). Si se decide añadirlo (defensivo), la entrada sería `"q-session": {"memory.schema.json", "quorum memory save"}` y el `SKILL.md` deberá referenciar ambos tokens y la frase `field values` en inglés.

---

## 8. Invariantes de Test que el `SKILL.md` Debe Satisfacer (críticos)

`internal/core/skill_protocol_test.go` aplica a `q-session` (por ser `q-*`). El `SKILL.md` falla `go test ./...` si no cumple TODOS:

| Test | Requisito concreto |
|---|---|
| `TestSkillProtocolWaitIndicatorIsConditional` | Debe contener `Communication Protocol` y describir el indicador de espera como **condicional** (`only when`/`when`/`if`). Prohibida la frase `close every turn`. |
| `TestSkillProtocolSinglePhaseBoundaryPreserved` | Debe contener `ALWAYS respond in Spanish`, la palabra `single-phase`, y `Do NOT activate any other skill` **o** `Auto-chaining violates Rule #9`. |
| `TestSkillProtocolSuccessHandoffOmitsWaitIndicator` | El bloque ```` ```text ```` de cierre exitoso (con `Artefactos producidos:`) **no** debe terminar en `ESPERANDO RESPUESTA DEL USUARIO...`. |
| `TestSkillProtocolUserVisibleOutputTemplatesAreSpanish` | Si hay sección `## Output…`, los bloques ```` ```text ```` no deben contener labels en inglés (`Status:`, `Next:`, `Findings:`, etc.). |
| `TestSkillProtocolFencedCommandContextPrefix` | Toda línea de comando (`- quorum…`, `1. quorum…`, `git…`) dentro de un bloque ```` ```text ```` debe incluir `[ROOT]` o `[WORKTREE`. |

---

## 9. Ingesta al Flujo SDD (`/q-brief` la consume directo)

Tarea Quorum **`F-06`**. Todos los campos listos; no abrir entrevista.

- **task_id:** `F-06`
- **summary:** Add q-session skill to capture per-session decisions, patterns, and lessons into curated SQLite memory. Risk low.
- **goal:** Provide a human-invoked, single-phase `q-session` skill that distills durable decisions/patterns/lessons from the working session (not from a lifecycle task) and persists them as typed memory entries through the existing `quorum memory save` pipeline, using a `SESSION-YYYY-MM-DD` sentinel in `source_task`, with zero schema or SQLite changes.
- **invariants:**
  - `memory.schema.json` and the SQLite memory tables remain unchanged.
  - `quorum memory save` stays the only write path for curated memory.
  - Hash idempotency holds: same memory in the same session yields `unchanged`; different sessions never collide.
  - `q-session` is single-phase and terminal; it never auto-transitions or auto-chains and is absent from the authorized auto-transition table.
  - Capture stays human-invoked; no time/volume/background auto-capture.
  - Memory is persisted only after the human confirms the proposed candidates.
  - Persisted field values are written in concise English; user chat stays Spanish.
- **acceptance:**
  - AC-1 — `.agents/skills/q-session/SKILL.md` exists and passes every assertion in `go test ./internal/core -run TestSkillProtocol`.
  - AC-2 — The skill persists entries via `quorum memory save` with a `SESSION-`-prefixed `source_task`, and `quorum memory search` retrieves them.
  - AC-3 — A session memory with `source_task=SESSION-YYYY-MM-DD` saves as `inserted`, and re-saving the identical payload returns `unchanged`, with no schema or SQLite changes (covered by a test in `memory_service_test.go`).
  - AC-4 — `docs/adr/0007-captura-de-memoria-por-sesion.md` records the decision (reuse pipeline, sentinel `source_task`, single-phase, human-curated, no new type/table).
  - AC-5 — `go test ./...` passes.
- **non_goals:**
  - Do not build a reader skill (`q-recall`); search and the viewer already cover reads.
  - Do not emit narrative markdown handoffs.
  - Do not add a new memory type enum or table.
  - Do not add any auto-capture path.
- **constraints:**
  - No new runtime dependencies.
  - Skill name must be `q-session` (must carry the `q-` prefix).
  - Reuse the existing `quorum memory save` pipeline; no new Go production code beyond the skill, the ADR, and a test.
- **risk:** low

---

## 10. Trazabilidad de la Decisión

- **Objetivo conservado** (skill externo de session handoff): exprimir el conocimiento del diálogo humano↔agente para construir una BDD de conocimiento reutilizable a futuro.
- **Mecanismo adaptado:** memoria **tipada y curada en SQLite** vía `quorum memory save`, no archivos markdown narrativos, porque Quorum exige memoria tipada, validada y subordinada a la única ruta de escritura (Reglas #5 y gobernanza de memoria).
- **Honestidad técnica:** el único punto de fricción real (`source_task` required) se resuelve con un centinela porque el campo es string libre sin patrón; cero cambios de schema/SQLite y cero impacto en idempotencia/hash.
- **Constitucionalidad:** human-invoked (no auto-captura), single-phase (Regla #9), única ruta de escritura intacta (ADR 0006), curación humana obligatoria ("curated, never automatic").
- **Decisión cerrada:** solo escritor + solo memorias tipadas + centinela `SESSION-YYYY-MM-DD` + cero código Go de producción nuevo. Lectura y markdown narrativo rechazados explícitamente (§3).
