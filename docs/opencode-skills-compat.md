# Compatibilidad Skills Quorum ↔ OpenCode — Memoria para futuros agentes

> Fecha: 2026-09-16. Objetivo: base común para llevar TODOS los skills mencionados a 100% compatible en OpenCode, entendido como **(a) válido = OpenCode los descubre y carga** + **(b) ejecutable = OpenCode puede correr TODAS sus instrucciones sin adaptación manual**.
> Fuentes verificadas en disco y docs oficiales (no memoria): `quorum.md` v1.1, `README.md`, `go.mod`, `main.go`, `embed_agents.go`, `.agents/skills/*/SKILL.md`, `.claude/skills/`, `~/.agents/skills/q-orchestrate/SKILL.md` + `assets/subagent-prompt-template.md`, `.agents/fleet/agents.yaml`, `https://opencode.ai/docs/skills`, `https://opencode.ai/docs/agents`, `https://opencode.ai/docs/tools/`.

## 1. Qué es Quorum (resumen mínimo necesario)

- Framework Go que orquesta agentes IA vía **Spec-Driven Contracts (SDC)**. Convierte intención humana en artefactos validados `00→07` + diffs Git verificados. Autoridad canónica: `quorum.md`.
- **NO es** chatbot general, ni herramienta de cambios triviales, ni generador de docs narrativas, ni merge automático.
- Ciclo: `00-spec.yaml` (q-brief) → `01-blueprint.yaml` + `02-contract.yaml` (q-blueprint) → `04-implementation-log.yaml` (q-implement) → `05-validation.json` (q-verify) → `06-review.json` (q-review) → `q-accept` + merge humano → `q-memory` (SQLite curada). `07-trace.json` append-only. Sin `03/08/09/10`.
- Reglas duras: Git es verdad de código; contexto determinista (`context_bundle`); sin parches fuera de `touch`; validación es finalidad (`verify.commands` exit 0); sistema commitea, humano mergea; skills fase-única (solo 3 auto-transiciones forward: brief→blueprint, decompose→split, blueprint→start; `back` siempre humano).
- Monorepo dos módulos Go sin imports cruzados (`go.work`): `quorum` raíz (Go puro, `CGO_ENABLED=0`, `modernc.org/sqlite`, CLI `cobra`) y `github.com/hsme/core` en `semantic/` (HSME, CGO+Ollama, subordinado: informa, no decide).
- Fleet: `quorum fleet route/bundle/dispatch/run/status/enable/disable/smoke/stats/catalog`. Único transporte activo verificado al 2026-09-09+: `opencode_go`. `agy/agy_edit/opencode/aider/codex` en `active: false`.

## 2. Inventario de skills verificado (2026-09-16)

### 2.1 Skills del repo (15, canónicos)

Ubicación doble, contenido idéntico verificado por muestreo (`q-brief`):

- `.agents/skills/*/SKILL.md`
- `.claude/skills/*/SKILL.md`

Lista: `fleet-cli-usage`, `hsme-cli-usage`, `q-accept`, `q-analyze`, `q-blueprint`, `q-brief`, `q-decompose`, `q-dispatch`, `q-implement`, `q-memory`, `q-report`, `q-review`, `q-session`, `q-status`, `q-verify`.

Notas:
- `q-dispatch` es cara humana de `route→bundle→dispatch` implement externo. `fleet-cli-usage` es guía `fleet run` non-lifecycle. `q-session` usa sentinela `SESSION-YYYY-MM-DD`. `q-report` + `quorum serve` es excepción read-only acotada (ADR 0004/0006/0007).
- `quorum init` + binario embebido (`embed_agents.go` → `go:embed all:.agents`) distribuye estos skills. `.claude/skills` en repo aparece como árbol copiado (no symlink verificado aquí; en consumidor `quorum init` crea symlink según AGENTS.md).

### 2.2 Skill global personal (1, fuera del repo)

- `~/.agents/skills/q-orchestrate/SKILL.md` v1.8 (`license: Apache-2.0`, `metadata: {author: gentleman-programming, version: "1.8"}`) + `assets/subagent-prompt-template.md`.
- Posición constitucional declarada en el propio archivo (línea ~379): vive GLOBAL como scaffolding personal per fleet corpus (D40). **No migrar al repo sin Fase 3 / ADR-D (E2).**
- Propósito: orquestar pipeline completo brief→memory delegando cada fase a subagentes efímeros con ruteo por dificultad + Fleet Gate externo primero + ledger de medición.

## 3. Compatibilidad OpenCode — estado actual

### 3.1 Descubrimiento (válido) — ✅ SÍ para todos

OpenCode escanea (doc oficial `/docs/skills` → `Place files`):

1. `.opencode/skills/<name>/SKILL.md` (proyecto)
2. `~/.config/opencode/skills/<name>/SKILL.md` (global)
3. `.claude/skills/<name>/SKILL.md` (proyecto Claude-compat)
4. `~/.claude/skills/<name>/SKILL.md` (global Claude-compat)
5. `.agents/skills/<name>/SKILL.md` (proyecto agent-compat)
6. `~/.agents/skills/<name>/SKILL.md` (global agent-compat)

Consecuencia:
- Los 15 del repo caen en casos 3 y 5 → descubiertos sin mover nada.
- `q-orchestrate` cae en caso 6 → descubierto sin mover nada.

### 3.2 Frontmatter (válido) — ✅ SÍ con advertencia

OpenCode exige solo `name` (requerido) + `description` (requerido). Opcionales reconocidos: `license`, `compatibility`, `metadata` (mapa string→string). **Campos desconocidos se ignoran.**

- Repo: `name: q-brief`, `description: ...`, `user-invocable: true` → carga OK, `user-invocable` ignorado (no rompe).
- `q-orchestrate`: `name`, `description` multilínea, `license: Apache-2.0`, `metadata: {author, version}` → carga OK, 100% dentro del schema.
- Mejora opcional (no bloqueante): añadir `compatibility: opencode` para declarar intención.

### 3.3 Ejecución q-* atómicos — ✅ SÍ con 3 precondiciones

Los 15 del repo son **transport-agnósticos**: solo invocan `quorum task/*`, `quorum analyze/*`, `quorum fleet/*`, `quorum validate/memory/doctor/serve`, `bash`, `git rev-parse`, `hsme-cli ... --project <slug> --json --no-input`, `jq`, `cat/redirección`.

OpenCode tiene `bash`, `skill` (carga on-demand vía `skill` tool), `read/edit/glob/grep/list`, `task`, por lo que las instrucciones corren tal cual **SI**:

1. Binario `quorum` en `PATH` del proceso opencode.
2. `quorum init` corrido en el proyecto consumidor (`.agents/schemas|policies|config.yaml`, `.ai/tasks/`).
3. Repo Git real (worktrees `worktrees/<ID>/` + ramas `ai/<ID>`).

Sin eso, el skill carga pero falla en runtime. No es bug del skill, es falta de cimientos.

### 3.4 Ejecución q-orchestrate — ❌ NO completa (bloqueantes verificados)

Carga SÍ, ejecución completa NO. Bloqueantes Claude-específicos:

| # | Instrucción en SKILL.md | Por qué falla en OpenCode | Evidencia |
|---|---|---|---|
| 1 | `Agent(subagent_type: <rol>)`, `never pass model:` — template `assets/subagent-prompt-template.md` | OpenCode usa `Task` tool + `permission.task` (globs), no `Agent(subagent_type)`. Roles (`arquitecto`, `especificador-sonnet-high`, `ejecutor`, `revisor-profundo`, grid `<role>-<model>-<effort>` ej. `ejecutor-fable-medium`) no existen | `SKILL.md` Step 1 + tabla ruteo; template línea 4-5, 14-15; doc `/docs/agents` → `Task permissions`, `Define custom agent in opencode.jsonc` |
| 2 | Roles pineados en `~/.claude/agents/<rol>.md` (modelo+effort, `disallowedTools` en reviewers) | OpenCode define agentes en `opencode.json` → `agent: {<nombre>: {model, prompt, tools:{write:false}}}`. Ruta `~/.claude/agents/` no resuelve | `SKILL.md` Step 1 línea 66, 72-75; doc `/docs/config` |
| 3 | `Step 0: batch up to 4 AskUserQuestion items` | Nativo OpenCode es herramienta `question` (`permission.question: allow`), schema distinto (header/pregunta/opciones). `AskUserQuestion` solo vía plugin ej. `@kirmad/askuserquestion` | `SKILL.md` Step 0; doc `/docs/tools/` → `question`; plugin verificado vía websearch 2026 |
| 4 | `Core Principle 4: orchestrator runs on OPUS, never fable ... enforced by /model` | `/model` es concepto Claude Code. En OpenCode el modelo se fija por config/agente, no hay equivalencia 1:1 opus/fable/sonnet/haiku | `SKILL.md` línea 34 |
| 5 | `Ledger ~/.claude/quorum-agent-ledger.jsonl` (una línea JSON por fase con `executor/gate_reason/tokens/...`) | Ruta hardcodeada a Claude. Hay que remapear | `SKILL.md` Step 1 líneas 111-136 |
| 6 | `codebase-memory-mcp cli index_status / index_repository --project <slug>` | Solo si ese MCP está configurado en OpenCode. Si no, degradar a grep/Read | `SKILL.md` Subagent Delegation Contract + freshness |
| 7 | Fleet Gate y fases mecánicas (Step 1.5/1.6) | ✅ ESTO SÍ PORTA: `quorum fleet route`, `/q-dispatch`, `quorum analyze complexity-score`, `quorum fleet stats --json`, `quorum fleet run --agent opencode_go --model <de --schema> --cwd <scratch> --input <file> --no-input --json` + `git status --porcelain` | `SKILL.md` Step 1.5-1.6 líneas 139-282 |

Regla invariante que SÍ se conserva en cualquier host (Guardrails 10-11): ruteo siempre vía `quorum fleet route` (nunca el orquestador elige modelo), agotar `reroute_budget` con `exclusions` antes de fallback interno, y **nunca confiar en el claim del delegado** — cierre determinista (exit code, `quorum validate`, `git status`, gates `jq`).

## 4. Definición de DONE para "100% compatible"

Un skill está DONE solo si cumple las 5:

1. **Descubrible:** en una de las 6 rutas + `SKILL.md` mayúsculas + `name/description` presentes.
2. **Parseable:** frontmatter solo con `name/description/license/compatibility/metadata`. Sin `user-invocable` u otros custom (o documentados como ignorados).
3. **Herramientas mapeadas:** cada `Agent/Skill/AskUserQuestion/Bash/Read/...` del SKILL tiene equivalente `task/skill/question/bash/read/...` en OpenCode con permisos `allow` explícitos donde aplique (`skill`, `task`, `question`, `bash`, `read/edit`).
4. **Rutas portables:** sin `~/.claude/...` hardcodeado. Ledger, agentes, config apuntan a `~/.config/opencode/...` o rutas relativas al proyecto.
5. **Verificado en runtime:** `quorum --help` desde OpenCode + un `q-brief`/`q-status` de prueba + (para orquestador) un ciclo `brief→blueprint→dispatch` en scratch sin intervención manual salvo gates humanos.

## 5. Plan sugerido para futuros agentes (no ejecutar sin orden humana)

1. **No tocar los 15 atómicos** salvo quitar/renombrar `user-invocable` o añadir `compatibility: opencode` + `license` si falta. Riesgo bajo.
2. **Crear variante `q-orchestrate-opencode`** (no sobrescribir el original Claude). Mapeo mínimo:
   - `Agent(subagent_type:<rol>)` → `Task` + `agent` definidos en `opencode.json` (recrear grid necesario, no los 20+ de golpe; empezar con 4: especificador/ejecutor/revisor/ejecutor-mecanico).
   - `~/.claude/agents/<rol>.md` → `opencode.json: {agent: {<rol>: {model, prompt, tools}}}`. Revisores con `write:false, edit:false`.
   - `AskUserQuestion x4` → `question` nativa o plugin `askuserquestion`. Documentar cuál se eligió.
   - `~/.claude/quorum-agent-ledger.jsonl` → `~/.config/opencode/quorum-agent-ledger.jsonl` (mismo schema JSON, añadir `host: opencode`).
   - `/model opus` → modelo OpenCode Go equivalente documentado en el skill (leer de `--schema`, nunca hardcodear modelo retirado).
   - `codebase-memory-mcp` → opcional con degradación a `glob/grep/read` si MCP ausente.
3. **Probar en este orden:** `skill load` → `quorum task specify/status` → `complexity-score` → `fleet route` (dry) → `fleet run` en scratch + `git status --porcelain` → ciclo corto real.
4. **No migrar `q-orchestrate` al repo** (respeta su `Constitutional position`). La variante opencode también vive global hasta ADR que diga lo contrario.

## 6. Alternativas (tradeoffs registrados)

- **A. OpenCode como HOST de q-* atómicos + orquestación manual.** Menor riesgo, máximo control. Pierde automatización end-to-end. Recomendado hoy.
- **B. Port `q-orchestrate-opencode`.** Gana automatización, cuesta recrear grid de agentes + question + ledger + pruebas. Solo si el volumen lo justifica.
- **C. `fleet-delegate` / `think-cheap` en OpenCode.** Si el objetivo es solo ahorrar tokens sin pipeline SDC completo, no portar orquestador pesado.

## 7. Referencias rápidas

- Manifiesto: `quorum.md` (gana si discrepa con código).
- Uso: `README.md` → Inicio Rápido + `quorum task ...` + `quorum fleet ...`.
- CLI: `cmd/*.go`, lógica pura `internal/core/*.go` (`fleet_route.go`, `fleet_dispatch.go`, `fleet_catalog.go`, `risk.go`, `feedback.go`).
- Skills repo: `.agents/skills/*/SKILL.md` (canónico embebido vía `embed_agents.go`).
- Skill personal: `~/.agents/skills/q-orchestrate/SKILL.md` + `assets/subagent-prompt-template.md`.
- Fleet data: `.agents/config.yaml`, `.agents/policies/routing.yaml`, `.agents/fleet/agents.yaml`, `.ai/fleet-control.json`.
- OpenCode docs: `/docs/skills` (Place files, Write frontmatter), `/docs/agents` (Task permissions, custom agent), `/docs/tools/` (skill, question, task).
