# 🧪 Propuesta Técnica: Anti-Vacuidad por Evidencia TDD

**Estado:** Propuesta — eslabón 3 de 3, cierre de la cadena ([11] estructura → [12] trazabilidad → este).
**Contexto:** Evolución de Quorum v1.2+. **Depende de [11] (acceptance.id) y [12] (test_scenarios.covers).**
**Mecanismo elegido:** Evidencia TDD nativa. Descartados explícitamente el motor de mutación de código y la mutación de spec estilo Gherkin (ver §3 y §6).
**Origen:** Destilado de la prueba de mutación de R.C. Martin (*"¿qué prueba que tus pruebas prueban?"*). Se conserva el OBJETIVO (un test no-vacuo); se adapta el MECANISMO, porque Quorum **no genera tests desde el spec** y por tanto la mutación de spec de Martin no mapea a su arquitectura.

---

## 1. El Problema Real (acotado)

Las Reglas #4 ("validation is finality") y #8 ("tests are the only proof") garantizan que `verify.commands` retorna 0. Pero **exit 0 no prueba que el test sea no-vacuo.** Un test así pasa y, con el doc [12], hasta "cubre" un criterio:

```python
def test_payment_selectable():  # cubre AC-1 según blueprint.covers
    assert True                  # nunca ejerce el código real
```

- El doc [12] prueba que **existe un test enlazado** a cada `acceptance.id`.
- El doc [13] prueba que **ese test efectivamente ejerce el criterio** — que alguna vez estuvo en ROJO sin la implementación.

**Realidad técnica (sin maquillar):** ninguna superficie actual captura esto.

| Artefacto | Qué captura | Lo que NO captura |
|---|---|---|
| `07-trace.json.attempts[]` (líneas 56-112) | `phase` (verify/review/...) + `result` (passed/failed) a nivel de **fase**, append-only | nada por-test ni por-`acceptance.id`; ROJO→VERDE no es deducible a nivel de criterio |
| `05-validation.json.commands[]` (líneas 28-57) | `command`/`exit_code`/`duration`/`output` de **una** corrida (la verde) | la corrida ROJA previa; vínculo al `acceptance.id` |
| `04-implementation-log.yaml.entries[]` | `changed_files`/`notes`/`verify_pending` | `acceptance.id`; evidencia ROJO→VERDE |

**Conclusión:** la opción nativa es la más barata de las tres evaluadas, pero exige una superficie de captura mínima + un toque de workflow en `q-verify`. No es gratis; es *barata y constitucional*.

---

## 2. Componentes a Implementar

### 2.1 Captura de evidencia TDD por `acceptance.id`

**Qué hace:** registrar que el test que cubre cada criterio fue observado **fallando antes** de la implementación que lo hace pasar.

**Decisión de diseño (constitucional):** se **extiende un artefacto existente** (`05-validation.json`), NO se crea un artefacto numerado nuevo — el manifiesto rechaza nuevos slots sin ADR, pero extender el schema de un slot existente es legítimo.

```json
// 05-validation.json (extensión opcional)
"tdd_evidence": [
  {
    "acceptance_id": "AC-1",
    "command": "pytest tests/test_payment.py::test_selectable",
    "red_exit_code": 1,    // observado ANTES de implementar
    "green_exit_code": 0   // observado DESPUÉS
  }
]
```

| Aspecto | Decisión propuesta |
|---|---|
| Home | Campo opcional `tdd_evidence[]` en `validation.schema.json`. Sin artefacto nuevo. |
| Captura | `q-verify` (o `q-implement` en disciplina TDD) corre el test que cubre el `acceptance.id` **antes** del cambio, registra `red_exit_code ≠ 0`, luego `green_exit_code == 0`. |
| Append-only | Coherente con la naturaleza append de la evidencia: una vez registrada la transición ROJO→VERDE de un AC, no se reescribe. |

### 2.2 Gate advisory en `q-accept`

**Qué hace:** nuevo ítem de checklist — para cada `acceptance.id` con cobertura (doc [12]), reportar si **falta** evidencia TDD.

**Decisión de diseño (constitucional):** **advisory, no bloqueo duro.**
- Convertirlo en gate duro de `verify.commands` chocaría con la Regla #4 ("done means verify.commands returned 0; no diagnostic agent can waive this") — añadiría una condición de finalidad nueva. Prohibido sin ADR de peso.
- El home correcto es `q-accept` (compuerta **humana**, Regla #6), exactamente como ya se reporta `acceptance.bdd_suite` (`q-accept/SKILL.md:40`). El humano decide si la falta de evidencia bloquea su merge.

Nuevo ítem en el Checklist de `q-accept/SKILL.md`:
> 9. Para cada `acceptance.id` cubierto, reportar si `05-validation.json.tdd_evidence` registra ROJO→VERDE. Advisory: el humano juzga.

### 2.3 (Opcional) `quorum analyze tdd-evidence`

Helper read-only que cruza `acceptance[]` ↔ `covers[]` ↔ `tdd_evidence[]` y reporta criterios sin evidencia. Mismo molde que `acceptance-coverage` (doc [12]) y `decomposition-coverage`: `internal/core/tdd_evidence.go` + `cmd/analyze_tdd_evidence.go`, puro y sin efectos.

---

## 3. Lo que NO se Implementará (límites explícitos)

| Componente rechazado | Razón |
|---|---|
| Motor de mutación de código (gremlins/go-mutesting) | Acopla a un runner por-lenguaje y agrega minutos al loop. Descartado en la decisión de mecanismo (se eligió evidencia TDD nativa). Reconsiderable en ADR futuro si la evidencia TDD resulta insuficiente. |
| Mutación de spec estilo R.C. Martin | Requiere generación automática de tests desde el criterio — Quorum no la tiene y chocaría con Reglas #5 y #1. Probable v2.0, no aquí. |
| Gate duro en `verify.commands` | Violaría la Regla #4 (añadir finalidad). La anti-vacuidad vive como advisory en la compuerta humana. |
| Nuevo artefacto numerado (`08`...) | Prohibido sin ADR. Se extiende `05-validation.json`. |
| Obligar evidencia TDD para criterios `string` legacy (sin `id`) | Imposible sin doc [11]. Solo aplica a criterios con `acceptance.id`. |

---

## 4. Decisión Cerrada: granularidad per-AC (Opción B)

La granularidad de la evidencia **está decidida: per-`acceptance.id` (Opción B)**. R.C. Martin no la resuelve (su evidencia es spec-generada), así que se eligió explícitamente contra la alternativa de nivel-tarea.

| Opción | Qué implica | Veredicto |
|---|---|---|
| A. Nivel-tarea (inferida) | Deducir ROJO→VERDE del historial de `07-trace.attempts[]` (un `verify` con `failed` seguido de uno `passed`). Cero captura nueva. | **Rechazada.** Coarse: prueba que *algo* estuvo rojo, no que el test de un criterio puntual lo estuvo. Un test vacuo de `AC-2` queda escondido detrás del rojo legítimo de `AC-1`. Anula el propósito del eslabón. |
| **B. Per-AC (capturada)** | `tdd_evidence[]` en `05-validation` por `acceptance.id` (§2.1). `q-verify` corre el test cubridor de cada criterio contra el código pre-cambio y registra `red_exit_code`. | **ELEGIDA.** Única opción que detecta no-vacuidad por criterio — el valor real del doc. Costo: toca `q-verify`. |

**Implicación de workflow (parte del alcance de la tarea):** `q-verify` (o `q-implement` en disciplina TDD) debe, por cada `acceptance.id` cubierto, ejecutar su test cubridor **antes** de la implementación y registrar `red_exit_code`, luego el `green_exit_code` post-implementación. Sin esta corrida-en-rojo no hay evidencia B.

**Límite honesto asumido:** un `red_exit_code ≠ 0` puede originarse en un fallo trivial (import roto), no en una aserción genuina. B prueba "el test dependía de algo ausente", no "el test es perfecto". Aun así es estrictamente más fuerte que A. No se añade detección de causa del rojo en este eslabón (posible refinamiento futuro, sin ADR).

---

## 5. Dependencias y Orden

```text
[11] acceptance.id  →  [12] covers + cobertura  →  [13] ESTE: evidencia de no-vacuidad
     (identidad)         (qué test cubre qué)        (ese test, ¿prueba de verdad?)
```

- **Bloqueado por [11] y [12]:** sin `id` no hay a qué atar evidencia; sin `covers` no se sabe qué test corresponde a qué criterio.
- **Cierre de la cadena:** completa el salto de "tests pasan" (hoy) a "tests pasan, cubren el spec, y no son vacuos".

---

## 6. Ingesta al Flujo SDD

Ingesta para `/q-brief`. Tarea Quorum sugerida `FEAT-TDD-1` (implementar **después** de [11] y [12]):

- **goal:** capturar evidencia de no-vacuidad (ROJO→VERDE) por criterio de aceptación y reportarla como advisory en la compuerta humana, sin motor de mutación ni artefacto nuevo.
- **invariants:** `05-validation.json` sin `tdd_evidence` sigue válido; `verify.commands` sigue siendo el único determinante de finalidad (Regla #4 intacta); `q-accept` no bloquea ni mergea (Regla #6).
- **acceptance:**
  - AC-1 — `validation.schema.json` acepta `tdd_evidence[]` opcional con `acceptance_id`+`red_exit_code`+`green_exit_code`.
  - AC-2 — `q-accept` reporta como advisory todo `acceptance.id` cubierto sin evidencia TDD.
  - AC-3 — la ausencia de `tdd_evidence` NO cambia `overall_result` ni bloquea `verify.commands`.

---

## 7. Trazabilidad de la Decisión

- **Objetivo conservado (R.C. Martin):** que el test pruebe realmente el criterio, no que pase por casualidad.
- **Mecanismo adaptado:** evidencia ROJO→VERDE en vez de mutación, porque Quorum no genera tests del spec; la mutación de spec de Martin presupone esa generación.
- **Honestidad técnica:** no hay campo nativo de evidencia per-AC hoy; este eslabón lo agrega extendiendo `05-validation`, no creando un slot.
- **Constitucionalidad:** advisory en compuerta humana (Regla #6), nunca finalidad nueva (Regla #4), nunca artefacto nuevo (sin ADR).
- **Decisión cerrada:** granularidad per-AC (Opción B); nivel-tarea (A) rechazada por esconder tests vacuos detrás de un rojo legítimo ajeno. Ver §4.
