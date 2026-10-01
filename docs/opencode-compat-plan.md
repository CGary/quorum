# Plan: 100% compatibilidad Quorum ↔ OpenCode (válido + ejecutable)

> Fecha: 2026-09-16. Alcance deliberadamente MÍNIMO. No cubre todas las casuísticas. Las ambigüedades van en §5 sin solución inventada.
> Verificado en: `.agents/skills/*/SKILL.md` (15), `~/.agents/skills/q-orchestrate/SKILL.md` v1.8 + assets, `quorum.md` v1.1, `embed_agents.go`, `.agents/fleet/agents.yaml`, docs oficiales `opencode.ai/docs/skills|agents|tools`.

## 0. Principio rector (filosofía Quorum, no negociable)

El plan preserva, no adapta a conveniencia:

1. **Skills fase-única.** Cada `/q-*` ejecuta UNA fase y para. Solo 3 auto-transiciones forward existen (`q-brief→blueprint`, `q-decompose→split`, `q-blueprint→start`). `quorum task back` siempre humano. Ningún port a OpenCode puede auto-encadenar skills ni auto-llamar `back`.
2. **Orquestador delgado + workers efímeros.** El main window guarda veredictos y estado, NUNCA código/diffs/dumps. Un subagente = una fase, devuelve un párrafo.
3. **Contrato es autoridad.** `02-contract.yaml` (`touch/forbid/verify.commands/límites`) manda. Validación es finalidad (exit 0). Sistema commitea, humano mergea.
4. **Costo acotado por política.** Ruteo vía `quorum fleet route` (datos en `config.yaml+routing.yaml+agents.yaml`). El orquestador NUNCA elige modelo/agente a mano. Agotar `reroute_budget` con `exclusions` antes de fallback interno.
5. **Claim del delegado nunca es evidencia.** Todo externo cierra con chequeo determinista propio (exit code, `quorum validate`, `git status --porcelain`, gates `jq`).

Si un cambio viola 1-5, no es "port", es bug contra la constitución.

## 1. Hallazgo base (lo que hace el plan barato)

Los 15 skills del repo **NO usan** `AskUserQuestion` ni `Agent(subagent_type)` ni `~/.claude/agents/`. Verificado por grep 2026-09-16: 0 matches en `.agents/skills/*/SKILL.md` para esos tres patrones.

Usan: texto plano en español + `ESPERANDO RESPUESTA DEL USUARIO...` como última línea solo en turnos que preguntan/bloquean/deciden; `quorum` CLI + `bash` + `git`; `hsme-cli ... --project quorum --json --no-input` con `timeout 20` y degradación graceful ya escrita (si falta/timeout → seguir con nota de una línea).

Excepciones reales (las únicas que piden código):

- `q-review` línea 101: fallback `~/.claude/skills/think-cheap/scripts/rung0-cells.py --mode agentic --class standard`. Ruta Claude-global hardcodeada. En OpenCode esa ruta no existe.
- `q-brief/q-blueprint/q-memory/q-session`: hook HSME con `SQLITE_DB_PATH="<hsme-db-path>"` + `--project quorum`. El placeholder `<hsme-db-path>` y el slug `quorum` por defecto asumen proyecto quorum; en consumidor el slug y path cambian (ver §5).
- `q-dispatch` v2: auto-dispatch sin confirmación salvo codex (hoy muerto: codex `active:false`). Asume que el humano puede interrumpir con veto ("elegir otro/cancelar") mid-run. Semántica de interrupción en OpenCode no verificada (ver §5).

`q-orchestrate` (global, fuera del repo) SÍ usa todo lo incompatible: `Agent(subagent_type:<rol>)`, `~/.claude/agents/<rol>.md` + grid `<role>-<model>-<effort>`, `AskUserQuestion x4`, `/model opus/fable`, ledger `~/.claude/quorum-agent-ledger.jsonl`, `codebase-memory-mcp cli index_*`. Por eso va en fase separada como skill nuevo, nunca overwrite.

## 2. Fase 1 — Hacer VÁLIDOS los 15 (carga en OpenCode)

Cambios mínimos, riesgo bajo. Nada de lógica.

1.1. Frontmatter: en cada `.agents/skills/*/SKILL.md`, mantener `name` + `description`; **quitar `user-invocable: true`** (desconocido para OpenCode, hoy ignorado pero es ruido) y añadir `compatibility: opencode` + `license` donde falte. No tocar el cuerpo.
1.2. `q-review` § pre-lectura externa: sustituir la línea 101 por resolución portable: "si `quorum fleet route` phase=review no resuelve, OMITIR la pre-lectura externa y seguir con revisión interna; no referenciar `~/.claude/...`". No inventar ruta opencode para `rung0-cells.py` (ver §5.4).
1.3. Regenerar espejo `.claude/skills/` desde `.agents/skills/` (hoy árbol copiado) o documentar cuál es canónico para `quorum init` + `embed_agents.go` (`go:embed all:.agents` manda: canónico es `.agents/`).
1.4. Verificación: `SKILL.md` en mayúsculas, carpeta por skill, nombres únicos, `name/description` presentes. Criterio: aparecen en lista de skills de OpenCode en proyecto quorum limpio.

## 3. Fase 2 — Hacer EJECUTABLES los 15 (runtime en OpenCode)

Sin cambiar filosofía, solo permisos y precondiciones.

2.1. `opencode.json` mínimo del proyecto consumidor (documentarlo en `README.md` o template `quorum init`, no hardcodear en skills):
```json
{ "$schema": "https://opencode.ai/config.json",
  "permission": { "bash": "ask", "skill": "allow", "question": "allow", "task": "allow" } }
```
`bash: ask` porque los skills ejecutan `quorum/git/timeout/hsme-cli`; `skill/question/task: allow` porque el flujo los necesita sin fricción. Ajuste fino por glob queda fuera de este plan.
2.2. Precondiciones runtime (el skill debe fallar con mensaje claro si faltan, no improvisar):
- `quorum` en `PATH` (probar `quorum --help`).
- `quorum init` corrido (existen `.agents/schemas/`, `.agents/policies/`, `.ai/tasks/`).
- Repo Git (existe `.git`; `git rev-parse` OK para worktrees).
- `hsme-cli` OPCIONAL (los 4 skills con hook ya degradan; no hacerlo obligatorio).
2.3. Preguntas al humano: mantener protocolo actual (texto + `ESPERANDO...`). NO migrar a `question` tool nativa en esta fase. `question: allow` queda habilitado para futuro, pero los skills no lo exigen hoy. Evita reescribir 13 handoffs.
2.4. `q-dispatch` en OpenCode: mantener v2 (mostrar decisión y auto-despachar). Documentar como ambigüedad (§5.2) que el veto por interrupción puede no existir igual que en Claude; mientras tanto el veto se expresa como mensaje siguiente ("elegir otro/cancelar"), no como interrupción mid-dispatch.
2.5. Verificación: matriz mínima, no exhaustiva: `q-status` (solo lectura) → `q-brief` en inbox de prueba → `q-blueprint` (crea worktree) → `quorum task back` humano (limpia). Si esos 3 corren, el resto es mismo sustrato (bash+quorum+git).

## 4. Fase 3 — Orquestación con q-orchestrate (skill NUEVO, no port in-place)

Respetando su posición constitucional (global, no migrar al repo sin ADR):

3.1. Crear `~/.config/opencode/skills/q-orchestrate-opencode/SKILL.md` (global OpenCode-compat). No tocar `~/.agents/skills/q-orchestrate/`. Reutilizar Fases 1-2 como workers: el orquestador OpenCode despacha `/q-brief`, `/q-blueprint`, `/q-analyze`, `/q-implement` o `/q-dispatch`, `/q-verify`, `/q-review`, `/q-accept`, `/q-memory` vía `skill` + `task`, uno por fase.
3.2. Mapeo mínimo de workers (empezar con 4, no el grid de 20+):
- `especificador` (brief/blueprint default) → `agent` opencode con modelo Sonnet-equivalente disponible en OpenCode Go + `edit: deny` donde sea read-only.
- `ejecutor` (implement fallback interno) → agente con `bash/edit` permitidos, acotado al worktree vía `external_directory`.
- `revisor` (review, nunca más débil que implementador) → `write:false, edit:false`.
- `ejecutor-mecanico` (fallback de fases mecánicas) → modelo barato, mismo acote.
Definirlos en `opencode.json` → `agent: {...}`, NO en `~/.claude/agents/`. Documentar modelo exacto leído de `quorum fleet run --agent opencode_go --schema` el día de la prueba; nunca hardcodear nombres del ladder 2026-09-13 en el SKILL (ese ladder es informativo, `/q-dispatch` resuelve el vivo).
3.3. Step 0 (desambiguación): mantener las 4 preguntas por texto + `ESPERANDO...`. Solo si `question` tool demuestra ser estable en el proyecto, migrar después. No bloquear Fase 3 en eso.
3.4. Fleet Gate (implement): copiar lógica v1.8 tal cual pero ejecutada en main window OpenCode: producir `risk` (de `00-spec`) + `band` (`quorum analyze complexity-score`) → `/q-dispatch <ID>` → mostrar candidato + `quorum fleet stats --json` (éxito observado) → auto-dispatch. Solo 3 caminos legales a implement interno (router `blocked/no_viable_candidate` literal; veto humano literal; presupuesto `reroute_budget` agotado con conteo). `gate_reason` obligatorio en ledger.
3.5. Fases mecánicas (analyze/verify/accept/memory): default externo vía `quorum fleet run --agent opencode_go --model <de --schema> --cwd <scratch> --input <bundle> --no-input --json` + chequeo determinista propio (tabla Step 1.6 original). Un intento externo por fase; al fallar → `ejecutor-mecanico` interno con output como contexto. Nunca reintentar misma celda externa. `ready` siempre veredicto del orquestador vía `jq` sobre `05/06/07`, nunca del delegado. `quorum memory save` solo tras mostrar entries al humano.
3.6. Ledger: misma línea JSON, nueva ruta `~/.config/opencode/quorum-agent-ledger.jsonl` + campo `host: "opencode"`. Sin ledger no hay retune (regla: ~20-30 fases antes de cambiar defaults).
3.7. Idioma: mantener regla original (hilo humano en español, prompts orquestador→subagente + bundles en inglés desde template verbatim). No re-autorar template en español.
3.8. Verificación: un ciclo corto real S/M-band low-risk en scratch: `specify→brief→blueprint→dispatch(externo)→verify→review→accept→memory→STOP` con merge humano manual. Si Fleet Gate nunca se evalúa o implement va interno sin evidencia, es violación de protocolo, no "optimización".

## 5. Ambigüedades explícitas (sin solución inventada)

5.1. **Slug y DB de HSME en consumidor.** Los skills dicen `--project quorum` y `SQLITE_DB_PATH="<hsme-db-path>"`. En un proyecto consumidor el slug no es `quorum` y el path real lo pone el entorno HSME del usuario. No se propone default mágico; cada skill debe documentar que esos dos valores los provee el humano/entorno y que la ausencia degrada a seguir sin contexto (comportamiento ya escrito).
5.2. **Veto por interrupción en `q-dispatch`.** El SKILL asume que el humano puede interrumpir mid-dispatch ("elegir otro/cancelar"). No verificado si OpenCode permite interrumpir igual que Claude ni con qué UX. Plan asume veto como mensaje siguiente; si OpenCode no lo soporta así, el gate codex/veto queda degradado y debe anotarse como limitación, no parchearse con polling inventado.
5.3. **Paralelismo y límites de `task` en OpenCode.** `q-orchestrate` asume un subagente por fase secuencial con carry-forward acumulativo. No verificado cuántos `task` concurrentes permite OpenCode ni timeouts máximos por defecto. Plan no propone fases en paralelo; todo secuencial. Si el timeout por defecto es <600s (necesario por celda lenta ej. nivel 3), debe configurarse explícito o la fase fallará por infraestructura, no por modelo.
5.4. **`rung0-cells.py` en OpenCode.** `q-review` lo referencia bajo `~/.claude/skills/think-cheap/...`. No existe equivalente verificado en `~/.config/opencode/...`. Plan lo omite (revisión interna pura). Proponer una ruta sin verificar sería solución imposible en la práctica.
5.5. **Modelos y precios.** El SKILL original nombra `opus/sonnet/haiku/fable` + esfuerzos y precios $/MTok a 2026-08-31. Ese catálogo es Anthropic/Claude y churnea; en OpenCode Go los modelos se leen de `--schema`. Plan prohíbe hardcodear reemplazos 1:1 sin evidencia de ledger propio en OpenCode.
5.6. **`codebase-memory-mcp` en OpenCode.** No verificado que ese MCP esté instalado/configurado en el entorno OpenCode del usuario. Plan lo trata como opcional con fallback a `glob/grep/read`. No se propone auto-indexar desde el skill.
5.7. **`question` vs texto.** No verificado que `question` tool renderice igual en TUI/headless/agentes anidados en la versión del usuario. Por eso Fase 2 mantiene texto + `ESPERANDO...`. Migrar a `question` sin esa prueba sería frágil.

## 6. Lo que este plan NO hará (anti-scope)

- No auto-encadenar skills, no auto-`back`, no auto-merge, no nuevos artefactos numerados, no renegociación automática de contrato.
- No reescribir los 15 cuerpos a `question` tool ni al grid completo de roles.
- No fijar modelos por nombre en ningún SKILL (leer de `--schema` / `fleet route` el día de la ejecución).
- No prometer cobertura de headless/CI, monorepos gigantes, ni BDD lento como `verify.commands` (el humano corre BDD aparte por política de testing).

## 7. Criterio DONE (verificable, no exhaustivo)

1. Los 15 cargan en OpenCode (lista visible) con frontmatter §2.
2. `q-status` + `q-brief` + `q-blueprint` corren en proyecto de prueba con `opencode.json` §3 y worktree creado/limpiado vía `back` humano.
3. `q-dispatch` muestra candidato de `fleet route` y despacha en `opencode_go` (o cae a interno con una de las 3 evidencias legales).
4. `q-orchestrate-opencode` completa un ciclo S/M low-risk hasta `accept: ready` + `memory` mostrado + STOP con bloque merge humano impreso, con ledger con `executor/gate_reason` en cada fase externa potencial.
5. `docs/opencode-skills-compat.md` actualizado con lo que se aprendió (rutas reales, modelos reales usados, timeouts reales).
