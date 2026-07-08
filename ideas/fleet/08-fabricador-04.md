# 08 — Fabricador del `04-implementation-log.yaml` (compliance by construction)

**Tipo:** función determinista en core, integrada al flujo de dispatch.
**Depende de:** 06 (consume `result.json` + worktree). Independiente de 07.
**Riesgo sugerido:** medium.

## Contexto

Pedirle a un delegado headless que produzca un `04` válido contra schema, en inglés y con estructura exacta, hunde a cualquier modelo en fallos de compliance — y cada fallo quemaría un attempt. La v2 lo resolvió por construcción: al delegado se le pide lo que sabe hacer (diff + notas libres); el artefacto válido lo fabrica el sistema. Restricción dura verificada en el schema real: `implementation-log.schema.json` es estricto (`additionalProperties: false`) — top-level solo `task_id/summary/entries/tdd_red_runs`; cada entry exactamente `changed_files/notes/verify_pending`. **No cabe metadata de flota en el `04`**: eso va a `trace.events[]` (tarea 03).

## Objetivo

Función `FabricateImplementationLog(result, worktree) → 04 válido`, llamada por el dispatch tras un attempt con diff, persistida vía la ruta `SaveArtifact` (validación antes de escribir, como manda el core).

## Diseño propuesto

**Frontera dura entre hechos y narrativa** (este es el punto que evita reintroducir el riesgo por la puerta de atrás):

- **Campos fácticos salen SOLO de git**: `entries[].changed_files` = `git diff --name-only` del attempt. Jamás de lo que el modelo diga que tocó.
- **Campos narrativos salen de las notas del delegado**: `summary` y `entries[].notes` se derivan del bloque `NOTES:` del output (delimitadores pedidos por el protocolo del bundle, tarea 05), con transformación **determinista v1**: extracción + sanitizado + truncado a límites del schema. Si las notas vienen en español u otro idioma, v1 las persiste tal cual con prefijo `[delegate notes]` — imperfecto pero honesto.
- `verify_pending: true` siempre (verify aún no corrió; lo apaga el flujo normal).
- Sin notas parseables → `summary` sintético determinista: `"Delegated implementation via <agent>/<model>; N files changed. See trace events <dispatch_id>."`

**Formateador LLM: explícitamente fuera de v1.** La v2 contemplaba un modelo L0 como formateador de notas; eso introduce una llamada LLM no presupuestada dentro del componente "determinista". Queda en el horizonte (tarea 16) con su gate: solo si la telemetría muestra que las notas crudas degradan el review, y entonces presupuestada y logueada como dispatch propio.

## Criterios de aceptación

- [ ] Todo `04` fabricado pasa `quorum validate` (test con la matriz: notas perfectas / notas sucias multiidioma / sin notas / diff de 1 y de 50 archivos).
- [ ] `changed_files` proviene de git en el 100% de los casos (test: notas que mienten sobre archivos tocados no contaminan el artefacto).
- [ ] Persistencia vía `SaveArtifact`/`task artifact-save` (nunca escritura directa), respetando los invariantes del task manager.
- [ ] Un attempt con diff válido y notas horribles NO cuenta como fallo de compliance (el caso entero desaparece, que era el objetivo).
- [ ] `go test ./...` verde.

## Decisiones abiertas para el brief

- Límites de truncado de `notes`/`summary` (¿el schema impone maxLength? verificar; si no, fijar límite propio razonable).
- ¿Append de entries en attempts sucesivos del mismo task (el manifiesto dice "append per attempt") o un `04` por attempt? Verificar la convención vigente de `q-implement` y seguirla.
- El prefijo `[delegate notes]` para notas sin traducir: ¿aceptable para `q-review` o preferís summary sintético siempre?
