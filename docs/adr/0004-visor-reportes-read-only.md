# 0004: Ampliación de Misión — Visor de Reportes Read-Only (`quorum serve` + `q-report`)

**Date:** 2026-05-31
**Status:** Accepted

## Context

El manifiesto (`quorum.md`), en la sección **"What Quorum IS NOT"**, declara explícitamente que Quorum **no es** *"a Documentation Tool: It does not generate stakeholder reports"* ni *"a Human-Centric UI"*. La autoridad canónica es el manifiesto: cuando el código y el manifiesto no concuerdan, gana el manifiesto y el código se considera el bug.

Existe, sin embargo, un problema operativo real: revisar muchas salidas densas de agentes y de la CLI genera fatiga cognitiva en el humano que debe inspeccionarlas. Se propuso una feature en dos piezas:

1. Un skill `/q-report` que rellena, desde una plantilla y en una sola pasada, un archivo **YAML** de reporte con una estructura predefinida orientada a reducir esa fatiga.
2. Un comando `quorum serve` que levanta un servidor configurable de **solo lectura** y sirve un visor estático (embebido vía `go:embed`) capaz de listar proyectos desde la SQLite central y mostrar los reportes de cada proyecto.

Ambas piezas son, literalmente, generación y visualización de reportes para humanos: justo lo que la sección "What Quorum IS NOT" rechaza. La feature **no viola ninguna de las 9 Reglas Inmutables**, pero contradice una autodefinición del manifiesto. Por la regla "gana el manifiesto", construirla sin registrar la decisión convertiría la feature en una violación. Este ADR mueve esa frontera a propósito y deja rastro auditable.

## Decision

Se amplía la misión para **sancionar una capacidad acotada de visualización de reportes de solo lectura**, bajo las siguientes condiciones normativas vinculantes:

1. **Solo lectura.** `quorum serve` NUNCA muta estado de tarea, NUNCA escribe en `.ai/tasks/` y NUNCA escribe en la SQLite de memoria (cuyo `CHECK (type IN ('pattern','decision','lesson'))` ya rechaza estructuralmente cualquier reporte). La SQLite se lee únicamente para enumerar la tabla `projects`. El host de escucha es configurable para permitir acceso desde otras interfaces cuando el operador lo solicita; el valor por defecto sigue siendo `127.0.0.1`.

2. **La autoría es un skill auxiliar.** `/q-report` es un skill de un solo propósito, **NO** una fase del ciclo SDC (`00`–`07`). Cumple el protocolo de skills (salida en español, valores persistidos en inglés, `ESPERANDO RESPUESTA DEL USUARIO...` solo en turnos de espera) y **no tiene auto-transición** (no es ninguna de las tres autorizadas). No auto-activa otros skills (Regla #9 intacta).

3. **El reporte es un artefacto transitorio/auxiliar, no numerado.** Vive en `<root_path>/.ai/reports/<id>.yaml`, gitignored como `.ai/tasks/`. No es verdad de código (Regla #1 intacta) ni un slot de ciclo de vida `00`–`07`. Por tanto NO requiere relajar la frontera de "no nuevo artefacto numerado".

4. **Reuso de maquinaria, no infraestructura nueva.** El esqueleto se copia desde `.agents/templates/report.yaml`; la validación reusa `.agents/schemas/report.schema.json` a través del motor existente `internal/core/schema.go` (`ValidateArtifact`). El vínculo proyecto↔reporte es por carpeta: `quorum serve` lee `projects.root_path` (poblado por `EnsureMemoryProject`) y escanea `<root_path>/.ai/reports/`.

5. **La Constitución queda intacta.** El visor no emite veredicto y no tiene autoridad de merge: Regla #4 (Validation is Finality) y Regla #6 (The System Commits, Never Merges) se mantienen sin cambios. La memoria sigue curada y human-invoked (Memory Governance intacta).

6. **Capacidad acotada.** El alcance es visualizar reportes autorados por `q-report`. La visualización de artefactos del ciclo (`07-trace.json`, `05-validation.json`, `06-review.json`) queda **fuera** de este ADR. Cualquier expansión adicional (export a PDF, temas, constructor de reportes, mutación de estado) requiere un ADR posterior.

7. **Enmienda del manifiesto.** La sección "What Quorum IS NOT" de `quorum.md` debe enmendarse para referenciar este ADR, de modo que el código y el manifiesto vuelvan a concordar. Hasta que esa enmienda se aplique, este ADR es el registro de la intención.

## Consequences

- **Positivas:** Reduce la fatiga cognitiva al revisar salidas densas; reusa el patrón plantilla+schema, el motor de validación y la tabla `projects` existentes; la ampliación de misión queda explícita y auditable en vez de oculta en un commit.
- **Negativas:** Expande la superficie de Quorum más allá de un motor de ejecución puro; riesgo de scope creep (temas, export, edición), acotado por las condiciones 1–6 y por el requisito de ADR posterior.
- **Neutrales:** No se introduce ningún artefacto numerado de ciclo de vida; los reportes son archivos transitorios auxiliares; la gobernanza de la SQLite no cambia. Este ADR no supersede a ninguno previo; se relaciona con 0003 (memoria centralizada / tabla `projects`).
