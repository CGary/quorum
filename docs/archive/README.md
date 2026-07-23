# Archivo de documentación histórica

Convención RFC/ADR: los documentos de propuesta ya ejecutados o superados por el código real
nunca se borran. Se mueven aquí con `git mv` (preservando la historia de Git) y llevan un banner
`> **ARCHIVADO ... — IMPLEMENTADO.**` con la evidencia de su implementación. Las propuestas aún
vivas o diferidas siguen en `ideas/` y `ideas/fleet/`.

## Contenido

- `0-sistema-inteligente-enrutamiento-tareas-entre-múltiples-llms.md` — doc origen del dispatcher multi-LLM (v1).
- `0-sistema-inteligente-enrutamiento-tareas-entre-múltiples-llms.analysis-v2.md` — análisis de factibilidad v2 del mismo dispatcher.
- `0-sistema-inteligente-enrutamiento-tareas-entre-múltiples-llms.plan-fases.md` — plan de fases derivado del análisis v2.
- `bug-fleet-dispatch-argv-e2big.md` — bug de `quorum fleet dispatch` con E2BIG en bundles grandes.
- `fleet/` — serie de tareas 01–17 (excepto 12 y 16) que implementaron el dispatcher de flota; ver `ideas/fleet/00-indice.md` para el índice y estado completo.
