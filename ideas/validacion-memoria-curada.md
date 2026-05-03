# 🧠 Propuesta Técnica: Validación de Memoria Curada (Similarity & Drift Detection)

**Estado:** Diferida — implementar cuando se cumplan las precondiciones (ver §4).
**Contexto:** Evolución de Quorum v1.2+.
**Origen:** Subconjunto refinado de la propuesta original "Gobernanza de la Memoria Semántica", filtrado por análisis de factibilidad contra el código real.
**Veredicto:** Solo dos componentes de la propuesta original tienen valor incremental neto sobre lo que `q-memory` ya implementa. Todo lo demás se rechaza por redundancia o sobre-ingeniería (ver §3).

---

## 1. El Problema Real (acotado)

Quorum ya implementa el patrón "biblioteca curada" mediante `q-memory` + `memory/{patterns,decisions,lessons}/`. La ingesta NO es automática: requiere invocación explícita post-acceptance. Esto **previene** los problemas clásicos de Model Collapse e ingesta de ruido.

Sin embargo, dos modos de fallo permanecen sin mitigar:

1. **Fragmentación de conocimiento**: el curador (humano o agente) puede capturar una memoria sustancialmente similar a una existente sin saberlo. El campo `supersedes` del schema queda inutilizado.
2. **Deriva silenciosa entre plan e implementación**: una memoria capturada puede contradecir el `01-blueprint.yaml` original (señal de scope creep, re-blueprint no documentado, o aprendizaje genuino que merece revisión arquitectónica). Hoy nadie detecta esto.

---

## 2. Componentes a Implementar (a futuro)

### 2.1 Detección de Similitud Antes de Escribir

**Qué hace:** antes de que `q-memory` cree una nueva entrada, compara su contenido contra las memorias existentes del mismo tipo. Si la similitud supera un umbral, presenta al curador tres opciones:

- **Superseder** la memoria similar (usa el campo `supersedes` ya existente).
- **Crear como variante** (usa `related` para vincular ambas).
- **Confirmar como nueva** (justificación obligatoria en `content`).

**Diseño técnico:**

| Aspecto | Decisión propuesta |
|---|---|
| Algoritmo | Empezar con lexical match (Jaccard sobre `title` + n-gramas de `content`). Embeddings solo si la versión lexical genera demasiados falsos negativos tras 20+ memorias acumuladas. |
| Umbral inicial | 70% de similitud lexical. Ajustable en `policies/memory.yaml` (archivo nuevo). |
| Costo | Cero si es lexical; bajo si es embeddings (modelo local o L0). |
| Almacenamiento | Sin índice persistente en v1: scan completo de `memory/{type}/`. Indexar solo si las memorias superan ~100 entradas. |

**Cambios requeridos:**
- Nueva sección en `q-memory/SKILL.md` con el protocolo de detección.
- Nuevo archivo `policies/memory.yaml` con umbrales.
- Función auxiliar en `.agents/cli/core/` para el match lexical.

### 2.2 Validador de Deriva Blueprint↔Memoria

**Qué hace:** cuando `q-memory` está por capturar una memoria, compara los archivos/símbolos/decisiones referenciados en ella contra `01-blueprint.yaml` y `02-contract.yaml` originales. Si detecta divergencia significativa, marca la memoria como `requires_review: true` para inspección humana.

**Señales de deriva a detectar:**

| Señal | Interpretación |
|---|---|
| Memoria menciona archivos no listados en `blueprint.affected_files` | Posible scope creep o re-blueprint no documentado. |
| Memoria describe decisión que contradice `00-spec.yaml.invariants` | Violación de invariante o invariante mal especificado. |
| Memoria captura "lesson" sobre comportamiento que NO está en `acceptance_criteria` | Aprendizaje fuera de alcance — útil pero requiere ADR. |

**Cambios requeridos:**
- Nuevo campo opcional `requires_review: boolean` en `memory.schema.json`.
- Función de comparación estructurada en `q-memory`.
- Alerta en CLI cuando se detecta deriva.

---

## 3. Lo que NO se Implementará (rechazado del original)

Estos componentes de la propuesta original quedan formalmente descartados. No reabrir sin nueva evidencia:

| Componente rechazado | Razón de rechazo |
|---|---|
| Estados `gold_standard` / `operational_log` / `discarded` | Redundante con la tipología existente (`pattern` ≈ gold; `lesson` ≈ operacional). Añade dimensión ortogonal sin justificación. |
| Artefacto dedicado `09/10-impact-report.json` | Innecesario: `q-memory` escribe directamente en `memory/*`. Añadir un artefacto intermedio duplica el flujo. |
| Integración con HSME (RRF, time-decay, factor de confianza) | Out of scope. Quorum es local-first y machine-first sobre disco. HSME es un sistema externo del usuario, no del framework. Romper esa frontera viola Regla #1 ("Git es la verdad"). |
| Campo `confidence_score: 0.95` | Score arbitrario sin fuente verificable. Anti-patrón conocido en sistemas LLM (falsa precisión). |
| CLI dedicada `agents task consolidate <ID> --as-gold` | El binomio `q-accept` + `q-memory` ya cubre el flujo. Añadir un comando duplica responsabilidades. |
| Veto humano explícito durante el Merge Gate | Ya implícito: `q-memory` es invocación humana, no automatismo. No hay automatismo que vetar. |
| Reformular `q-memory` como "Curation Loop" | El skill ya implementa curación selectiva. La propuesta original asumía un problema que Quorum no tiene (ingesta automática estilo Spec Kitty). |

---

## 4. Precondiciones para Desbloquear

**No implementar §2 hasta que TODAS estas condiciones se cumplan:**

| # | Precondición | Justificación |
|---|---|---|
| 1 | `memory/` contiene **≥10 entradas reales** (no de prueba). | Sin masa crítica, la detección de similitud no tiene contra qué comparar. |
| 2 | Plan `MEM-001` (anti-patrones + supersesión) está implementado y en uso. | Es el cimiento estructural de §2.1. |
| 3 | Al menos **2 casos documentados** de duplicación o contradicción detectados manualmente. | Evidencia de que el problema existe en este proyecto, no solo en teoría. |
| 4 | `q-memory` se ha invocado en ≥5 tareas distintas. | Confirma que el flujo de captura está adoptado antes de invertir en validación. |

Si alguna condición no se cumple en 3 meses, **revisar si el problema es real**. Es posible que la disciplina manual de `q-memory` sea suficiente y la automatización sea sobre-ingeniería.

---

## 5. Trazabilidad de la Decisión

- **Propuesta original:** ingesta automática + curación + HSME + estados de confianza + 7 capacidades nuevas.
- **Análisis de factibilidad:** 70% de la propuesta describía comportamiento que `q-memory` ya implementa. 20% era out-of-scope (HSME). Solo 10% (similitud + drift) tenía valor incremental neto.
- **Acción inmediata extraída:** plan `MEM-001` (anti-patrones + supersesión) — implementación trivial, alto valor.
- **Acción diferida:** este documento (validación de memoria curada) — esperar datos antes de invertir.
- **Acción rechazada:** §3 — no reabrir sin nueva evidencia.

---

## 6. Próximos Pasos para el Agente Revisor (cuando se desbloquee)

1. Validar que las 4 precondiciones de §4 se cumplen con datos verificables (no impresiones).
2. Implementar §2.1 antes que §2.2 (similitud es prerrequisito conceptual de drift).
3. Definir el formato exacto de `policies/memory.yaml` y armonizarlo con `policies/risk.yaml`.
4. Escribir ADR en `docs/adr/` documentando la decisión de añadir validación a la memoria, citando los 2+ casos reales que la justificaron.
5. Confirmar que la implementación NO reintroduce ninguno de los componentes rechazados en §3.
