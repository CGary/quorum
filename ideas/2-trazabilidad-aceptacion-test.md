# 🔗 Propuesta Técnica: Trazabilidad Verificable Aceptación ↔ Test

**Estado:** Propuesta — eslabón 2 de 3 de la cadena ([1] estructura → este → [5] anti-vacuidad).
**Contexto:** Evolución de Quorum v1.2+. **Depende del doc [1]** (criterios con `id` estable).
**Origen:** Destilado del flujo de R.C. Martin (el Codificador deriva tests del spec). Se conserva el CONCEPTO de derivación trazable; no se adopta generación automática de tests (eso es el doc [5], decisión abierta).
**Veredicto:** Quorum YA intenta este cruce, pero como juicio blando del agente sin respaldo estructural ni CLI. Endurecerlo de "opinión" a "cobertura verificable" reutiliza un patrón que ya existe en el código (`coverageForItems`). Alto valor, bajo costo.

---

## 1. El Problema Real (acotado)

`q-analyze` ya declara este chequeo, pero sin dientes:

- Pass #5 "Cross-Artifact Consistency" (`q-analyze/SKILL.md:74`) marca *"acceptance criteria without test scenarios"* — pero es **juicio subjetivo del agente**, sin ID, sin CLI, sin determinismo.
- `blueprint.test_scenarios` (`blueprint.schema.json:44-50`) es `[]string` de prosa libre: no referencia qué criterio cubre cada escenario.
- `validation.schema.json` captura `command/exit_code/duration/output` (líneas 28-57) pero **no enlaza** ningún resultado a un criterio de aceptación.
- `review.schema.json` tiene `missing_tests: []string` (líneas 48-53), también prosa libre sin ID.

**Consecuencia:** una tarea puede llegar a `done` con `verify.commands` en verde (Regla #4 satisfecha) **sin que ningún test cubra un criterio de aceptación.** Los tests prueban *algo*, pero nadie verifica que prueban *lo que el spec pidió*. La Regla #8 ("tests son la única prueba") se cumple en la forma, no en el fondo.

---

## 2. Componentes a Implementar

### 2.1 Enlace explícito test_scenario → acceptance.id

**Qué hace:** cada escenario del blueprint puede declarar qué criterios de aceptación cubre.

```yaml
# 01-blueprint.yaml
test_scenarios:
  - statement: Unit test asserting CASH/QR/CARD selectable pre-completion.
    covers: [AC-1]
  - Fast lint pass over the changed POS module.   # forma string sigue válida
```

**Diseño técnico:**

| Aspecto | Decisión propuesta |
|---|---|
| Forma del item | `oneOf: [ string, object{statement, covers:[id]} ]`. Retrocompatible igual que el doc [1]. |
| Referencia | `covers[]` referencia `^AC-[0-9]+$` del `00-spec.yaml`. Match por **ID explícito**, NO léxico — más fuerte que el `coversItem()` actual. |
| Validación de integridad | Un `covers` que apunta a un `id` inexistente es un finding `high` de `q-analyze`. |

### 2.2 Comando `quorum analyze acceptance-coverage`

**Qué hace:** helper read-only que cruza `00-spec.yaml.acceptance[]` contra `01-blueprint.yaml.test_scenarios[].covers[]` y reporta criterios sin cubrir. **Espeja el patrón que YA existe** para padre↔hijo.

**Diseño técnico (reutiliza infraestructura existente):**

| Pieza nueva | Espejo existente |
|---|---|
| `internal/core/acceptance_coverage.go` | `internal/core/decomposition_analysis.go` (espeja el patrón con un struct propio `{item_id, statement, covered_by}` — `CoverageRow.Item` es un string plano y NO se reutiliza tal cual; `Finding` sí se reutiliza) |
| `cmd/analyze_acceptance_coverage.go` (shim stdin→JSON) | `cmd/analyze_decomposition_coverage.go` (copia casi 1:1) |
| Request: `{ "spec_path": "...", "blueprint_path": "..." }` | `AnalyzeDecompositionCoverageRequest` |
| Salida: `coverage[]{item_id, statement, covered_by[]}`, `gaps[]`, `status: pass\|issues_found\|blocked` | `DecompositionAnalysisResult` |

Lógica núcleo (pura, sin efectos, como todo `internal/core` analítico): para cada `acceptance.id`, buscar qué `test_scenarios[].covers` lo nombran; `covered_by == []` → gap `high`.

**Casos borde resueltos (no dejarlos a interpretación del implementador):**

- **Criterios `string` legacy (sin `id`):** se reportan con estado explícito `legacy_untracked`, NUNCA como `gap`. Un gap exige un `id` no cubierto; reportar legacy como gap genera ruido permanente, y excluirlo en silencio crea un punto ciego. El estado explícito deja la decisión visible al humano.
- **`id` duplicado en `acceptance[]`:** con duplicados la cobertura es indefinida. La unicidad la garantiza el finding `high` del doc [1] (JSON Schema no puede exigirla); si este comando los encuentra de todos modos, responde `status: blocked` en lugar de computar cobertura ambigua.
- **Blueprints de tareas hijo:** `covers[]` referencia SIEMPRE los `acceptance.id` del `00-spec.yaml` de la propia tarea. Tras `split` no hay ambigüedad entre hermanos porque el doc [1] (§2.2.1) obliga a `split` a despojar la forma objeto a `statement` plano: ningún hijo hereda ids del padre.

### 2.3 Endurecer `q-analyze` pass #5

**Qué hace:** sustituir el juicio blando de la línea 74 por el resultado determinista del comando 2.2.

**Cambios:**
- `q-analyze/SKILL.md` pass #5: invocar `quorum analyze acceptance-coverage` y reportar gaps como findings estructurados, igual que ya hace el pass #6 con `decomposition-coverage` (`q-analyze/SKILL.md:81-97`).
- Sigue siendo **read-only y advisory**: `q-analyze` no bloquea merges (eso lo decide el humano en `q-accept`). Mantiene la naturaleza read-only del skill.

---

## 3. Lo que NO se Implementará (límites explícitos)

| Componente rechazado | Razón |
|---|---|
| Ejecutar tests dentro de `acceptance-coverage` | Es analítico y read-only, como todo `quorum analyze`. Ejecutar tests es de `q-verify`. |
| Bloquear el merge por cobertura incompleta | `q-analyze` es advisory; la finalidad la da `verify.commands` (Regla #4) y el gate humano (Regla #6). Convertirlo en gate duro es decisión de [5], no de aquí. |
| Generación automática de tests desde el criterio | Es el corazón de la decisión abierta del doc [5]. No se asume aquí. |
| Nuevo artefacto numerado para la matriz de cobertura | Prohibido por el manifiesto (sin ADR). El cruce se computa on-demand, no se persiste. La traza vive en `feedback.json` si hay findings. |
| Match léxico como mecanismo primario | Teniendo `id` estable (doc 1), el ID explícito es superior. El léxico queda solo como fallback opcional para items `string` legacy. |

---

## 4. Dependencias y Orden

```text
[1] aceptación estructurada  →  [2] ESTE: trazabilidad a tests  →  [5] anti-vacuidad
     (provee los AC-id)            (provee el cruce id↔test)          (lo consume)
```

- **Bloqueado por [1]:** sin `acceptance.id` no hay a qué apuntar `covers[]`.
- **Habilita [5]:** la anti-vacuidad necesita saber qué test corresponde a qué criterio para poder mutar/verificar la relación.

---

## 5. Ingesta al Flujo SDD

Ingesta para `/q-brief`. Tarea Quorum sugerida `FEAT-COV-1`:

- **goal:** convertir el cruce aceptación↔test de juicio del agente a cobertura verificable por CLI, espejando `decomposition-coverage`.
- **invariants:** `blueprint.test_scenarios` `string`-only sigue válido; `quorum analyze` permanece read-only y puro; `go test ./...` verde.
- **acceptance:**
  - AC-1 — `quorum analyze acceptance-coverage` reporta como gap un criterio sin `covers`.
  - AC-2 — un `covers` que apunta a un `id` inexistente produce finding `high`.
  - AC-3 — `q-analyze` incorpora el resultado del comando en su reporte, sin persistir matriz nueva.
  - AC-4 — el comando no muta estado ni ejecuta tests.

---

## 6. Trazabilidad de la Decisión

- **Concepto conservado:** trazabilidad criterio→test (el Codificador de R.C. Martin deriva tests del spec; aquí lo hacemos auditable sin generarlos automáticamente).
- **Hallazgo clave:** el 70% de la infraestructura ya existe (`coverageForItems`, `CoverageRow`, el shim de `decomposition-coverage`, el pass #6 de `q-analyze`). Esto es **endurecer + espejar**, no construir de cero.
- **Rechazado:** ejecución de tests, gate de bloqueo, generación automática, artefacto persistido — todos fuera del alcance read-only/advisory de este eslabón.
- **Habilita:** doc [5], donde se decide si la cobertura se vuelve gate y si se introduce mutación.
