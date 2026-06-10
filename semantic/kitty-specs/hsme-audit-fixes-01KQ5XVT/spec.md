# HSME Audit Fixes — Observability Env, Vector Search, Rollup Catch-up

## Purpose

### TLDR
Reparar tres regresiones halladas en auditoría con escenarios BDD que fallen antes y pasen después del fix.

### Context
Una auditoría con DB y binarios aislados detectó que (1) search_fuzzy con filtro de proyecto cae a solo lexical por restricción de vec0 violada en JOIN+LIMIT; (2) el README documenta OBS_LEVEL pero el código solo lee HSME_OBS_LEVEL apagando toda la observabilidad; y (3) el rollup procesa únicamente el bucket actual descartando buckets perdidos. Cada fix se considera done solo cuando un escenario BDD reproduce el comportamiento defectuoso, falla previo al fix y pasa después.

## User Scenarios & Testing

### FR-001: Búsqueda semántica con filtro de proyecto

**Scenario: search_fuzzy con project filter retorna resultados vectoriales**
- **Given**: un proyecto "acme" existe en la base de datos con documentos indexados
- **And**: la base de datos tiene soporte vec0 habilitado (sqlite_fts5 con vec)
- **When**: el usuario ejecuta `search_fuzzy query="consulta test" project="acme" k=10`
- **Then**: el resultado incluye candidaros vectoriales (cobertura "complete" no es suficiente)
- **And**: no se dispara el error "A LIMIT or 'k = ?' constraint is required on vec0 knn queries"
- **And**: el campo `coverage` del resultado refleja candidaros de embedding, no solo lexical

**Scenario: search_fuzzy sin project filter funciona normalmente**
- **Given**: un proyecto "acme" existe con documentos indexados
- **When**: el usuario ejecuta `search_fuzzy query="consulta test" k=10` (sin project)
- **Then**: la búsqueda retorna resultados mixtos (vector + lexical) si hay embedding
- **And**: RRF fusiona correctamente los candidatos

**Scenario de rollback: project filter sin vec0 disponible**
- **Given**: la base de datos no tiene soporte vec0
- **When**: se ejecuta search_fuzzy con project filter
- **Then**: el sistema cae a búsqueda lexical gracefully
- **And**: retorna un coverage="partial" con mensaje claro

---

### FR-002: Observabilidad con variable de entorno correcta

**Scenario: HSME_OBS_LEVEL=trace produce trazas**
- **Given**: el proceso hsme se inicia con `HSME_OBS_LEVEL=trace`
- **When**: se ejecutan operaciones de store y search
- **Then**: las tablas `obs_traces`, `obs_spans`, y `obs_events` contienen registros
- **And**: `obs_traces` tiene > 0 rows
- **And**: `obs_spans` tiene > 0 rows
- **And**: `obs_events` tiene > 0 rows

**Scenario: OBS_LEVEL (sin HSME_ prefix) no produce trazas**
- **Given**: el proceso hsme se inicia con `OBS_LEVEL=trace` (sin HSME_ prefix)
- **When**: se ejecutan operaciones de store y search
- **Then**: las tablas de observabilidad permanecen vacías
- **And**: no se genera ningún warning de variable desconocida

**Scenario: README actualizado refleja la variable correcta**
- **Given**: un nuevo usuario lee el README
- **When**: sigue las instrucciones de configuración de observabilidad
- **Then**: la variable documentada (`HSME_OBS_LEVEL`) funciona correctamente
- **And**: no necesita buscar en el código fuente para descubrir la variable correcta

---

### FR-003: Rollup catch-up para buckets perdidos

**Scenario: rollup procesa bucket actual**
- **Given**: el servicio hsme-ops corriendo con rollup configurado
- **When**: el cron trigger ejecuta runRawToMinute
- **Then**: el bucket de `now.Truncate(minute)` se procesa correctamente
- **And**: `last_completed_bucket_start_utc` se actualiza al bucket procesado

**Scenario: rollup hace catch-up de buckets perdidos**
- **Given**: el servicio hsme-ops estuvo caído por 5 minutos
- **And**: los buckets [T-5min, T-4min, T-3min, T-2min] no fueron procesados
- **When**: el servicio se reinicia y ejecuta runRawToMinute
- **Then**: los 5 buckets perdidos se procesan en orden
- **And**: `last_completed_bucket_start_utc` avanza secuencialmente por cada bucket
- **And**: ningún bucket se pierde por gaps de retención

**Scenario: rollup con checkpoint persistente**
- **Given**: el servicio se reinicia tras procesar hasta bucket T-10min
- **When**: runRawToMinute se ejecuta
- **Then**: el procesamiento comienza desde `last_completed_bucket_start_utc` = T-10min
- **And**: no se reprocesa el bucket T-10min
- **And**: se procesan T-9min, T-8min, ..., T (hasta el bucket actual)

**Scenario de edge case: gaps mayores a ventana de retención**
- **Given**: el servicio estuvo caído por más de 7 días (retention window)
- **When**: se reinicia y ejecuta runRawToMinute
- **Then**: solo se procesan buckets dentro de la ventana de retención
- **And**: buckets fuera de retención se ignoran sin error
- **And**: `last_completed_bucket_start_utc` se actualiza al bucket más antiguo procesable

---

## Functional Requirements

| ID | Requirement | Status |
|----|-------------|--------|
| FR-001 | search_fuzzy con filtro `project=X` debe ejecutar la query vectorial correctamente incluyendo el constraint `k = ?` en el query plan de vec0 | pending |
| FR-002 | El sistema debe documentar y usar consistentemente `HSME_OBS_LEVEL` como variable de entorno (no `OBS_LEVEL`) | pending |
| FR-003 | runRawToMinute y runDerivedRollup deben leer `last_completed_bucket_start_utc` para determinar el punto de inicio del próximo procesamiento | pending |
| FR-004 | Cada fix debe tener un escenario BDD que falle en main previo al fix y pase después | pending |

---

## Non-Functional Requirements

| ID | Requirement | Threshold | Status |
|----|-------------|-----------|--------|
| NFR-001 | Cobertura de tests BDD para Issue #1 (search_fuzzy con project) | 1 escenario que falla antes del fix | pending |
| NFR-002 | Cobertura de tests BDD para Issue #2 (OBS_LEVEL mismatch) | 1 escenario que falla antes del fix | pending |
| NFR-003 | Cobertura de tests BDD para Issue #3 (rollup catch-up) | 1 escenario que falla antes del fix | pending |
| NFR-004 | No regresión: search_fuzzy sin project filter sigue funcionando como antes | 100% backward compatible | pending |

---

## Constraints

| ID | Constraint | Rationale |
|----|------------|-----------|
| C-001 | El fix de FR-001 no debe cambiar el comportamiento de search_fuzzy sin project filter | No regresión |
| C-002 | El fix de FR-002 debe ser documentación (README update) + código (verificar que HSME_OBS_LEVEL es la única variable leída) | Asegurar que el fix es completo |
| C-003 | El fix de FR-003 no debe reprocesar buckets ya marcados como `last_completed_bucket_start_utc` | Eficiencia, evitar duplicación |
| C-004 | El proceso de catch-up debe ser idempotente: correr dos veces el mismo bucket produce el mismo resultado | Consistencia |

---

## Success Criteria

| ID | Criterion | Measurement |
|----|-----------|-------------|
| SC-001 | search_fuzzy con project filter retorna resultados vectoriales | Query a base de test con vec0 retorna coverage=complete con candidatos embedding |
| SC-002 | HSME_OBS_LEVEL=trace produce datos en obs_traces/spans/events | 6+ traces, 33+ spans, 2+ events con trace activo |
| SC-003 | Rollup recupera buckets perdidos al reiniciar | 5 buckets perdidos se procesan en < 1 minuto tras restart |
| SC-004 | Los 3 escenarios BDD fallan en main y pasan tras el fix | godog (o equivalente BDD) reporta 0 failures post-fix |

---

## Key Entities

| Entity | Description |
|--------|-------------|
| `search_fuzzy` | Función de búsqueda híbrida lexical + vectorial con soporte a filtros |
| `HSME_OBS_LEVEL` | Variable de entorno que controla el nivel de logging de observabilidad |
| `last_completed_bucket_start_utc` | Checkpoint de progreso del rollup, persiste el último bucket procesado |
| `runRawToMinute` | Función de rollup que agrega métricas raw por minuto |
| `runDerivedRollup` | Función de rollup que calcula métricas derivadas |

---

## Assumptions

| ID | Assumption |
|----|------------|
| AS-001 | La base de datos de test tiene soporte vec0 habilitado (sqlite_fts5 con vec) |
| AS-002 | El framework BDD a usar será godog (Ginkgo BDD o similar es decisión de plan) |
| AS-003 | La ventana de retención para rollup es configurable y default es 7 días |
