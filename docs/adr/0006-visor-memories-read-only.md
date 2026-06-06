# ADR 0006: Visor read-only de memoria curada

## Estado

Aceptado

## Contexto

ADR 0004 permitió que `quorum serve` visualice reportes semánticos read-only desde proyectos registrados. Quorum también mantiene memoria curada en SQLite, ingerida exclusivamente por `q-memory` mediante `quorum memory save`. Exponer esa memoria en el visor ayuda a revisar decisiones, patrones y lecciones por proyecto, pero no debe convertir al visor en una ruta alternativa de ingestión ni debilitar las invariantes de evidencia del ciclo SDC.

## Decisión

`quorum serve` puede exponer memoria curada por proyecto mediante handlers HTTP estrictamente read-only.

Permitido:

- Listar memorias de un proyecto registrado con `root_path` válido.
- Ver el detalle completo de una memoria curada del proyecto.
- Filtrar o buscar memorias usando consultas SQL de solo lectura sobre tablas normalizadas y satélite.

Prohibido:

- Crear, editar, borrar, exportar o promover memorias desde el visor o sus handlers.
- Ejecutar `SaveMemoryEntry`, `EnsureMemoryProject`, migraciones, o cualquier escritura SQLite desde handlers del visor.
- Usar el visor como ruta de ingestión: `q-memory` sigue siendo la única ruta de captura curada.
- Visualizar artefactos lifecycle `05-validation.json`, `06-review.json` o `07-trace.json` desde esta superficie.

## Consecuencias

La API del visor puede añadir endpoints `GET` para memorias, pero sus handlers deben limitarse a `SELECT` y deben respetar aislamiento por `project_id`. Los proyectos sin `root_path` siguen omitidos de `/api/projects`, y sus subrutas directas deben responder como no disponibles. La memoria permanece subordinada a Git, a los artefactos lifecycle y a la ingestión curada de `q-memory`; el visor no establece verdad de código ni evidencia de finalización.
