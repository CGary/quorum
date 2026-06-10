# 🧪 Propuesta Técnica: Anti-Vacuidad por Evidencia TDD

**Estado:** Propuesta — eslabón 3 de 3, cierre de la cadena ([1] estructura → [2] trazabilidad → este).
**Contexto:** Evolución de Quorum v1.2+. **Depende de [1] (acceptance.id) y [2] (test_scenarios.covers).**
**Mecanismo elegido:** Evidencia TDD nativa. Descartados explícitamente el motor de mutación de código y la mutación de spec estilo Gherkin (ver §3 y §6).
**Origen:** Destilado de la prueba de mutación de R.C. Martin (*"¿qué prueba que tus pruebas prueban?"*). Se conserva el OBJETIVO (un test no-vacuo); se adapta el MECANISMO, porque Quorum **no genera tests desde el spec** y por tanto la mutación de spec de Martin no mapea a su arquitectura.

---

## 1. El Problema Real (acotado)

Las Reglas #4 ("validation is finality") y #8 ("tests are the only proof") garantizan que `verify.commands` retorna 0. Pero **exit 0 no prueba que el test sea no-vacuo.** Un test así pasa y, con el doc [2], hasta "cubre" un criterio:

```python
def test_payment_selectable():  # cubre AC-1 según blueprint.covers
    assert True                  # nunca ejerce el código real
```

- El doc [2] prueba que **existe un test enlazado** a cada `acceptance.id`.
- El doc [5] prueba que **ese test efectivamente ejerce el criterio** — que alguna vez estuvo en ROJO sin la implementación.

**Realidad técnica (sin maquillar):** ninguna superficie actual captura esto.

| Artefacto | Qué captura | Lo que NO captura |
|---|---|---|
| `07-trace.json.attempts[]` (líneas 56-112) | `phase` (verify/review/...) + `result` (passed/failed) a nivel de **fase**, append-only | nada por-test ni por-`acceptance.id`; ROJO→VERDE no es deducible a nivel de criterio |
| `05-validation.json.commands[]` (líneas 28-57) | `command`/`exit_code`/`duration`/`output` de **una** corrida (la verde) | la corrida ROJA previa; vínculo al `acceptance.id` |
| `04-implementation-log.yaml.entries[]` | `changed_files`/`notes`/`verify_pending` | `acceptance.id`; evidencia ROJO→VERDE |

**Conclusión:** la opción nativa es la más barata de las tres evaluadas, pero exige una superficie de captura mínima + un toque de workflow en `q-implement` (rojo) y `q-verify` (verde + consolidación). No es gratis; es *barata y constitucional*.

---

## 2. Componentes a Implementar

### 2.1 Captura de evidencia TDD por `acceptance.id`

**Qué hace:** registrar que el test que cubre cada criterio fue observado **fallando antes** de la implementación que lo hace pasar.

**Decisión de diseño (constitucional):** se **extienden dos artefactos existentes** (`04-implementation-log.yaml` para el ROJO, `05-validation.json` para la evidencia consolidada), NO se crea un artefacto numerado nuevo — el manifiesto rechaza nuevos slots sin ADR, pero extender el schema de un slot existente es legítimo. La división respeta la matriz de productores: `04` lo produce `q-implement` y `05` lo produce `q-verify`; ninguno escribe el artefacto del otro.

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
| Home final | Campo opcional `tdd_evidence[]` en `validation.schema.json`. Sin artefacto nuevo. |
| Captura del ROJO | **`q-implement`**, ANTES de escribir la implementación: corre el test cubridor y registra `{acceptance_id, command, red_exit_code}` en un campo opcional `tdd_red_runs[]` de `04-implementation-log.yaml`. `q-verify` NO puede observar el rojo: corre después de implementar, y reconstruir el estado pre-cambio (stash/checkout en el worktree) queda rechazado en §3. |
| Captura del VERDE | `q-verify` re-ejecuta cada comando registrado en `04.tdd_red_runs[]`, obtiene `green_exit_code` y consolida `tdd_evidence[]` en `05-validation.json`. |
| Reescritura de `05` | `05-validation.json` se sobreescribe en cada corrida de `q-verify` — solo `07-trace` tiene enforcement append-only (`EnsureTraceAppendOnly`). NO se promete inmutabilidad de `tdd_evidence`: es **reconstruible** en cada corrida, porque el rojo persiste en `04` y el verde se recalcula. Sin invariante nuevo en `SaveArtifact`. |
| Ventana única del ROJO | El rojo solo es observable mientras la implementación no existe. Si `q-implement` no lo capturó, es irrecuperable sin revertir código: `q-accept` lo reportará como `sin evidencia` de forma permanente y honesta. No hay re-captura retroactiva. |

### 2.2 Gate advisory en `q-accept`

**Qué hace:** nuevo ítem de checklist — para cada `acceptance.id` con cobertura (doc [2]), reportar si **falta** evidencia TDD.

**Decisión de diseño (constitucional):** **advisory, no bloqueo duro.**
- Convertirlo en gate duro de `verify.commands` chocaría con la Regla #4 ("done means verify.commands returned 0; no diagnostic agent can waive this") — añadiría una condición de finalidad nueva. Prohibido sin ADR de peso.
- El home correcto es `q-accept` (compuerta **humana**, Regla #6), exactamente como ya se reporta `acceptance.bdd_suite` (`q-accept/SKILL.md:40`). El humano decide si la falta de evidencia bloquea su merge.

Nuevo ítem en el Checklist de `q-accept/SKILL.md`:
> 9. Para cada `acceptance.id` cubierto, reportar si `05-validation.json.tdd_evidence` registra ROJO→VERDE. Advisory: el humano juzga.

### 2.3 (Opcional) `quorum analyze tdd-evidence`

Helper read-only que cruza `acceptance[]` ↔ `covers[]` ↔ `tdd_evidence[]` y reporta criterios sin evidencia. Mismo molde que `acceptance-coverage` (doc [2]) y `decomposition-coverage`: `internal/core/tdd_evidence.go` + `cmd/analyze_tdd_evidence.go`, puro y sin efectos.

---

## 3. Lo que NO se Implementará (límites explícitos)

| Componente rechazado | Razón |
|---|---|
| Motor de mutación de código (gremlins/go-mutesting) | Acopla a un runner por-lenguaje y agrega minutos al loop. Descartado en la decisión de mecanismo (se eligió evidencia TDD nativa). Reconsiderable en ADR futuro si la evidencia TDD resulta insuficiente. |
| Mutación de spec estilo R.C. Martin | Requiere generación automática de tests desde el criterio — Quorum no la tiene y chocaría con Reglas #5 y #1. Probable v2.0, no aquí. |
| Gate duro en `verify.commands` | Violaría la Regla #4 (añadir finalidad). La anti-vacuidad vive como advisory en la compuerta humana. |
| Nuevo artefacto numerado (`08`...) | Prohibido sin ADR. Se extienden `04-implementation-log.yaml` y `05-validation.json`. |
| Obligar evidencia TDD para criterios `string` legacy (sin `id`) | Imposible sin doc [1]. Solo aplica a criterios con `acceptance.id`. |
| Reconstruir el estado pre-cambio en `q-verify` (stash/checkout del worktree) | Operación destructiva y frágil dentro del worktree quirúrgico, y duplica lo que `q-implement` puede observar gratis en el momento correcto. El rojo se captura donde ocurre. |
| Invariante append-only nuevo para `tdd_evidence` en `SaveArtifact` | Innecesario: la evidencia es reconstruible desde `04.tdd_red_runs[]` + re-ejecución del verde. Prometer inmutabilidad sin punto de enforcement sería deuda, no diseño. |

---

## 4. Decisión Cerrada: granularidad per-AC (Opción B)

La granularidad de la evidencia **está decidida: per-`acceptance.id` (Opción B)**. R.C. Martin no la resuelve (su evidencia es spec-generada), así que se eligió explícitamente contra la alternativa de nivel-tarea.

| Opción | Qué implica | Veredicto |
|---|---|---|
| A. Nivel-tarea (inferida) | Deducir ROJO→VERDE del historial de `07-trace.attempts[]` (un `verify` con `failed` seguido de uno `passed`). Cero captura nueva. | **Rechazada.** Coarse: prueba que *algo* estuvo rojo, no que el test de un criterio puntual lo estuvo. Un test vacuo de `AC-2` queda escondido detrás del rojo legítimo de `AC-1`. Anula el propósito del eslabón. |
| **B. Per-AC (capturada)** | Rojo en `04.tdd_red_runs[]` por `acceptance.id` (capturado por `q-implement` pre-implementación); verde y consolidación en `05.tdd_evidence[]` (por `q-verify`). Ver §2.1. | **ELEGIDA.** Única opción que detecta no-vacuidad por criterio — el valor real del doc. Costo: toca `q-implement` y `q-verify`. |

**Implicación de workflow (parte del alcance de la tarea):** `q-implement` debe, por cada `acceptance.id` cubierto, ejecutar su test cubridor **antes** de la implementación y registrar `red_exit_code` en `04.tdd_red_runs[]`; `q-verify` registra después el `green_exit_code` y consolida en `05`. Sin esa corrida-en-rojo no hay evidencia B, y no es recuperable a posteriori (ventana única, §2.1).

**Límites honestos asumidos:**

1. Un `red_exit_code ≠ 0` puede originarse en un fallo trivial (import roto), no en una aserción genuina. B prueba "el test dependía de algo ausente", no "el test es perfecto". Aun así es estrictamente más fuerte que A. No se añade detección de causa del rojo en este eslabón (posible refinamiento futuro, sin ADR).
2. **La evidencia es auto-reportada** por el mismo agente que implementa y verifica; nada impide fabricar exit codes. Mitigación: el gate es advisory ante humano (Regla #6) y el comando registrado es re-ejecutable para auditar el verde; el rojo es irrepetible por naturaleza. Riesgo aceptado, no resuelto.
3. **Interacción con `error_category` (FAIL-001):** si la corrida registra `error_category == flaky` (`validation.schema.json:66`), `q-accept` marca la evidencia TDD de ese intento como no confiable — un rojo o un verde flaky no prueba transición alguna.

---

## 5. Dependencias y Orden

```text
[1] acceptance.id  →  [2] covers + cobertura  →  [5] ESTE: evidencia de no-vacuidad
     (identidad)         (qué test cubre qué)        (ese test, ¿prueba de verdad?)
```

- **Bloqueado por [1] y [2]:** sin `id` no hay a qué atar evidencia; sin `covers` no se sabe qué test corresponde a qué criterio.
- **Cierre de la cadena:** completa el salto de "tests pasan" (hoy) a "tests pasan, cubren el spec, y no son vacuos".

---

## 6. Ingesta al Flujo SDD

Ingesta para `/q-brief`. Tarea Quorum sugerida `FEAT-TDD-1` (implementar **después** de [1] y [2]):

- **goal:** capturar evidencia de no-vacuidad (ROJO→VERDE) por criterio de aceptación — rojo en `04` vía `q-implement`, consolidación en `05` vía `q-verify` — y reportarla como advisory en la compuerta humana, sin motor de mutación ni artefacto nuevo.
- **invariants:** `05-validation.json` sin `tdd_evidence` y `04-implementation-log.yaml` sin `tdd_red_runs` siguen válidos; `verify.commands` sigue siendo el único determinante de finalidad (Regla #4 intacta); `q-accept` no bloquea ni mergea (Regla #6); la matriz de productores no cambia (`04`→`q-implement`, `05`→`q-verify`).
- **acceptance:**
  - AC-1 — `implementation-log.schema.json` acepta `tdd_red_runs[]` opcional con `acceptance_id`+`command`+`red_exit_code`.
  - AC-2 — `validation.schema.json` acepta `tdd_evidence[]` opcional con `acceptance_id`+`command`+`red_exit_code`+`green_exit_code`.
  - AC-3 — `q-verify` consolida `04.tdd_red_runs[]` + verde recalculado en `05.tdd_evidence[]`.
  - AC-4 — `q-accept` reporta como advisory todo `acceptance.id` cubierto sin evidencia TDD, y marca como no confiable la evidencia de intentos con `error_category == flaky`.
  - AC-5 — la ausencia de `tdd_evidence` NO cambia `overall_result` ni bloquea `verify.commands`.

---

## 7. Trazabilidad de la Decisión

- **Objetivo conservado (R.C. Martin):** que el test pruebe realmente el criterio, no que pase por casualidad.
- **Mecanismo adaptado:** evidencia ROJO→VERDE en vez de mutación, porque Quorum no genera tests del spec; la mutación de spec de Martin presupone esa generación.
- **Honestidad técnica:** no hay campo nativo de evidencia per-AC hoy; este eslabón lo agrega extendiendo `04-implementation-log` (rojo) y `05-validation` (consolidado), no creando un slot. La matriz de productores se respeta: cada skill escribe solo su artefacto.
- **Constitucionalidad:** advisory en compuerta humana (Regla #6), nunca finalidad nueva (Regla #4), nunca artefacto nuevo (sin ADR).
- **Decisión cerrada:** granularidad per-AC (Opción B); nivel-tarea (A) rechazada por esconder tests vacuos detrás de un rojo legítimo ajeno. Ver §4.
