# 01 — Fase 0a: validación manual de delegación (gate G0)

> **ARCHIVADO 2026-07-23 — IMPLEMENTADO.** Gate G0 ratificado 2026-07-11 (ver resultados-fase-0a.md §7).

**Tipo:** experimento manual, **cero código en el repo**.
**Depende de:** nada. **Gate que produce:** G0 — si delegar a mano no funciona, la serie entera se archiva sin haber tocado el core.
**Riesgo sugerido:** low.

## Contexto

Antes de escribir bundler, wrappers o router, hay que probar la tesis mínima con el costo de una tarde: que un CLI externo, invocado headless dentro de un worktree de Quorum con el contexto de los artefactos, produce un diff que pasa `q-verify`. Si esto falla a mano, ninguna automatización lo arregla.

## Objetivo

Orquestar **a mano** (Claude Code + Bash, sin tooling nuevo) la implementación de 1–2 tareas hijas reales: una con `codex exec`, una con `agy --print`. Verificar con el flujo Quorum normal.

## Alcance

1. Elegir 1–2 hijas reales de complejidad S/M y riesgo low (de un proyecto consumidor o de este repo).
2. Armar el prompt a mano: contenido de `00`+`01`+`02` + instrucción mínima ("aplica los cambios en este worktree; deja notas de qué hiciste y por qué"). Entrada por stdin (codex) / prompt razonable (agy). Prohibido interpolar en shell.
3. Ejecutar en el worktree de la tarea: codex con `-C <worktree> --sandbox workspace-write --json -o <file>`; agy con cwd=worktree, `--sandbox`, `--print-timeout`.
4. Correr `q-verify` y `q-review` normalmente sobre el resultado.
5. **Inventario** (subproducto obligatorio, queda escrito en `ideas/fleet/resultados-fase-0a.md`):
   - codex: formato del JSONL de `--json`, qué reporta de usage, utilidad real de `--output-schema`, comportamiento del sandbox.
   - agy: cómo fijar cwd (no se observó flag `-C`; ¿usa el cwd del proceso? ¿`--add-dir`?), formato de salida de `--print` (no hay flag JSON visible), si reporta usage, nombres exactos de modelo para `--model` (mapear contra `agy models`).
   - Para ambos: comportamiento ante prompt multi-KB, ante timeout, y exit codes en fallo.

## No-objetivos

Sin wrappers, sin agents.yaml, sin métricas formales, sin tocar `internal/core`. Esto NO es la Fase 0 de medición (esa es la tarea 10).

## Criterios de aceptación

- [ ] Al menos una hija implementada por codex pasa `q-verify` con orquestación manual.
- [ ] agy ejecutado headless sobre un worktree con resultado evaluado (pase o no: lo que importa es el dato).
- [ ] `resultados-fase-0a.md` existe con el inventario completo y un veredicto **GO / NO-GO** explícito.
- [ ] Si GO: lista de sorpresas que obligan a ajustar docs 02–15 (puede ser vacía).

## Decisiones abiertas para el brief

- Qué tareas hijas concretas usar (¿de este repo dogfooding o de un proyecto consumidor?).
- Si `agy` no permite fijar cwd de forma fiable, ¿queda en la flota v1 o se pospone su adapter (tarea 07)?
