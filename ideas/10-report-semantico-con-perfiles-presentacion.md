# Propuesta técnica: Reportes semánticos con perfiles de presentación

> **Estado de implementación (2026-06-06):** Implementada en su forma SEMÁNTICA (Fases 1 y 2), con una desviación registrada en **`docs/adr/0005-modelo-de-reporte-semantico-puro.md`**: el modelo legacy fue ELIMINADO (no se mantuvo la coexistencia). Quedan **obsoletas** las secciones que asumen unión exclusiva legacy↔semantic: §4.2, §8 Fase 1.3 (coherencia modelo↔versión / autofill `1.0`) y las decisiones **K, Z, I (mitad legacy), A, F** de §15. La **§8 Fase 4 (migración legacy)** también queda DESCARTADA: el comando `quorum report migrate` fue eliminado por ser incoherente con el mandato no-legacy (ver ADR-0005 punto 4). El resto (modelo semántico, roles, perfiles, viewer, validación Go) sigue vigente. Ver ADR-0005 para el detalle.

**Estado:** Idea implementable.
**Prioridad:** Alta para cerrar la brecha con `cognitive-report` sin seguir agregando componentes top-level ad hoc.
**Tipo:** Evolución de schema + viewer + skill protocol para `q-report`.
**Alcance inicial:** `.agents/schemas/report.schema.json`, `.agents/skills/q-report/SKILL.md`, `.agents/templates/report.yaml`, `internal/server/web/app.js`, `internal/server/web/style.css`, `cmd/report.go`, tests de schema/protocolo/render.
**Motivación inmediata:** Los reportes generados con `/home/gary/Downloads/files/cognitive-report.skill` se sienten con más resolución que `q-report` + `quorum serve start` porque el primero tiene plantillas narrativas, renderer visual y bloques de análisis más ricos. Agregar algunos bloques narrativos resuelve el problema de hoy, pero no abstrae el problema para futuros tipos de reporte.

---

## 1. Problema

`q-report` hoy modela el reporte como una lista cerrada de componentes visuales de primer nivel:

- `verdict`
- `summary`
- `decisionSurface`
- `callouts`
- `verify`
- `keyFindings`
- `diagrams`
- `findings`
- `evidence`
- `tradeoffs`
- `risks`
- `actionPlan`
- `appendix`

Ese modelo es estable y validable, pero acopla tres responsabilidades:

1. **Semántica:** qué intención comunicativa tiene una parte del reporte.
2. **Estructura:** qué campos YAML son válidos.
3. **Presentación:** qué renderer visual usa el servidor web.

Como consecuencia, cada necesidad nueva tiende a convertirse en un nuevo componente top-level. Ejemplos probables:

- `analysis`
- `bars`
- `confidence`
- `title`
- `kicker`
- `customTitles`
- `layout`
- `audience`
- `detailLevel`
- `teachingSteps`
- `executiveSummary`

Ese camino escala mal: el schema crece como catálogo visual, no como lenguaje semántico.

---

## 2. Objetivo

Introducir un grado de indirección para que un reporte diga primero **qué quiere comunicar** y solo después **cómo debe verse**.

La propuesta agrega un modelo canónico basado en:

```yaml
kind: project_usage | refactor_plan | refactor_result | audit | decision_brief | technical_analysis | generic
presentation:
  profile: cognitive | executive | audit | teaching | raw
  density: low | medium | high
  audience: engineer | maintainer | reviewer | manager | user
content:
  title: string
  kicker: string
  verdict: object
  sections: list[semantic_section]
```

El renderer decide cómo mostrar cada sección según:

- `kind`: tipo de reporte;
- `presentation.profile`: estrategia visual;
- `presentation.density`: compactación/detalle;
- `content.sections[].role`: intención comunicativa de cada sección.

---

## 3. Principio de diseño

**El reporte debe ser un documento semántico, no una plantilla visual.**

Un bloque no debería llamarse `keyFindings` porque el renderer lo pinta como tabla. Debería existir una sección con `role: findings`; el renderer decide si esa sección va como tabla, cards, lista compacta o detalle colapsable.

La unidad estable a largo plazo es el `role`, no el componente visual.

---

## 4. Modelo propuesto

### 4.1 Forma canónica nueva

```yaml
meta:
  id: quorum-usage
  schemaVersion: "1.1"
  date: "2026-06-03T15:00:00Z"

kind: project_usage

presentation:
  profile: cognitive
  density: high
  audience: engineer
  language: es

content:
  title: "Cómo correr y trabajar con Quorum"
  kicker: "Guía técnica de uso"
  verdict:
    text: "Quorum organiza cambios mediante contratos SDC validados, no mediante conversación libre."
    confidence: high

  sections:
    - id: at-a-glance
      role: decision_surface
      title: "De un vistazo"
      body:
        recommendation: "Usar el ciclo q-brief → q-blueprint → q-implement → q-verify → q-review."
        mainRisk: "Saltar el contrato y tocar archivos fuera de scope."
        bestNextAction: "Inicializar con quorum init y crear la primera tarea."

    - id: verify-first
      role: verification
      title: "Verificá esto primero"
      items:
        - what: "Contrato CLI actual"
          why: "Documentos antiguos pueden describir entrypoints Python obsoletos."
          check: "go test ./... && ./quorum --help"

    - id: lifecycle
      role: analysis
      title: "Arquitectura: ciclo 00→07"
      body: |
        Quorum convierte intención humana en artefactos validados. Cada fase produce un archivo con schema propio y el avance se controla por comandos CLI.
      details:
        label: "Ver artefactos"
        body: |
          00-spec.yaml: intención y criterios.
          01-blueprint.yaml: plan técnico.
          02-contract.yaml: límites de modificación.
          05-validation.json: evidencia de comandos.
          06-review.json: revisión.
          07-trace.json: historial append-only.

    - id: flow
      role: diagram
      title: "Flujo de ejecución"
      diagram:
        type: mermaid
        code: |
          flowchart LR
            A[q-brief] --> B[q-blueprint]
            B --> C[q-implement]
            C --> D[q-verify]
            D --> E[q-review]
            E --> F[q-accept]

    - id: next-steps
      role: action_plan
      title: "Próximos pasos"
      items:
        - step: 1
          action: "Ejecutar quorum init en el proyecto destino."
          owner: "human"
        - step: 2
          action: "Crear la primera especificación con /q-brief."
          owner: "agent"
```

### 4.2 Compatibilidad con reportes actuales

**Terminología (dos "legacy" distintos):** a lo largo del documento conviven dos conceptos que NO deben confundirse:

- **`legacy report model`** — el schema de datos viejo: componentes top-level (`verdict` string, `keyFindings`, `decisionSurface`, etc.). Es un MODELO de datos.
- **`legacy report-new direct-write path`** — el comportamiento de CLI por el cual `quorum report new` sin `--output` escribe directo en `.ai/reports/` (§8 Fase 3, decisión H). Es un COMPORTAMIENTO de CLI.

Cuando esta propuesta dice "legacy" sin calificar, se refiere al **modelo** salvo en §8 Fase 3 / decisión H, que hablan del **path de escritura**.

---

Los campos top-level actuales NO deben romperse en la primera iteración, pero tampoco deben poder mezclarse silenciosamente con el modelo semántico. La compatibilidad se implementa como **unión exclusiva de modelos**:

- **`legacy report model`:** permite los campos top-level actuales y prohíbe `kind`, `presentation` y `content`.
- **Modelo semántico:** requiere `kind`, `presentation` y `content`, y prohíbe todos los componentes del `legacy report model` (`verdict`, `summary`, `decisionSurface`, `callouts`, `verify`, `keyFindings`, `diagrams`, `findings`, `evidence`, `tradeoffs`, `risks`, `actionPlan`, `appendix`).
- Un archivo que mezcle legacy + semantic debe fallar validación antes de escribirse.
- `content.sections` debe tener `minItems: 1`. Un reporte semántico sin secciones no es válido.

**Mecanismo de discriminación raíz (obligatorio):** la unión exclusiva NO debe implementarse con `oneOf: [legacy, semantic]` en la raíz. Con `oneOf`, un reporte semántico con un error real falla contra AMBAS ramas y `internal/core/schema.go` (`chooseError`) puede elegir la hoja de la rama legacy (por ejemplo `'content' was unexpected`) en vez del error semántico real, degradando los mensajes `field=...; reason=...`. Es el mismo problema que §6.2 evita para roles, pero a nivel de modelo.

La condición NO debe discriminar solo por `content`. Si discriminara solo por `content`, un reporte semántico al que le falta `content` (typo, o solo trae `kind`/`presentation`) caería al `legacy report model`, que prohíbe `kind`/`presentation` → el error sería `'kind' was unexpected` en vez del correcto `'content' is a required property`. El discriminador debe dispararse ante **cualquier** marcador semántico (los tres están prohibidos en legacy), de modo que cualquier intención semántica rutee al branch semántico y reporte el `required` faltante real:

```json
{
  "if": {
    "anyOf": [
      { "required": ["content"] },
      { "required": ["kind"] },
      { "required": ["presentation"] }
    ]
  },
  "then": { "$ref": "#/$defs/semanticModel" },
  "else": { "$ref": "#/$defs/legacyModel" }
}
```

- `legacyModel` define las propiedades top-level actuales y prohíbe `kind`, `presentation`, `content`.
- `semanticModel` requiere `kind`, `presentation`, `content` y prohíbe los componentes del `legacy report model`.

Esto NO choca con la detección del viewer (`Boolean(data.content)`, §8 Fase 2): el viewer solo recibe payloads ya validados, donde un payload semántico siempre tiene `content`. Discriminador (robusto a errores, en validación) y detección de viewer (post-validación) son deliberadamente distintos.

**Deprecación:** la ventana de compatibilidad legacy dura exactamente hasta `schemaVersion: "2.0"`. En v1.1 y v1.x se mantiene lectura/render legacy. En v2.0 se puede retirar el modelo legacy con ADR o tarea de migración explícita.

Esto permite migración gradual de `.ai/reports/*.yaml` existentes sin duplicar indefinidamente la semántica ni permitir pérdida silenciosa de datos.

---

## 5. Campos exactos

### 5.1 `kind`

Campo obligatorio para el modelo semántico. No hay default implícito en el schema: los defaults solo pueden aparecer en scaffolds/templates generados por `quorum report new`. Si falta `kind` en un payload semántico, la validación debe fallar.

Valores permitidos:

| Valor | Uso |
|---|---|
| `generic` | Reporte técnico general. |
| `project_usage` | Guía de uso/onboarding de proyecto. |
| `refactor_plan` | Plan antes de cambiar código. |
| `refactor_result` | Resultado después de una refactorización. |
| `audit` | Revisión con hallazgos/evidencia/severidad. |
| `decision_brief` | Documento corto para tomar una decisión. |
| `technical_analysis` | Análisis narrativo técnico con evidencia. |

### 5.2 `presentation`

Campo obligatorio para el modelo semántico. No hay defaults implícitos en el schema: los defaults solo pueden aparecer en scaffolds/templates generados por `quorum report new`. Si falta `presentation` en un payload semántico, la validación debe fallar.

Template recomendado:

```yaml
presentation:
  profile: cognitive
  density: medium
  audience: engineer
  language: es
```

Schema:

```yaml
presentation:
  profile: cognitive | executive | audit | teaching | raw
  density: low | medium | high
  audience: engineer | maintainer | reviewer | manager | user
  language: es | en
```

Los cuatro subcampos (`profile`, `density`, `audience`, `language`) son **obligatorios** dentro de `presentation` (`required: [profile, density, audience, language]`, `additionalProperties: false`). No son opcionales con default implícito: el renderer depende de valores explícitos (p. ej. `density` decide colapsado en §7.1) y los defaults están prohibidos en schema/renderer (decisión S). Los valores por defecto solo se materializan en el scaffold de `quorum report new`.

Semántica:

| Campo | Decisión |
|---|---|
| `profile` | Selecciona orden visual, énfasis y renderers preferidos. |
| `density` | Controla cuánto texto se muestra expandido por defecto. |
| `audience` | Ajusta etiquetas y prioridad visual; no cambia la verdad del contenido. |
| `language` | Idioma de labels generados por el viewer. Fase 1 solo admite `es` o `en`; no hay autodetección para evitar comportamiento no determinista. |

### 5.3 `content`

Campo obligatorio para el modelo semántico. Está prohibido en el modelo legacy.

```yaml
content:
  title: string
  kicker: string optional
  summary: string optional
  verdict:
    text: string
    confidence: high | medium | low optional
  sections: SemanticSection[]
```

Campos obligatorios de `content` para el modelo semántico:

```yaml
required: [title, verdict, sections]
```

Reglas:

- `content.title` debe mostrarse como título humano del reporte.
- `meta.id` sigue siendo la identidad persistente y nombre de archivo.
- `content.verdict.text` reemplaza semánticamente al legacy `verdict` string.
- `content.sections` es la única fuente del cuerpo semántico.
- `content.sections` debe tener al menos 1 item.
- `content.summary` y `content.verdict` son campos fijos del encabezado semántico; no son roles y no forman parte de `sections[]`.

### 5.4 `SemanticSection`

**Esta forma es ILUSTRATIVA, no autoritativa.** Lista el universo de campos posibles para legibilidad, pero la fuente de verdad es el `$def` por role de §6.2: cada role fija su propio `required` y `additionalProperties: false`. Un campo de esta lista solo es válido en una sección si el `$def` de ese role lo declara. En particular `severity` NO es un campo genérico de toda sección; solo existe donde el role lo declara (ver §6.1).

```yaml
- id: string                  # kebab-case; único dentro del reporte
  role: string                # enum, ver tabla §6.1
  title: string
  body: string | object       # forma depende del role
  items: array                # forma depende del role
  details: object             # solo roles que lo declaren; forma cerrada {label?: string, body: string}
  diagram: object             # solo role: diagram; Fase 1 solo permite {type: mermaid, code: string}
```

Reglas generales:

- `id` debe matchear `^[a-z0-9][a-z0-9-]*$`.
- `id` debe ser único dentro de `content.sections`. Esta unicidad NO se puede garantizar solo con JSON Schema porque aplica a un subcampo de objetos dentro de un array; debe validarse en Go en el hook post-schema descrito en §8 Fase 1.2 (dentro de `ValidateAgainstSchema`, `schema.go:30`), no en el comando `report save`. Si viviera solo en `report save`, un archivo con IDs duplicados pasaría `quorum validate --schema report` pero fallaría al persistir — incoherencia. Debe fallar idéntico en ambos caminos, con campo preciso (`field=$.content.sections[2].id; reason=duplicate section id "flow"`).
- `role` determina qué combinaciones de campos son válidas (§6.2 autoritativo).
- `title` es obligatorio para navegación.
- El viewer debe ignorar campos opcionales ausentes, pero el schema debe rechazar campos fuera del catálogo del role.

**Enum de severidad/impacto:** los campos que el viewer pinta como pills de color (`findings.items[].severity`, `risks.items[].impact`) deben usar un dominio cerrado, no `string` libre. Valores permitidos: `low | medium | high | critical`. Sin enum, el color del pill es indefinido y la IA puede inyectar strings arbitrarios.

**Schema común cerrado:**

```yaml
details:
  label: string optional
  body: string required

severity_or_impact: low | medium | high | critical
```

---

## 6. Roles semánticos

### 6.1 Catálogo inicial

| Role | Campos válidos | Render por defecto |
|---|---|---|
| `decision_surface` | `body: object` con `additionalProperties: string` | Key-value table/card. |
| `verification` | `items: [{what, why, check}]` con `minItems: 1, maxItems: 4` (paridad con el legacy `verify`: 1-4 checks para reducir fatiga) | Tabla destacada cerca del inicio. |
| `findings` | `items: [{id, finding, why?, action?, severity?}]` (`id` kebab-case, `severity ∈ low\|medium\|high\|critical`) | Tabla escaneable. |
| `analysis` | `body: string`, `details?: {label?: string, body: string}` | Prosa corta + details colapsable. |
| `diagram` | `diagram: {type: mermaid, code: string}` | Mermaid. Fase 1 no admite otros renderers. |
| `tradeoffs` | `items: [{option, upside?, downside?, useWhen?, avoidWhen?}]` | Tabla comparativa. |
| `risks` | `items: [{risk, signal?, impact, mitigation?}]` (`impact ∈ low\|medium\|high\|critical`) | Tabla con pill de impacto. |
| `action_plan` | `items: [{step: integer, action: string, owner: string}]` | Tabla ordenada. |
| `evidence` | `items: [{findingId?, path?, details}]` (`findingId?` referencia `findings.items[].id`; opcional para permitir evidencia autónoma) | Tabla/links. |
| `appendix` | `body: string` | `<details>` colapsado por defecto. |
| `metrics` | `items: [{label: string, value: number, unit?: string, display?: string}]` | Barras o tabla según profile. |
| `callout` | `body: string`, `kind: decision|warning|note`, `label?` | Caja visual decision/warning/note. |

### 6.2 Validación por role

No usar `oneOf` para discriminar roles. Con muchos roles y `const`, una sección inválida falla contra todas las ramas y `internal/core/schema.go` puede elegir una hoja de error no relacionada, degradando los mensajes Python-compatible `field=...; reason=...`.

**Enum explícito de `role` (obligatorio).** La base de sección DEBE declarar `role: { "enum": [<todos los roles>] }`. No es opcional ni redundante: el mecanismo `if`/`then` de abajo es **abierto por construcción**. Si una sección trae `role: "foobar"`, NINGUNA condición `if (role == X)` matchea → NINGÚN `then` aplica → no se impone restricción alguna → la sección con role inválido **validaría**. Sin el enum en la base, el test #4 ("role desconocido falla") no pasa. El enum es lo que cierra el universo de roles; el `if`/`then` solo valida la forma de cada role conocido.

Usar validación condicional `if`/`then`/`else` basada en `role`, o una cadena `allOf` de condicionales por role. La forma requerida es:

```json
{
  "allOf": [
    {
      "if": { "properties": { "role": { "const": "analysis" } }, "required": ["role"] },
      "then": { "$ref": "#/$defs/analysisSection" }
    },
    {
      "if": { "properties": { "role": { "const": "verification" } }, "required": ["role"] },
      "then": { "$ref": "#/$defs/verificationSection" }
    }
  ]
}
```

Cada `$defs/*Section` debe fijar:

- `role` con `const`;
- `required` específico;
- `additionalProperties: false`.

Ejemplo para `analysis`:

```json
{
  "type": "object",
  "properties": {
    "id": { "$ref": "#/$defs/sectionID" },
    "role": { "const": "analysis" },
    "title": { "type": "string" },
    "body": { "type": "string" },
    "details": { "$ref": "#/$defs/details" }
  },
  "required": ["id", "role", "title", "body"],
  "additionalProperties": false
}
```

**Referencia evidence↔findings:** JSON Schema puede validar la forma de `findingId`, pero no la integridad referencial. Se valida en Go, en el mismo helper de validaciones semánticas que valida unicidad de `content.sections[].id`. La integridad tiene dos invariantes:

1. **Unicidad de finding ids.** Todos los `findings.items[].id` deben ser únicos **dentro del reporte** (no solo dentro de una sección `findings`: puede haber varias). Sin esto, un `findingId` referenciado resolvería a múltiples targets y el enlace visual sería no determinista. Si hay colisión, fallar con `field=$.content.sections[N].items[M].id; reason=duplicate finding id "..."`.
2. **Existencia.** `findingId` es **opcional** (ver §6.1): una evidencia puede ser autónoma (un `path`/`details` sin finding formal asociado, p. ej. en `technical_analysis`). Pero cuando `findingId` está presente, debe existir entre los `id` de todos los items de secciones `findings`; si no existe, fallar con `field=$.content.sections[N].items[M].findingId; reason=unknown finding id "..."`.

Ambos invariantes viven en el motor (`schema.go`), no en el comando, para que `validate --schema report` y `report save` fallen idéntico (ver §5.4).

---

## 7. Perfiles de presentación

Los perfiles NO cambian datos. Solo cambian orden, énfasis y widgets visuales.

**Determinismo de `orderSections` (obligatorio):** el orden preferido de cada perfil es un subconjunto; un perfil puede no enumerar todos los roles. La función debe ser determinista:

- Las secciones cuyo role aparece en el orden preferido se ubican según ese orden.
- Los roles NO enumerados ("resto") van después, en **orden de autoría** de `content.sections[]`.
- Dos secciones del mismo role mantienen su orden de autoría relativo (sort estable).

Sin esta regla, el render de roles no listados (o de secciones repetidas del mismo role) sería no determinista.

### 7.1 `cognitive`

Objetivo: reducir fatiga cognitiva.

Render fijo antes de ordenar secciones:

1. `content.verdict`
2. `content.summary` si existe

Orden preferido para `content.sections[]`:

1. `decision_surface`
2. `verification`
3. `callout`
4. `findings`
5. `diagram`
6. `analysis`
7. `tradeoffs`
8. `risks`
9. `action_plan`
10. `evidence`
11. `metrics`
12. `appendix`

Reglas visuales:

- `verification` siempre arriba y con estilo de advertencia.
- `appendix` colapsado.
- `analysis.details` colapsado si `density` no es `high`.
- Tablas para comparación.
- Mermaid para relaciones.

### 7.2 `executive`

Objetivo: decisión rápida.

Render fijo antes de ordenar secciones:

1. `content.verdict`
2. `content.summary` si existe

Orden preferido para `content.sections[]`:

1. `decision_surface`
2. `risks`
3. `tradeoffs`
4. `action_plan`
5. roles no enumerados al final, en orden de autoría, colapsados por defecto.

Reglas visuales:

- Mostrar máximo 5 secciones expandidas.
- `analysis` colapsado por defecto.
- `evidence` colapsado por defecto.

### 7.3 `audit`

Objetivo: revisión trazable.

Render fijo antes de ordenar secciones:

1. `content.verdict`
2. `content.summary` si existe

Orden preferido para `content.sections[]`:

1. `verification`
2. `findings`
3. `evidence`
4. `risks`
5. `action_plan`
6. `appendix`

Reglas visuales:

- `severity` e `impact` como pills.
- `evidence.items[].findingId` debe enlazar visualmente con `findings.items[].id`; la integridad se valida en Go (§6.2).

### 7.4 `teaching`

Objetivo: aprendizaje guiado.

Render fijo antes de ordenar secciones:

1. `content.verdict`
2. `content.summary` si existe

Orden preferido para `content.sections[]`:

1. `diagram`
2. `analysis`
3. `action_plan`
4. `verification`
5. `appendix`

Reglas visuales:

- `analysis` expandido por defecto.
- `details` puede estar abierto si `density: high`.

### 7.5 `raw`

Objetivo: máxima fidelidad técnica.

Reglas:

- Renderizar primero `content.verdict` y `content.summary` si existen.
- Respetar orden de autoría de `content.sections`.
- No colapsar salvo `appendix`.
- Menor transformación visual.

---

## 8. Estrategia de implementación

### Fase 1: Modelo nuevo sin romper legacy

Cambios:

1. Actualizar `.agents/schemas/report.schema.json`:
   - mantener un branch legacy para propiedades actuales;
   - agregar un branch semántico con `kind`, `presentation`, `content`;
   - hacer los branches mutuamente exclusivos: legacy prohíbe `content`; semantic prohíbe componentes legacy top-level;
   - agregar `$defs` para roles semánticos;
   - validar roles con `if`/`then`, no con `oneOf`;
   - agregar `minItems: 1` a `content.sections`;
   - requerir `kind`, `presentation` (con sus cuatro subcampos `profile`/`density`/`audience`/`language`), `content.title`, `content.verdict` y `content.sections` en el modelo semántico;
   - restringir `meta.schemaVersion` semántico a `"1.1"` exacto;
   - mantener `additionalProperties: false` en raíz y secciones.
2. Validación Go de invariantes no expresables en JSON Schema. **Ubicación exacta del hook:** dentro de `ValidateAgainstSchema` (`internal/core/schema.go:30`), DESPUÉS de que `schema.Validate(payload)` retorna sin error y ANTES del `return nil`, gateado por `schemaName == "report.schema.json"`. Es el único choke point que ambos caminos atraviesan: `report save` → `SaveArtifact` (`artifact.go:80`) → `ValidateArtifact` (`schema.go:46`) → `ValidateAgainstSchema`; y `validate --schema report` (`cmd/validate.go`) → `ValidateAgainstSchema` directo. Por eso NO debe vivir en el comando `report save` (ver §5.4): ahí `validate` lo saltearía. Preferir un pequeño registry `map[string]func(payload any) error` de validadores post-schema por nombre de schema, en vez de un `if` hardcodeado, para que sea extensible; el gate por nombre evita que corra para spec/blueprint/etc. Invariantes a chequear:
   - detectar IDs duplicados en `content.sections[].id`;
   - detectar IDs duplicados en `findings.items[].id` a través de todas las secciones `findings` del reporte (§6.2 invariante 1);
   - validar integridad `evidence.items[].findingId` → `findings.items[].id` solo cuando `findingId` esté presente (§6.2 invariante 2);
   - devolver `ArtifactValidationError` con campo preciso, por ejemplo `field=$.content.sections[2].id; reason=duplicate section id "flow"`.
3. Actualizar metadata (`fillReportMetadata`, corre antes de validar):
   - reportes legacy (sin `content`): default `schemaVersion: "1.0"` cuando se omite;
   - payload semántico (`content` presente): autofill `schemaVersion: "1.1"` cuando se omite;
   - **cerrar el hueco de coherencia:** `fillReportMetadata` solo rellena cuando el campo está vacío. Un archivo legacy con `schemaVersion: "1.0"` literal al que se le agrega `content` quedaría persistido como semántico marcado "1.0". Por eso Fase 1 debe RECHAZAR todo payload semántico cuyo `meta.schemaVersion` no sea exactamente `"1.1"` (no basta con el autofill). No implementar comparación semver en Fase 1; versiones futuras requieren cambio explícito de schema;
   - `schemaVersion` no decide el renderer; el modelo lo decide la presencia válida de `content`, pero la versión debe reflejar el formato persistido.
4. Actualizar `cmd/report.go`:
   - no cambiar persistencia;
   - no cambiar paths existentes;
   - validar con schema nuevo + validación Go de unicidad de secciones.
5. Actualizar `.agents/templates/report.yaml` (decisión: el seed pasa a ser **semántico**, coherente con W):
   - el seed debe ser un reporte semántico VÁLIDO por construcción, con TODOS los required nuevos (`kind`, `presentation` completo, `content.title`, `content.verdict`, `content.sections` con ≥1 item) y `meta.schemaVersion: "1.1"` exacto. Esto es obligatorio porque el test existente (`report_test.go`, "el seed valida contra report.schema.json") seguirá corriendo;
   - el menú legacy se conserva como comentario, no como YAML activo, para no violar la unión exclusiva (§4.2);
   - el menú comentado debe nombrar `kind`, `presentation`, `content` y cada role, para satisfacer también `TestReportCatalogDocsInSyncWithSchema` (§11.3, J).
6. Actualizar `.agents/skills/q-report/SKILL.md`:
   - instruir a preferir el modelo nuevo;
   - mantener legacy solo para reportes mínimos;
   - agregar selección de `kind` y `presentation.profile`;
   - documentar `kind`, `presentation`, `content` y cada role semántico en el catálogo, y reflejarlos en el menú de `report.yaml`, para satisfacer `TestReportCatalogDocsInSyncWithSchema` (ver §11.3).

Criterio de éxito:

- Un reporte legacy existente valida.
- Un reporte semántico nuevo valida.
- `quorum report save <id>` funciona para ambos.

### Fase 2: Viewer semántico

Cambios en `internal/server/web/app.js`:

1. Implementar detección por presencia de `content`, no por `content.sections`, porque el schema ya garantiza que un payload semántico válido tiene secciones. Si `content` existe pero es inválido, la API debe haber rechazado el reporte antes del viewer.

```js
const isSemanticReport = Boolean(data.content);
if (isSemanticReport) renderSemanticReport(reportId, data);
else renderLegacyReport(reportId, data);
```

2. Implementar:

```js
function renderSemanticReport(reportId, data) {}
function renderSemanticHeader(content, presentation) {}
function orderSections(sections, profile) {}
function renderSemanticSection(section, presentation) {}
```

`renderSemanticHeader` renderiza `content.title`, `content.kicker`, `content.verdict` y `content.summary`. `orderSections` solo reordena `content.sections[]`.

3. Crear mapa de renderers por role:

```js
const ROLE_RENDERERS = {
  decision_surface: renderDecisionSurfaceSection,
  verification: renderVerificationSection,
  findings: renderFindingsSection,
  analysis: renderAnalysisSection,
  diagram: renderDiagramSection,
  tradeoffs: renderTradeoffsSection,
  risks: renderRisksSection,
  action_plan: renderActionPlanSection,
  evidence: renderEvidenceSection,
  appendix: renderAppendixSection,
  metrics: renderMetricsSection,
  callout: renderCalloutSection
};
```

4. Reutilizar helpers legacy donde sea posible, pero sin duplicar lógica innecesaria: los renderers legacy deben convertirse en wrappers del renderer por role cuando la forma de datos sea equivalente. Durante compatibilidad hay dos adaptadores de entrada, no dos implementaciones visuales completas por componente.

   - `appendTable`
   - `renderKeyValue`
   - `renderDiagrams`
   - `renderAppendix`

Criterio de éxito:

- El viewer muestra `content.title` como H1.
- `presentation.profile` cambia el orden visual.
- `verification` aparece cerca del inicio en `cognitive`.
- `analysis.details` se renderiza como `<details>`.
- Reportes legacy siguen renderizando igual.

### Fase 3: Plantillas por `kind`

Nota de compatibilidad CLI: el código actual permite `quorum report new <id>` sin `--output` y escribe directamente en `.ai/reports/` vía `SaveArtifact`. Esta propuesta NO corrige ese comportamiento en Fase 3 para evitar un breaking change. La regla operativa para `q-report` sigue siendo usar `--output .tmp/<id>.yaml` + `report save`; el CLI legacy puede permanecer hasta una decisión separada.


Agregar templates semánticos:

```text
.agents/templates/reports/project_usage.yaml
.agents/templates/reports/refactor_plan.yaml
.agents/templates/reports/refactor_result.yaml
.agents/templates/reports/audit.yaml
.agents/templates/reports/decision_brief.yaml
.agents/templates/reports/technical_analysis.yaml
```

Estos templates deben quedar embebidos en el binario vía `EmbeddedAgentFile` (como hoy `templates/report.yaml`), con el mismo patrón de resolución on-disk → embebido de `loadReportTemplate`. Si solo existieran en disco, `report new --kind` fallaría en proyectos donde `quorum init` no colocó templates.

Opcional: extender CLI:

```bash
quorum report new <id> --kind project_usage --output .tmp/<id>.yaml
```

Regla:

- Si `--kind` falta, usar template genérico.
- Si `--kind` existe, cargar template específico.
- Con `--output`, no escribir en `.ai/reports/`; producir scaffold en `.tmp/` para luego persistir vía `report save`.
- Sin `--output`, conservar comportamiento legacy de `report new` hasta deprecación explícita; no ampliar ese camino en `q-report`.

Criterio de éxito:

- `quorum report new onboarding --kind project_usage --output .tmp/onboarding.yaml` crea scaffold válido.

### Fase 4: Migración opcional de legacy

Agregar comando read-only o dry-run:

```bash
quorum report migrate <id> --dry-run
```

Salida propuesta:

- lee `.ai/reports/<id>.yaml`;
- produce YAML semántico equivalente por stdout;
- no escribe salvo `--output` explícito.

Esta fase es opcional. No bloquear la entrega principal.

---

## 9. Reglas para `q-report`

Actualizar la skill con estas reglas:

1. Generar siempre el modelo semántico (`kind`, `presentation`, `content.sections`) para reportes nuevos. Legacy es solo compatibilidad de lectura/render para reportes existentes, no formato de autoría nuevo.
2. Derivar `kind` desde la intención del usuario:
   - “cómo se usa” → `project_usage`;
   - “plan” → `refactor_plan` o `technical_analysis`;
   - “cómo quedó” → `refactor_result`;
   - “auditoría/revisión” → `audit`;
   - “decisión/recomendación” → `decision_brief`.
3. Elegir `presentation.profile` (la skill siempre escribe un valor explícito; no hay default de schema, §5.2):
   - la skill usa `cognitive` salvo que el usuario indique otra cosa;
   - `executive` si el usuario pide algo corto para decidir;
   - `audit` si hay hallazgos/evidencia;
   - `teaching` si el objetivo es aprender/onboardear;
   - `raw` si pide detalle técnico completo.
4. Incluir `verification` cuando haya afirmaciones de IA no verificadas o análisis inferido.
5. Usar `analysis` para explicación causal, no meter prosa larga en `summary`.
6. Usar `appendix` para preservar detalle exhaustivo, logs o comandos largos.
7. No inventar roles fuera del schema.
8. No auto-activar otra skill.
9. Persistir únicamente vía `quorum report save <id>`.

---

## 10. Ejemplo completo mínimo

```yaml
meta:
  id: sample-semantic-report
  schemaVersion: "1.1"
  date: "2026-06-03T15:00:00Z"
kind: technical_analysis
presentation:
  profile: cognitive
  density: medium
  audience: engineer
  language: es
content:
  title: "Por qué q-report se ve menos detallado"
  kicker: "Análisis de modelo de reporte"
  summary: "La diferencia no está en serve, sino en la riqueza del schema y del renderer."
  verdict:
    text: "Agregar secciones semánticas con perfiles de presentación resuelve la brecha sin convertir el schema en una lista infinita de widgets."
    confidence: high
  sections:
    - id: cause
      role: analysis
      title: "Causa: q-report modela widgets, no intención"
      body: |
        El schema actual enumera componentes visuales top-level. Eso hace que cada nueva necesidad narrativa requiera un campo nuevo, aunque semánticamente sea solo otra sección con rol distinto.
    - id: verify-first
      role: verification
      title: "Verificá esto primero"
      items:
        - what: "Compatibilidad legacy"
          why: "Cambiar el schema puede romper reportes existentes."
          check: "go test ./... y abrir un reporte legacy en quorum serve."
    - id: plan
      role: action_plan
      title: "Implementación"
      items:
        - step: 1
          action: "Agregar kind/presentation/content al schema."
          owner: "developer"
        - step: 2
          action: "Agregar renderSemanticReport en app.js."
          owner: "developer"
```

---

## 11. Tests requeridos

### 11.1 Schema

Agregar casos en `cmd/report_test.go` o nuevo test de core:

1. Legacy report actual valida.
2. Semantic report mínimo valida.
3. `content.sections[].id` inválido falla.
4. `content.sections[].role` desconocido falla.
5. `verification` sin `check` falla.
6. `analysis` sin `body` falla.
7. Campo extra dentro de sección falla por `additionalProperties: false`.
8. `presentation.profile` desconocido falla.
9. `kind` desconocido falla.
10. `meta.id` sigue debiendo coincidir con filename en `report save`.
11. Mezclar `content` con cualquier componente legacy top-level falla.
12. `content.sections: []` falla por `minItems: 1`.
13. IDs duplicados en `content.sections[].id` fallan por validación Go con error de campo preciso.
14. Payload semántico sin `content.sections` falla antes de llegar al viewer.
15. `callout.kind` fuera de `decision|warning|note` falla.
16. Reporte semántico autocompleta `schemaVersion: "1.1"`; reporte legacy autocompleta `schemaVersion: "1.0"`.
17. `findings.items[].severity` o `risks.items[].impact` fuera de `low|medium|high|critical` falla.
18. Payload semántico con `schemaVersion` distinto de `"1.1"` literal falla (coherencia modelo↔versión, §8 Fase 1.3).
19. Un reporte semántico con un error real (p. ej. `analysis` sin `body`) produce un error que apunta al campo semántico, NO a `'content' was unexpected` (valida el `if`/`then` raíz, no `oneOf`, §4.2).
20. La unicidad de `content.sections[].id` falla idéntico vía `quorum validate --schema report` y vía `quorum report save` (misma ruta de motor, §5.4).
21. Payload semántico sin `kind` falla.
22. Payload semántico sin `presentation` falla.
23. Payload semántico sin `content.verdict` falla.
24. `diagram.type` dentro de una sección `role: diagram` distinto de `mermaid` falla.
25. `decision_surface.body` con valor no-string falla.
26. `metrics.items[].value` no numérico falla.
27. `evidence.items[].findingId` presente que no exista en `findings.items[].id` falla en validación Go con campo preciso.
28. `evidence.items[]` SIN `findingId` (evidencia autónoma) valida (findingId es opcional, §6.2 invariante 2).
29. `findings.items[].id` duplicados a través del reporte fallan en validación Go con campo preciso (§6.2 invariante 1).
30. `presentation` sin alguno de `profile`/`density`/`audience`/`language` falla (los cuatro son obligatorios, §5.2).
31. Un payload con `kind`/`presentation` pero SIN `content` falla con `'content' is a required property` (no con `'kind' was unexpected`): valida el discriminador raíz por `anyOf`, no por `content` solo (§4.2).
32. `verification` con 0 items falla por `minItems: 1`; con 5 items falla por `maxItems: 4` (§6.1).
33. Una sección con `role` ausente del enum (p. ej. `role: foobar`) falla por el enum de base, no se cuela por el `if`/`then` abierto (§6.2; respalda al test #4).

### 11.2 Viewer

Agregar tests si existe harness JS; si no, cubrir con tests Go de assets o snapshots simples:

1. API `/api/projects/<id>/reports/<report>` devuelve payload semantic validado.
2. `app.js` contiene dispatch `renderSemanticReport` / `renderLegacyReport`.
3. `ROLE_RENDERERS` cubre todos los roles definidos en schema.
4. `PROFILE_ORDER` cubre todos los profiles definidos en schema.

### 11.3 Skill protocol

**Atención — test existente que se rompe.** `internal/core/skill_protocol_test.go` ya contiene `TestReportCatalogDocsInSyncWithSchema`, que itera TODAS las `properties` top-level del schema (excepto `meta`) y exige que cada una esté documentada en el catálogo de `q-report/SKILL.md` (como `` `nombre` ``) Y en el menú de `report.yaml`. Su modelo mental actual es "1 propiedad top-level = 1 componente visual", que es justo lo que esta propuesta abandona. Al agregar `kind`, `presentation`, `content` como propiedades top-level, este test falla hasta que:

- se documenten `kind`, `presentation`, `content` en SKILL.md y en el template; y
- se **adapte el test** para el modelo semántico: la cobertura de catálogo debe iterar los roles de `$defs` (no solo top-level), o separar la verificación en dos: top-level legacy + roles semánticos. Esta adaptación NO es opcional; sin ella la suite queda roja.

Asserts nuevos a agregar (además de adaptar el anterior), verificando que `q-report/SKILL.md`:

1. menciona `content.sections`;
2. menciona `kind`;
3. menciona `presentation.profile`;
4. mantiene persistencia por `quorum report save`;
5. mantiene salida en español;
6. no auto-encadena otra skill;
7. documenta cada role semántico del schema (cobertura role ↔ catálogo).

---

## 12. Riesgos

| Riesgo | Mitigación |
|---|---|
| Schema demasiado complejo | Mantener legacy y agregar pocos roles iniciales. No migrar todo en una sola fase. |
| Duplicación entre legacy y semantic | Definir semantic como preferido, legacy como compatibilidad temporal. |
| Viewer JS crece demasiado | Separar funciones por role dentro del mismo `app.js` inicialmente; extraer módulos solo si el bundle lo permite. |
| Roles mal usados por la IA | `q-report` debe incluir reglas de selección y ejemplos por `kind`. |
| Pérdida de validación estricta | Usar `if`/`then` por role, `additionalProperties: false` en cada sección y validación Go para invariantes no expresables en JSON Schema. |
| Markdown inseguro en viewer | No habilitar HTML crudo. Si se soporta Markdown, renderizar subset seguro o mantener `textContent`. |
| XSS vía `diagram.code` (Mermaid) | `diagram.code` es entrada que Mermaid renderiza; inicializar el viewer con `mermaid.initialize({ securityLevel: 'strict' })` para bloquear HTML/JS embebido en el diagrama. |
| Defaults implícitos divergentes | `kind` y `presentation` son obligatorios en semantic; los defaults solo viven en templates/scaffolds, no en validación ni renderer. |

---

## 13. Decisiones explícitas

1. `serve start` sigue siendo read-only. No genera, no completa, no migra.
2. Git y `.ai/reports/*.yaml` siguen siendo la fuente local del reporte.
3. No se agregan artefactos numerados al lifecycle.
4. Reportes siguen fuera de `00`→`07`.
5. `q-report` sigue siendo single-phase.
6. La primera versión debe preservar reportes legacy, pero la compatibilidad legacy termina en `schemaVersion: "2.0"` salvo ADR que la extienda.
7. El renderer decide presentación a partir de roles, no de nombres top-level infinitos.
8. `appendix` sigue siendo el lugar para detalle exhaustivo.
9. `verification` sigue siendo obligatorio cuando el reporte depende de inferencias no verificadas.
10. Reportes nuevos generados por `q-report` usan siempre el modelo semántico; legacy no es formato nuevo de autoría.
11. Fase 1 acepta solo `meta.schemaVersion: "1.1"` para semántico y solo Mermaid para diagramas.

---

## 14. Criterio de aceptación de la propuesta

La solución está implementada correctamente cuando:

1. Se puede guardar un reporte semántico con `quorum report save`.
2. Se puede abrir en `quorum serve start`.
3. El mismo viewer sigue abriendo reportes legacy.
4. `q-report` genera siempre reportes nuevos con `kind`, `presentation`, `content.verdict` y `content.sections`.
5. Un reporte tipo `project_usage` puede contener análisis narrativo, verificación, diagrama, plan y apéndice sin inventar campos fuera del schema.
6. Agregar un nuevo tipo futuro de reporte requiere preferentemente un nuevo `kind` o template, no necesariamente un nuevo componente top-level.
7. Agregar una nueva representación visual requiere preferentemente un nuevo `profile` o renderer por `role`, sin cambiar la semántica del contenido.

---

## 15. Correcciones anti-ambigüedad incorporadas

Esta sección fija explícitamente los puntos que deben quedar resueltos antes de convertir la idea en `00-spec.yaml`/`01-blueprint.yaml`/`02-contract.yaml`.

| ID | Decisión incorporada | Sección fuente |
|---|---|---|
| A | Legacy y semantic son modelos mutuamente exclusivos; compat legacy termina en `schemaVersion: "2.0"`; los renderers legacy deben ser adaptadores, no duplicación completa de app.js. | §4.2, §8 Fase 2, §13 |
| B | La unicidad de `content.sections[].id` se valida en Go porque JSON Schema no puede garantizar unicidad de subcampo en arrays de objetos. | §5.4, §8 Fase 1, §11.1 |
| C | No usar `oneOf` por role; usar `if`/`then` para preservar errores precisos de `schema.go`. | §6.2 |
| D | El viewer detecta modelo semántico por `Boolean(data.content)`, no por `content.sections`. El schema/API rechaza contenido semántico incompleto antes de render. | §8 Fase 2 |
| E | `content.verdict` y `content.summary` son encabezado fijo; `orderSections` solo reordena `content.sections[]`. | §5.3, §7, §8 Fase 2 |
| F | `schemaVersion` autocompleta `1.1` para semántico y `1.0` para legacy; no decide renderer, pero refleja formato persistido. | §8 Fase 1, §11.1 |
| G | `callout` usa `kind: decision|warning|note`, no `severity`. | §6.1, §11.1 |
| H | `report new` sin `--output` ya escribe en `.ai/reports/`; la propuesta conserva ese path por compatibilidad y restringe `q-report` a `.tmp/` + `report save`. | §8 Fase 3 |
| I | Mezclar legacy + semantic falla validación; `content.sections: []` falla por `minItems: 1`. | §4.2, §8 Fase 1, §11.1 |
| J | `TestReportCatalogDocsInSyncWithSchema` (existente) se rompe al agregar top-level: hay que documentar `kind`/`presentation`/`content` y adaptar el test para iterar roles de `$defs`, no solo top-level. No opcional. | §8 Fase 1, §11.3 |
| K | Discriminación raíz legacy↔semantic con `if (content) then ... else ...`, NUNCA `oneOf`, para preservar mensajes de error precisos (mismo motivo que C, a nivel modelo). | §4.2, §11.1 |
| L | `findings.items[].severity` y `risks.items[].impact` usan enum cerrado `low\|medium\|high\|critical`; no `string` libre (pills de color deterministas). | §5.4, §6.1, §11.1 |
| M | §5.4 es ILUSTRATIVA; la forma autoritativa de cada sección es el `$def` por role de §6.2. `severity` no es campo genérico de toda sección. | §5.4, §6.2 |
| N | La validación Go de unicidad de `id` vive en el motor (`schema.go`), no en el comando, para que `validate --schema report` y `report save` fallen idéntico. | §5.4, §8 Fase 1 |
| O | Coherencia modelo↔versión: en Fase 1 un payload semántico debe usar exactamente `schemaVersion: "1.1"`; el autofill no alcanza porque solo rellena campos vacíos. | §8 Fase 1, §11.1 |
| P | `orderSections` determinista: roles no enumerados van al final en orden de autoría; secciones del mismo role mantienen orden relativo (sort estable). | §7 |
| Q | Mermaid se inicializa con `securityLevel: 'strict'`; `diagram.code` es entrada no confiable. | §12 |
| R | Los templates por `kind` se embeben vía `EmbeddedAgentFile`, no solo en disco, para que `report new --kind` funcione sin `quorum init` previo. | §8 Fase 3 |
| S | `kind` y `presentation` son obligatorios en semantic; sus defaults solo existen en scaffolds/templates, no en schema ni renderer. | §5.1, §5.2, §8 Fase 1 |
| T | `content.verdict` es obligatorio en semantic junto con `content.title` y `content.sections`. | §5.3, §11.1 |
| U | `findings.items[].id` y `evidence.items[].findingId` habilitan enlace visual. Dos invariantes Go: (1) los `findings.items[].id` son únicos en todo el reporte; (2) `findingId` es opcional, pero si está presente debe existir. La evidencia autónoma (sin `findingId`) es válida. | §6.1, §6.2, §11.1 |
| V | Fase 1 solo admite `diagram.type: mermaid`; no hay renderers diagramáticos abiertos. | §6.1, §11.1, §13 |
| W | `q-report` debe generar siempre semantic para reportes nuevos; legacy queda solo para compatibilidad de lectura/render. | §9, §13 |
| X | Los cuatro subcampos de `presentation` (`profile`/`density`/`audience`/`language`) son obligatorios; el renderer depende de valores explícitos y los defaults solo viven en el scaffold. | §5.2, §8 Fase 1 |
| Y | El seed `report.yaml` pasa a ser semántico válido por construcción (todos los required + `schemaVersion: "1.1"`); el menú legacy queda como comentario que nombra los componentes para los tests de catálogo. | §8 Fase 1.5, §11.3 |
| Z | El discriminador raíz se dispara por `anyOf: [content, kind, presentation]`, no solo por `content`, para que un semántico incompleto reporte el `required` faltante real en vez de `'kind' was unexpected`. | §4.2, §11.1 |
| AA | La base de sección declara `role` como `enum` cerrado; el `if`/`then` es abierto y NO rechaza roles desconocidos por sí solo. Necesario para el test #4. | §6.2, §11.1 |
| AB | `verification.items` tiene `minItems: 1, maxItems: 4` (paridad con el legacy `verify`). | §6.1, §11.1 |
| AC | Dos sentidos de "legacy" desambiguados: `legacy report model` (datos) vs `legacy report-new direct-write path` (CLI). | §4.2, §8 Fase 3, §13 |
| AD | Hook Go post-schema ubicado exactamente en `ValidateAgainstSchema` (`schema.go:30`), tras `schema.Validate` y antes de `return nil`, gateado por nombre de schema; preferir registry sobre `if` hardcodeado. | §5.4, §6.2, §8 Fase 1.2 |

Estas decisiones deben trasladarse al contrato de implementación como invariantes verificables.

## 16. Próximo paso recomendado

Crear una tarea Quorum específica para implementar **Fase 1 + Fase 2** como primer incremento. No incluir Fase 3/Fase 4 en la misma tarea salvo que el contrato sea pequeño y los tests queden claros.

Título sugerido de tarea:

```text
REPORT-SEM-001: Add semantic report sections and presentation profiles
```

Alcance sugerido:

- schema v1.1;
- template genérico semántico;
- viewer con dispatch semantic/legacy;
- actualización de `q-report`;
- tests de validación y compatibilidad.
