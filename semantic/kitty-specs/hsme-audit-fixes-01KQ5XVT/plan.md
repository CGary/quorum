# Implementation Plan: HSME Audit Fixes — Observability Env, Vector Search, Rollup Catch-up

**Mission**: hsme-audit-fixes-01KQ5XVT
**Branch**: main | **Date**: 2026-04-26 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/kitty-specs/hsme-audit-fixes-01KQ5XVT/spec.md`

## Summary

Reparar tres regresiones de auditoría en el MCP server HSME (Go/SQLite):
1. **FR-001**: `search_fuzzy` con filtro `project=X` falla en el branch vectorial por violación de constraint `k = ?` en vec0 KNN queries — fallback silencioso a lexical nomás
2. **FR-002**: README documenta `OBS_LEVEL` pero el código lee `HSME_OBS_LEVEL` — fix 100% documentación
3. **FR-003**: `runRawToMinute` no lee `last_completed_bucket_start_utc` para hacer catch-up de buckets perdidos

Cada fix requiere escenarios BDD (godog) que fallen en main y pasen después del fix.

---

## Technical Context

**Language/Version**: Go 1.21+
**Primary Dependencies**: sqlite_fts5, sqlite_vec (vec0 virtual table), Ollama (nomic-embed-text), stdlib SQLite driver
**Storage**: SQLite con ext FTS5 + vec0
**Testing**: godog (BDD Gherkin) — escenarios que fallan pre-fix / pasan post-fix
**Target Platform**: Linux server (hsme binary, hsme-worker, hsme-ops)
**Performance Goals**: NFRs documentados en spec (P50 ≤ 100ms para search, 0 cross-project leakage)
**Scale/Scope**: MCP server con 3 commands (hsme, worker, ops) — repo único
**Project Type**: Go single-binary MCP server con plugins de ext SQLite

### Key Source Locations (from HSME memory 1014, 1015, 993)

| File | Relevance |
|------|-----------|
| `src/core/search/fuzzy.go:387-417` | FR-001 — VectorSearch project branch con JOIN que viola constraint vec0 |
| `src/core/search/fuzzy.go:186-189` | FR-001 — silent fallback cuando vectorResults = nil |
| `src/observability/config.go:32` | FR-002 — lee `HSME_OBS_LEVEL` correctamente (código OK) |
| `README.md:127` | FR-002 — dice `OBS_LEVEL` (doc wrong) |
| `src/observability/recorder.go` | FR-002 — recorder que hace no-op cuando Level=Off |
| `cmd/ops/main.go` | FR-003 — hsme-ops runner |
| `src/storage/sqlite/observability.go` | FR-003 — schema de rollup y checkpoint |
| `src/observability/maintenance.go` | FR-003 — donde debería estar el catch-up loop |

### BDD Framework Decision

**godog** (Go port de Cucumber/Gherkin) — preferido porque:
- Sintaxis Given/When/Then legible para negocio y devs
- Runner standalone: `godog run ./tests/bdd/`
- Es el estándar BDD en ecosistemas Go
- Alternativas (Ginkgo BDD) son más "Go-native" pero menos legibles para no-Go-experts

---

## Engineering Alignment

**3 Work Packages independientes** (una por bug fix + coverage BDD):

| WP | FR | Fix | Testing |
|----|----|-----|---------|
| WP01 | FR-001 | `fuzzy.go` VectorSearch project branch — restructure query para que LIMIT se aplique en el scan KNN antes del JOIN | godog scenario: falla pre-fix con vec0 error, pasa post-fix con coverage=vectorial |
| WP02 | FR-002 | `README.md` — cambiar `OBS_LEVEL` → `HSME_OBS_LEVEL` | godog scenario: falla pre-fix con 0 rows en obs_tables, pasa post-fix con 6+/33+/2+ rows |
| WP03 | FR-003 | `ops/main.go` + `maintenance.go` — leer `last_completed_bucket_start_utc` e iterar gaps | godog scenario: falla pre-fix (solo procesa current bucket), pasa post-fix (procesa 5 buckets perdidos) |

**Invariants**:
- FR-001 fix no debe afectar search_fuzzy sin project filter (C-001)
- FR-002 fix es solo doc — código ya está correcto (C-002)
- FR-003 catch-up debe ser idempotente (C-004) y no reprocesar buckets ya completados (C-003)

**Rollup catch-up design** (from HSME memory 993):
- Leer `last_completed_bucket_start_utc` de `obs_rollup_jobs`
- Calcular gaps: desde last_completed + 1 minute hasta `now.Truncate(minute)`
- Iterar sobre cada bucket faltante y llamar `processBucket(ts)`
- Actualizar checkpoint después de cada bucket exitoso
- Manejar edge case: gaps > retention window (7 días) — solo procesar dentro de ventana

---

## Charter Check

**GATE**: Must pass before Phase 0 research. Re-check after Phase 1 design.

- Charter file: `.kittify/charter/charter.md` — **NO EXISTE** (skip)
- No charter conflicts detected — sin governance constraints, se aplican defaults de proyecto

---

## Project Structure

### Source Code (repository root)

```
/home/gary/dev/hsme/
├── cmd/
│   ├── hsme/main.go           # MCP server binary
│   ├── worker/main.go         # background worker binary
│   └── ops/main.go            # ops runner (rollup/maintenance)
├── src/
│   ├── core/search/
│   │   └── fuzzy.go           # FR-001 fix location
│   ├── observability/
│   │   ├── config.go          # FR-002 code (already correct)
│   │   ├── recorder.go       # observability recorder
│   │   └── maintenance.go    # FR-003 catch-up logic
│   └── storage/sqlite/
│       └── observability.go   # FR-003 schema
├── tests/
│   └── bdd/                   # godog feature files
│       ├── search_fuzzy_project.feature
│       ├── observability_env.feature
│       └── rollup_catchup.feature
├── README.md                  # FR-002 doc fix location
└── kitty-specs/hsme-audit-fixes-01KQ5XVT/
    ├── spec.md
    ├── plan.md               # This file
    └── tasks/                # Generated by /spec-kitty.tasks
```

---

## Phase 0: Research

**Research done via HSME memory lookup** (memories 1014, 1015, 993, 988):

### FR-001: search_fuzzy project filter vec0 constraint violation

**Root cause confirmed**: `fuzzy.go:387` hace JOIN de 3 tablas en branch vectorial con project:
```sql
SELECT rowid FROM memory_chunks_vec v
  JOIN memory_chunks c ON c.id = v.rowid
  JOIN memories m ON m.id = c.memory_id
WHERE v.embedding MATCH ? AND m.project = ?
LIMIT ?
```
El error: `A LIMIT or 'k = ?' constraint is required on vec0 knn queries` — vec0 solo acepta LIMIT cuando se aplica directamente al scan KNN, no después de un JOIN.

**Fix options analyzed**:
- Option A: Inject `k = ?` constraint como condición inline de columna vec0
- Option B (preferred): CTE con KNN primero, luego JOIN — `WITH knn AS (SELECT rowid FROM memory_chunks_vec WHERE embedding MATCH ? LIMIT ?) SELECT ... FROM knn k JOIN ... WHERE m.project = ?`
  - Consideración: puede retornar fewer than `limit` rows post-filter; solution: over-fetch (LIMIT k*N) y trimmear

**Test approach**: godog scenario con DB de test que tiene vectores + project tags. Falla pre-fix con vec0 error en logs y coverage=partial. Pasa post-fix con coverage=complete.

### FR-002: OBS_LEVEL documentation mismatch

**Root cause confirmed**: Código 100% correcto (`src/observability/config.go:32` lee `HSME_OBS_LEVEL`). Bug es solo en README.

**Fix**: Cambiar `README.md:127` de:
```json
"env": { "OBS_LEVEL": "basic" }
```
a:
```json
"env": { "HSME_OBS_LEVEL": "basic" }
```

**Test approach**: godog scenario — iniciar proceso con `OBS_LEVEL=trace` (wrong), verificar que obs_tables quedan vacías. Luego con `HSME_OBS_LEVEL=trace`, verificar que se llenan.

### FR-003: Rollup catch-up

**Root cause confirmed** (from post-merge audit memory 993): checkpoint se persiste pero `runRawToMinute` no lo lee. Solo procesa `now.Truncate(minute)`.

**Fix approach**:
```go
func runRawToMinute(ctx context.Context, db *DB) error {
    lastCheckpoint := getLastCompletedBucket(db) // lee obs_rollup_jobs.last_completed_bucket_start_utc
    currentBucket := now.Truncate(time.Minute)

    for ts := lastCheckpoint.Add(1*time.Minute); !ts.After(currentBucket); ts = ts.Add(time.Minute) {
        if err := processBucket(ctx, db, ts); err != nil {
            return err
        }
        updateCheckpoint(db, ts) // actualiza last_completed_bucket_start_utc
    }
    return nil
}
```

**Test approach**: godog scenario con servicio caído 5 minutos. Falla pre-fix (solo bucket actual). Pasa post-fix (5 buckets procesados en orden).

---

## Phase 1: Design & Contracts

### WP01: search_fuzzy project filter fix

**File to modify**: `src/core/search/fuzzy.go` (~line 387-417)

**Design**:
- Cambiar query de project branch a CTE que aplica LIMIT en el scan KNN puro antes del JOIN
- Over-fetch con `LIMIT k*10` para compensar pérdida post-filter
- No cambiar el branch sin project filter

**Contract**: No cambia API pública. Solo comportamiento interno de búsqueda vectorial con filtro.

### WP02: README doc fix

**File to modify**: `README.md` (~line 127)

**Design**: Simple string replacement `OBS_LEVEL` → `HSME_OBS_LEVEL`

**Contract**: Documentación. El runtime no cambia.

### WP03: Rollup catch-up

**Files to modify**:
- `cmd/ops/main.go` — donde se invoca runRawToMinute
- `src/observability/maintenance.go` — loop de catch-up

**Design**:
- Leer checkpoint de `obs_rollup_jobs.last_completed_bucket_start_utc`
- Iterar desde checkpoint+1min hasta current bucket
- Actualizar checkpoint después de cada bucket exitoso
- Idempotencia: verificar si bucket ya fue procesado antes de procesar

**Contract**: No cambia API externa. Solo comportamiento de `hsme-ops run --mode rollup`.

---

## Complexity Tracking

No hay charter violations. Estructura simple: 3 bugs, 3 WPs, 3 archivos modificados en WPs 1 y 3, 1 archivo en WP2.

---

## Open Questions

Ninguna. El spec tenía 3 `[NEEDS CLARIFICATION]` markers que fueron resueltos por HSME research:

1. **FR-001 fix approach**: CTE con KNN primero (Option B) — over-fetch para compensar
2. **FR-002 scope**: Solo documentación (README fix) — código ya está correcto
3. **FR-003 mechanism**: Loop que lee checkpoint e itera gaps — confirmado por memory 993
