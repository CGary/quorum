# 05 — Context bundler: `quorum fleet bundle`

**Tipo:** comando nuevo en core (`cmd/` + `internal/core/`), determinista, sin LLM.
**Depende de:** 02 (ADR-A define dónde viven las cosas de flota). Independiente de 04.
**Riesgo sugerido:** medium (es la encarnación de la regla constitucional #2 para delegación; un bundler con fugas rompe "contexto determinista").

## Contexto

Los delegados headless reciben UN payload y mueren. Ese payload hoy se armaría a mano (Fase 0a) con riesgo de inconsistencia y de interpolación shell. La constitución #2 exige contexto determinista derivado del contrato, nunca el repo entero. El bundler es el componente con nombre propio que la v2 introdujo para esto, con el data-gate como puerta futura (tarea 16).

## Objetivo

`quorum fleet bundle <ID>` → construye en el directorio del dispatch un bundle **determinista**: mismos artefactos ⇒ mismo contenido ⇒ mismo hash.

## Diseño propuesto

Contenido del bundle (un archivo de prompt + manifest):

1. `00-spec.yaml` + `01-blueprint.yaml` + `02-contract.yaml` (verbatim).
2. **Protocolo mínimo en inglés** (plantilla fija versionada): "aplicá los cambios en este worktree respetando `touch`/`forbid`; dejá notas libres de decisiones y bloqueos; si no podés proceder, emití el señal BLOCKED con este formato…" — lo que todo modelo de código sabe hacer. NO se le pide el protocolo Quorum completo (schemas, idioma de artefactos): eso lo garantiza el wrapper (tareas 06/08), compliance by construction.
3. Slices de archivos derivados del `context_bundle` del contrato (rutas + rangos), jamás archivos fuera de él.
4. Framing anti-inyección: el contenido de archivos del repo va marcado como DATOS, las instrucciones del sistema separadas (mitigación parcial y declarada; las compuertas post-hoc siguen siendo la red).

Manifest JSON: `{task_id, bundle_hash, files: [...], protocol_version, created_at}`. El hash va al evento `dispatch_started` (convención de la tarea 03).

**Entrega al CLI:** el bundler produce archivo; el wrapper lo pasa por stdin o `@prompt_file` según `agents.yaml.input`. **La interpolación shell del prompt queda prohibida por construcción** — no existe código que la haga.

## No-objetivos

- Data-gate por `data_classification` (tarea 16, gated): el bundler v1 deja el punto de extensión (función `gate(bundle, provider) error` que hoy siempre pasa), no la política.
- Retrievers AST/import-graph como enriquecimiento: el bundle usa lo que el blueprint ya trae; enriquecer el blueprint es trabajo de `q-blueprint`, no del bundler.

## Criterios de aceptación

- [ ] Mismo input ⇒ hash idéntico (test de estabilidad; ordenamiento de archivos y timestamps fuera del hash).
- [ ] Bundle de una tarea real cabe en los límites de prompt de codex y agy (verificar contra inventario Fase 0a; si excede, truncado determinista por prioridad declarada en el manifest).
- [ ] Cero rutas fuera del `context_bundle` del contrato (test negativo).
- [ ] Plantilla de protocolo versionada (`protocol_version` en manifest) y en inglés.
- [ ] Golden test del bundle completo de una tarea sintética.
- [ ] `go test ./...` verde.

## Decisiones abiertas para el brief

- Ubicación del bundle en disco: propuesta `.ai/tasks/active/<ID>/dispatch/<dispatch_id>/` (gitignorado por las reglas runtime existentes) — ¿confirma?
- ¿El protocolo mínimo pide formato específico para las notas (delimitadores) para facilitar el parseo del fabricador del 04? Propuesta: sí, bloque `NOTES:` delimitado, con fallback a texto libre.
- Política de truncado si el bundle excede límites (¿prioridad: contrato > spec > slices?).
