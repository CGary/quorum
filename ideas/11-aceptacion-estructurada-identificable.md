# 🎯 Propuesta Técnica: Criterios de Aceptación Estructurados e Identificables

**Estado:** Propuesta — eslabón 1 de 3 de la cadena de dependencias (este → [12] trazabilidad → [13] anti-vacuidad).
**Contexto:** Evolución de Quorum v1.2+.
**Origen:** Destilado del flujo "Disciplined Agentic Engineering" de R. C. Martin, separando el CONCEPTO (criterio de aceptación como transición de estado verificable) de la HERRAMIENTA (Gherkin/`.feature`/cucumber/godog). **No se adopta Gherkin ni ningún framework BDD.**
**Veredicto:** El campo `spec.acceptance` es hoy el eslabón menos formal de toda la cadena `00→07`. Estructurarlo e identificarlo es prerrequisito de cualquier trazabilidad o validación de tests posterior. Cambio acotado al artefacto `00-spec.yaml`, retrocompatible.

---

## 1. El Problema Real (acotado)

`spec.schema.json` define hoy (líneas 36-42):

```json
"acceptance": { "type": "array", "items": { "type": "string" }, "minItems": 1 }
```

Es decir: **un array de strings de prosa libre.** El template de `q-brief` lo confirma:

```yaml
acceptance:
  - User can select CASH, QR, or CARD before completing a sale.
```

Esto produce tres modos de fallo, todos vivos hoy:

| # | Modo de fallo | Evidencia en el código |
|---|---|---|
| 1 | **Tensión constitucional.** El manifiesto (línea 11) afirma "Agents Process Invariants, Not Stories", pero `acceptance` es literalmente prosa narrativa libre — el formato que el manifiesto dice rechazar. | `spec.schema.json:36-42` |
| 2 | **Sin identidad estable.** Un criterio no tiene ID. No se puede referenciar desde un test, un commit, una validación ni una mutación. Cualquier cruce posterior depende de match léxico frágil. | `coversItem()` en `decomposition_analysis.go:345` usa `strings.Contains` sobre texto normalizado |
| 3 | **Sin estructura de verificación.** Un criterio no distingue precondición, acción y resultado esperado, así que "verificable externamente" queda a juicio del lector. `q-analyze` pass #2 (línea 44) lo evalúa de forma puramente subjetiva. | `q-analyze/SKILL.md:44` |

---

## 2. Componentes a Implementar

### 2.1 Criterio de aceptación como unidad identificable y estructurada

**Qué hace:** cada item de `acceptance` deja de ser un string suelto y pasa a ser una unidad con identidad y forma de transición de estado.

**Decisión de diseño (RESUELTA vía concepto de R.C. Martin):** la "transición de FSM" Given/When/Then es el CONCEPTO valioso de Gherkin. Se adopta como **tres campos de texto plano** (`given`, `when`, `then`), NO como sintaxis Gherkin ni archivo `.feature`. Sin parser, sin DSL, sin dependencia de framework. El YAML sigue siendo el artefacto machine-first que manda la Regla #5.

```yaml
acceptance:
  - id: AC-1
    statement: User can select CASH, QR, or CARD before completing a sale.
    given: an open POS Express sale screen with no payment selected
    when: the user picks a payment method
    then: the selected method is stored on the sale aggregate before completion
  - id: AC-2
    statement: Existing sale flow is unchanged when the user does not interact.
```

**Diseño técnico:**

| Aspecto | Decisión propuesta |
|---|---|
| Forma del item | `oneOf: [ string, object ]`. El `object` requiere `id` + `statement`; `given`/`when`/`then` son opcionales (transición explícita solo cuando aporta). |
| Patrón de `id` | `^AC-[0-9]+$`. Estable dentro del spec; nunca se renumera (igual filosofía append-only que `07-trace`). |
| Retrocompatibilidad | El `string` suelto sigue siendo válido. **No rompe** ningún spec en `.ai/tasks/done/` ni los golden-master de `internal/core/golden_master_test.go`. RESUELTA: aditiva/opt-in, no migración forzada. |
| Endurecimiento futuro | Cuando ≥80% de specs nuevos usen la forma estructurada, un ADR posterior puede volver `object` obligatorio. No ahora. |

**Cambios requeridos:**
- `spec.schema.json`: `acceptance.items` pasa de `{ "type": "string" }` a `oneOf: [string, object{id, statement, given?, when?, then?}]` con `additionalProperties: false` en el object.
- `q-brief/SKILL.md`: la Fase 2 (Logical Interview) pregunta por `id` + `given/when/then` cuando el criterio lo amerite. **Se relaja la regla de línea 76** ("Do NOT invent new YAML keys") exclusivamente para esta estructura ya schematizada — no es invención, es schema.
- Template `00-spec.yaml` y ejemplo embebido en `q-brief/SKILL.md:61`: mostrar ambas formas.

---

## 3. Lo que NO se Implementará (límites explícitos)

| Componente rechazado | Razón |
|---|---|
| Sintaxis Gherkin / archivos `.feature` | Acopla Quorum a una dialéctica BDD. La transición de estado se expresa en campos YAML planos. La herramienta se rechaza; el concepto se conserva. |
| Parser de Gherkin / motor FSM en Go | No hay nada que parsear: `given/when/then` son texto para el humano y el agente, no entradas de un motor. |
| Migración forzada de specs existentes | `oneOf` preserva la forma `string`. Romper specs en `done/` invalidaría golden-masters sin beneficio. |
| `object` obligatorio desde el día 1 | Fricción innecesaria en `q-brief` antes de tener evidencia de adopción. Diferido a ADR. |
| Tocar `02-contract.yaml` o cualquier artefacto de captura (`05/06/07`) | Este eslabón vive solo en `00-spec.yaml`. La trazabilidad a tests es responsabilidad del doc [12]. |

---

## 4. Dependencias y Orden

```text
[11] aceptación estructurada  →  [12] trazabilidad a tests  →  [13] anti-vacuidad
     (ESTE doc: la identidad)      (necesita los id)             (necesita el cruce)
```

Este doc es el cimiento: sin `id` estable no hay a qué enlazar un test (doc 12) ni qué mutar/verificar (doc 13). **Implementar primero, en su propia tarea Quorum.**

---

## 5. Ingesta al Flujo SDD

Este documento es ingesta para `/q-brief`. La tarea Quorum resultante (sugerido `FEAT-ACC-1` o el ID que asigne el orquestador):

- **goal:** dar identidad y estructura de transición a los criterios de aceptación en `00-spec.yaml`, sin acoplar a Gherkin.
- **invariants:** specs `string`-only existentes siguen validando; `go test ./...` permanece verde; ningún artefacto fuera de `00-spec.yaml` cambia su schema.
- **acceptance (dogfooding de sí mismo):**
  - AC-1 — `spec.schema.json` acepta criterios `object` con `id`+`statement` Y sigue aceptando `string`.
  - AC-2 — `q-brief` genera la forma estructurada cuando el usuario aporta given/when/then.
  - AC-3 — un spec con `acceptance` mixto (strings + objects) pasa `quorum validate`.

---

## 6. Trazabilidad de la Decisión

- **Propuesta original (R.C. Martin):** Gherkin como lenguaje formal de especificación que restringe a la IA, con triplets Given/When/Then como transiciones de FSM.
- **Filtro de factibilidad contra Quorum:** se conserva el concepto (criterio = transición de estado identificable); se rechaza la herramienta (Gherkin, `.feature`, parser). Motivo: Quorum es notación-agnóstico por diseño y machine-first (Regla #5); embeber prosa Gherkin en un artefacto violaría esa regla.
- **Acción inmediata:** este eslabón (estructura + identidad), retrocompatible, una sola tarea Quorum.
- **Habilita:** doc [12] (trazabilidad) y doc [13] (anti-vacuidad), que sin `id` estable son imposibles.
