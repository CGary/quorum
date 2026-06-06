# 0005: Modelo de Reporte Semántico Puro (eliminación del modelo legacy)

**Date:** 2026-06-06
**Status:** Accepted

## Context

La propuesta `ideas/10-report-semantico-con-perfiles-presentacion.md` introdujo el modelo de reporte semántico (`kind`, `presentation`, `content.sections[]` con roles) sobre la base del modelo de reporte previo ("legacy"), cuyos componentes eran propiedades top-level (`verdict` string, `summary`, `decisionSurface`, `keyFindings`, `callouts`, `verify`, `diagrams`, `findings`, `evidence`, `tradeoffs`, `risks`, `actionPlan`, `appendix`).

Esa propuesta especificó (§4.2, §8 Fase 1, y las decisiones K/Z/I/AC de §15) que ambos modelos debían **coexistir** como una **unión exclusiva**, discriminada en la raíz del schema mediante `if`/`anyOf: [content, kind, presentation]` → `semanticModel`, `else` → `legacyModel`, evitando `oneOf` para preservar los mensajes `field=...; reason=...` de `internal/core/schema.go`. La ventana de compatibilidad legacy debía durar hasta `schemaVersion: "2.0"`.

La implementación tomó una decisión **divergente**: el commit `fbb5b27` ("Remove legacy report backward compatibility, enforce semantic v1.1 model") eliminó por completo el modelo legacy en lugar de mantenerlo. El schema (`.agents/schemas/report.schema.json`) quedó como **semántico puro**: la raíz es `"$ref": "#/$defs/semanticModel"`, no existe `legacyModel` ni discriminador raíz `if`/`then`. Este ADR registra esa decisión y marca como obsoletas las secciones de la idea que asumían coexistencia.

## Decision

Se adopta un **modelo de reporte único y semántico (v1.1)**, sin modelo legacy, bajo estas condiciones:

1. **Raíz semántica directa.** `report.schema.json` valida contra `semanticModel` directamente (`meta`, `kind`, `presentation`, `content` requeridos; `additionalProperties: false`). No hay rama legacy ni discriminador raíz. Las decisiones **K, Z, I (mitad legacy), A** y la §4.2 de la idea quedan **obsoletas**: ya no aplica la unión exclusiva ni el `if`/`anyOf` raíz, porque no hay dos modelos que discriminar.

2. **`schemaVersion` fijo en `"1.1"`.** `metaSemantic` declara `schemaVersion` como `const "1.1"`. La lógica de "coherencia modelo↔versión" y de autofill `1.0` para legacy (decisión F, §8 Fase 1.3) queda **obsoleta**: no existe formato `1.0` que autocompletar. `fillReportMetadata` solo estampa `1.1` y `date` cuando faltan.

3. **Validación por role intacta.** La parte semántica de la idea (enum cerrado de `role`, validación `if`/`then` por role con `const` + `additionalProperties: false`, hook Go post-schema para unicidad de `id`/finding y referencia `evidence.findingId`) **se mantiene vigente** tal como la idea la especificó (decisiones C, AA, B, N, U, L, M, AD). Este ADR NO altera esos invariantes.

4. **Migración legacy preservada como herramienta puntual.** `quorum report migrate <id>` (`cmd/report.go`) sigue existiendo para **convertir** archivos legacy `1.0` en disco a la forma semántica `1.1`. El schema ya no acepta legacy, pero la herramienta de migración permite recuperar reportes antiguos sin pérdida. La migración es read-only salvo `--output`/escritura explícita.

5. **Constitución intacta.** Los reportes siguen fuera del ciclo `00`–`07`, `quorum serve` sigue siendo read-only, y `q-report` sigue siendo single-phase sin auto-transición. Este ADR no introduce artefactos numerados (consistente con 0004).

## Consequences

- **Positivas:** Schema más simple y mantenible (sin doble modelo ni discriminador raíz frágil); un único formato de autoría; menos superficie de error en los mensajes de validación. El test `TestReportCatalogDocsInSyncWithSchema` queda naturalmente verde porque `legacyModel` ya no existe (`getProperties("legacyModel")` devuelve nil, el bucle legacy es no-op).
- **Negativas:** Se pierde la compatibilidad de lectura/render legacy que la idea prometía hasta `2.0`. Reportes legacy preexistentes en disco NO validan contra el schema actual y deben pasar por `quorum report migrate` antes de guardarse o visualizarse.
- **Neutrales:** La idea `ideas/10-...md` queda parcialmente superada por este ADR en sus secciones §4.2, §8 Fase 1.3 y las decisiones K/Z/I/AC/F. El resto de la idea (modelo semántico, roles, perfiles de presentación, viewer) sigue siendo la referencia vigente. Este ADR se relaciona con 0004 (visor read-only) y no supersede a ningún ADR previo.
