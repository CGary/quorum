# 04 — `quorum analyze complexity-score`: banda de complejidad determinista

> **ARCHIVADO 2026-07-23 — IMPLEMENTADO.** cmd/analyze_complexity_score.go + internal/core/complexity_score.go.

**Tipo:** función pura en core + shim CLI (patrón `analyze` existente).
**Depende de:** 01 (G0). Independiente de 02/03.
**Riesgo sugerido:** low (función pura, aditiva, sin mutación de estado).

## Contexto

El router (tarea 13) necesita `complexity_band` como input, y el lema "Claude propone, la política dispone" prohíbe que un LLM la estime: tiene que ser una función determinista del blueprint, igual que `internal/core/risk.go` lo es del riesgo. Sin esto, la complejidad es vibes. El molde exacto ya existe en el repo: `risk.go` + `cmd/analyze_risk*.go` (request JSON por stdin, respuesta JSON, cero efectos).

## Objetivo

`quorum analyze complexity-score`: stdin `{blueprint_path}` (o blueprint inline) → `{band: "S"|"M"|"L", signals: [...], inputs: {...}}`.

## Diseño propuesto

Señales deterministas desde `01-blueprint.yaml` (mismo espíritu que risk-score):

- `|touch|` — cantidad de archivos del contrato/blueprint.
- Cantidad de símbolos/funciones afectados.
- Dependencias cross-módulo (si el blueprint trae import graph de `blueprint-context`).
- Flags binarios: migración, API pública, cambio de schema.

Cortes S/M/L viven en política (`.agents/policies/complexity.yaml` propuesto — NO hardcodeados en Go), con valores placeholder iniciales declarados como **no calibrados**. La calibración real llega con los datos de la tarea 10 (Fase 0); este doc solo construye la función y la hace configurable. La respuesta incluye `signals` e `inputs` para que la decisión sea auditable y reproducible (requisito de trazabilidad del routing).

Importante: igual que `risk.go` nunca pisa el riesgo humano de `00-spec.yaml`, esta función **no decide nada** — emite una banda; quien la consume (router) la combina con política. Divergencias humano/máquina, si en el futuro la complejidad se declarara en el spec, se registran como evento, no se corrigen en silencio (mismo patrón que `risk_level_divergence`).

## Criterios de aceptación

- [ ] `internal/core/complexity_score.go` + `cmd/analyze_complexity_score.go` siguiendo las convenciones de los `analyze` existentes (stdin JSON, shim fino).
- [ ] Cortes en `.agents/policies/complexity.yaml`, leídos en runtime; cambiar el YAML cambia la banda sin recompilar.
- [ ] Tests por tabla: blueprints sintéticos S/M/L + bordes exactos de los cortes + blueprint malformado (error claro, no pánico).
- [ ] Documentado en CLAUDE.md (sección "Analyze CLI surface") y `--help` coherente.
- [ ] `go test ./...` verde con `CGO_ENABLED=0`.

## Decisiones abiertas para el brief

- ¿Banda única (S/M/L) o score numérico + banda (como risk emite señal+nivel)? Propuesta: banda + señales, sin número mágico.
- ¿`complexity.yaml` propio o sección dentro de `risk.yaml`? Propuesta: archivo propio — riesgo y complejidad son dimensiones ortogonales y mezclarlas invita a confundirlas.
- Valores placeholder de los cortes (ej. S: ≤2 archivos y ≤3 símbolos; L: >5 archivos o migración) — proponer en la entrevista, marcar como no calibrados.
