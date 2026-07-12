# Resultados — Fase 0a: validación manual de delegación (gate G0)

**Tipo:** registro de evidencia (subproducto obligatorio de la tarea 01).
**Ejecutado:** 2026-07-11 (orquestación manual, cero código nuevo en el core).
**Produce:** insumo para la ratificación humana de G0 y ajustes a docs 02–17.
**Veredicto (abajo, §7):** GO — **RATIFICADO por el humano el 2026-07-11** (adenda §9).

---

## 1. Contexto y alcance

Se probó la tesis mínima de G0: un CLI externo, invocado headless dentro de un worktree de Quorum con el contexto de los artefactos `00`+`01`+`02`, produce un diff que pasa `q-verify`. Sin wrappers, sin `agents.yaml`, sin tocar `internal/core`. Se delegaron **dos hijas reales de dogfooding** (test-only, riesgo low), una por CLI:

| Tarea | Objetivo | Complejidad | CLI delegado |
|-------|----------|-------------|--------------|
| VAL-101 | Tests table-driven para `sanitizeYAML` en `cmd/validate.go` (0% → 100% cobertura) | S | `codex exec` |
| VAL-102 | Tests de contrato CLI para `quorum analyze feedback-partition` (stdin/stdout, subproceso) | S/M | `agy --print` |

Ambas recorrieron el ciclo real: `task specify` → `00`/`01`/`02` (validados contra schema) → `task start` (worktree + rama `ai/<ID>`) → delegación externa → `04-implementation-log.yaml` fabricado a mano → `q-verify` → `q-review`. No hubo merge (regla #6): ambas quedan en `active/` a la espera del gate humano.

El inventario de capacidades (§2–4) se levantó por separado (`CLI_INVENTORY_REPORT.txt`, sondas sin ejecución LLM).

---

## 2. Inventario codex (checklist doc 01)

- **Versión:** `codex-cli 0.144.1`. Auth: cuenta ChatGPT (`codex login status`).
- **Entrada de prompt:** posicional o stdin (`echo … | codex exec`, o `codex exec -` para forzar stdin). Si hay stdin piped + posicional, stdin se anexa como bloque `<stdin>`. En el experimento el prompt (5.7 KB) entró por stdin — sin interpolación en shell.
- **cwd:** `-C, --cd <DIR>` fija la raíz del agente. Usado: `-C worktrees/VAL-101`.
- **Sandbox:** `-s, --sandbox {read-only|workspace-write|danger-full-access}`. Usado `workspace-write`. Hallazgo operativo: bajo `workspace-write` la caché por defecto `~/.cache/go-build` es de solo lectura; codex se auto-corrigió prefijando `GOCACHE=/tmp/gocache` tras un fallo `read-only file system`. Confinamiento por escritura correcto.
- **Formato JSONL (`--json`):** eventos por línea. Corrida exitosa (VAL-101, attempt 2): `thread.started`, `turn.started`, luego `item.started×13` + `item.completed×15` (desglose de completados: `command_execution×10`, `mcp_tool_call×3`, `agent_message×2`), y cierre `turn.completed`. El `command_execution` incluye la propia ejecución de `go test` por codex (9 subtests PASS + cobertura 100%).
- **Usage:** reportado en `turn.completed.usage` — `input_tokens:346030, cached_input_tokens:329984, output_tokens:5000, reasoning_output_tokens:2484`. Usage honesto y estructurado, apto para `cli_reported` (doc 06 §8).
- **`--output-schema`:** existe (`-o` escribe el último mensaje a fichero; `--output-schema <FILE>` valida el mensaje final contra JSON Schema). **NO ejercitado** en esta fase — queda como ítem pendiente de inventario para el brief del doc 06 (estructurar notas + señal BLOCKED).
- **Sesiones:** persistidas por defecto; existe `--ephemeral` para no dejar ficheros de sesión. Existen `--ignore-user-config` y `-p/--profile`.
- **Exit codes:** `1` cuando hay `turn.failed`; `2` ante flag desconocido.

**Attempt 1 (fallo ambiental, no de la tesis):** `~/.codex/config.toml` fija el modelo `gpt-5.6-sol`; la API lo rechaza en tier ChatGPT. Resultado: exit 1, 6.8 s, JSONL de 5 líneas (`thread.started`, error de metadata, `turn.started`, `error`, `turn.failed`), sin usage, diff cero.

**Matriz de compatibilidad de modelo (descubierta por sonda):** rechazados en tier ChatGPT `gpt-5.6, gpt-5.6-codex, gpt-5.3-codex, gpt-5.2-codex, gpt-5.1-codex, gpt-5-codex, gpt-5.1, gpt-5` (todos con el mismo error "model not supported when using Codex with a ChatGPT account"). Aceptado **`gpt-5.5`**, pero solo tras sobreescribir `model_reasoning_effort` (el `"max"` del config es inválido para ese modelo) a `high` vía `-c model_reasoning_effort=high`.

---

## 3. Inventario agy (checklist doc 01)

- **Versión:** `1.1.1`. Auth: no se halló comando de login/estado (asumida por cuenta/proyecto).
- **cwd:** **no existe flag `-C`/`--cd`.** Se usa el **cwd del proceso**; hacer `cd` al worktree antes de invocar funciona de forma fiable (diff generado dentro del worktree, cero fuga a la raíz del repo). `--add-dir <DIR>` (repetible) añade directorios al workspace pero **no fue necesario** para fijar el directorio de trabajo. Este dato cierra la decisión abierta del doc 01 (ver §7).
- **Formato de salida:** **no hay modo JSON.** `--print`/`-p` (alias `--prompt`) emite prosa plana. Sin marcadores de progreso estructurados.
- **Usage / tokens:** **ninguno.** agy no reporta usage ni tokens en `--print`. Confirma la telemetría laxa del doc 01 §2: para la celda agy el wrapper solo puede registrar `estimated`/`none`, jamás `cli_reported`.
- **Modelos (`agy models`, nombres exactos para `--model`):** `Gemini 3.5 Flash (Medium/High/Low)`, `Gemini 3.1 Pro (Low/High)`, `Claude Sonnet 4.6 (Thinking)`, `Claude Opus 4.6 (Thinking)`, `GPT-OSS 120B (Medium)`. Usado: **Gemini 3.1 Pro (High)**.
- **Timeout:** `--print-timeout <DURATION>` (default `5m0s`); se fijó 10m, no se disparó (corrida 93 s).
- **Otros flags:** `--sandbox` (restringe acceso a terminal), `--conversation`, `-c/--continue`, `--agent`, `--dangerously-skip-permissions`, `--mode {accept-edits, plan}`. Exit `2` ante flag desconocido.

**TRAMPA CRÍTICA de `--print` (hallazgo de mayor impacto operativo):** `--print`/`-p` es un flag de **string estilo Go y es greedy**. La forma `agy --print --sandbox …` interpreta `print="--sandbox"` como el prompt; el modelo responde conversacionalmente sobre la cadena "--sandbox", termina con **exit 0**, y produce un **falso éxito** (cero diff, respuesta prosaica). Se detectó en una corrida de sanidad: el modelo devolvió un ensayo explicando qué hace `--sandbox` en vez de implementar la tarea. **Forma correcta: todos los flags primero y `--print "<prompt>"` como ÚLTIMO argumento.**

---

## 4. Mini-inventario aider (solo sondeo, sin ejecución LLM)

- **Versión:** `aider 0.86.2`. `OPENROUTER_API_KEY` presente en el entorno; config en `~/.aider.conf.yml`.
- **Headless:** `-m, --message <TXT>` (alias `--msg`/`--prompt`) o `-f, --message-file <FILE>`, más `--yes-always` para auto-responder prompts.
- **cwd/scope:** sin flag `-C`; `--subtree-only` limita al subárbol git; posicional `[FILE …]` acota archivos, no directorio.
- **RIESGO de defaults contra las convenciones de este repo:** `--auto-commits` está **ON por defecto** y `--attribute-co-authored-by` está **ON por defecto** (añade trailer `Co-authored-by`). Ambos violan las reglas del repo (system-commits controlados; sin atribución AI). El adapter (doc 17) DEBE forzar `--no-auto-commits` y desactivar la atribución.
- **Otros:** `--no-git`/`--subtree-only` existen; sin salida JSON; `--dry-run`; exit `2` ante flag inválido.

---

## 5. Resultados por tarea

| Tarea | CLI | Modelo | Duración | Diff | q-verify | q-review |
|-------|-----|--------|----------|------|----------|----------|
| VAL-101 | `codex exec` (0.144.1) | `gpt-5.5` (reasoning `high`) | 115 s (attempt 2) | `cmd/validate_test.go` +97 líneas, in-contract, **sin commit** (worktree untracked, HEAD en baseline `05b68c8`) | **passed** — `TestSanitizeYAML` 9 subtests + `go test ./...` verde | **approve** — AC-1/2/3 no vacuos; cobertura `sanitizeYAML` 100.0% re-verificada de forma independiente; gofmt/vet limpios |
| VAL-102 | `agy --print` (1.1.1) | Gemini 3.1 Pro (High) | 93 s | `cmd/analyze_feedback_partition_test.go` +85 líneas, in-contract, cero fuga a la raíz, **commiteado** (`24751e5`, autor Gary, mensaje convencional, sin atribución AI) | **passed** — subtests `ValidStdin`/`EmptyStdin`/`MalformedJSON` + suite completa (8.82 s) | **approve** — no vacuo; contenido de la partición contrastado vs `internal/core/feedback.go`; strings de error casan con `readStdinJSON`; helpers `buildAnalyzeCLI`/`runAnalyzeCmdErr` espejan el idiom de `cmd/memory_test.go` |

Prompts multi-KB (5.7 KB codex por stdin / 7.2 KB agy por argv) manejados sin problema por ambos CLIs. Ambos respetaron `touch`/`forbid`/`limits`; ninguno commiteó por sí mismo (el commit de VAL-102 lo hizo el operador — ver §6).

---

## 6. Fricciones y hallazgos del ciclo

1. **Fricción al fabricar el `04` (formato YAML).** Un escalar sin comillas con `:` embebido provocó error de parseo en `artifact-save`. Mitigación: entrecomillar strings. Baja severidad, pero real para el fabricador (doc 08).

2. **GAP DE SCHEMA en `implementation-log`.** `entries[]` exige `changed_files` con `minItems:1`. Una delegación **fallida con diff cero** (p. ej. codex attempt 1) **no puede representarse como una entrada válida**: `entries:[]` es ambiguo entre "no se intentó" y "se intentó, falló, sin salida". Relevante para el fabricador (doc 08) y para los gaps de schema de trace señalados en analysis-v2.

3. **Inconsistencia del punto de commit ("commit forense ≠ baseline limpio"), confirmada empíricamente.** Tras fases idénticas (`q-verify`+`q-review`), el diff de VAL-102 quedó **commiteado** (`24751e5`) mientras el de VAL-101 quedó **untracked/sin commit** en su worktree. Los system-commits son constitucionales (regla #6 intacta: no hubo merge); **la INCONSISTENCIA es el hallazgo**. Sin una semántica forense definida por el wrapper, el estado del worktree tras un attempt es no determinista. Exactamente el problema que el doc 06 §4 (snapshot como ref fuera de rama + `reset --hard` al baseline) pretende resolver.

4. **Fuga de config/MCP de codex (efecto colateral externo no instruido).** codex cargó la config global del usuario y, siguiendo el `~/.claude/CLAUDE.md` global (protocolo HSME), **escribió autónomamente en el MCP HSME** (`store_context`, `memory_id 1268`) — los 3 `mcp_tool_call` de la corrida exitosa son de HSME (`search_fuzzy`, `explore_knowledge_graph`, `store_context`). Además intentó arrancar el MCP `linear` (fallo de auth, ruido en stderr: `AuthorizationRequired`). Ninguna de estas escrituras toca git ni el `touch` del contrato, pero son **escrituras autónomas a estado externo sin instrucción en el prompt de la tarea**. El adapter debe aislar la config (ver §8).

5. **Inconsistencia interna de evidencia en VAL-102 (documentación stale).** El `04-implementation-log.yaml` de VAL-102 (fabricado 16:37) afirma `verify_pending: true` y que "q-verify no se corrió"; el `05-validation.json` posterior (16:39) demuestra que **sí se corrió y pasó**. El `q-review` lo marcó como inconsistencia de documentación (no bloqueante; `05-validation.json` es el artefacto de registro). Refuerza el punto 2/3: el `04` fabricado a mano se desincroniza fácilmente del estado real.

6. **Sub-attempts muertos por timeout del orquestador (error de operador, no del CLI).** Dos sub-attempts previos a la corrida exitosa de codex fueron matados por el timeout de 2 min de la herramienta del orquestador (worktree reseteado cada vez). No es fallo de codex ni del modelo; es un recordatorio de que el wrapper (doc 06 §1) necesita timeout real con kill del process group y precondición de worktree limpio.

---

## 7. Veredicto: **GO** (recomendación, pendiente de ratificación humana)

La tesis mínima de G0 se sostiene. Ambos criterios de aceptación del doc 01 están cumplidos:

- **[cumplido]** Al menos una hija implementada por **codex** pasa `q-verify` con orquestación manual: VAL-101, `q-verify` passed + `q-review` approve, cobertura 100% re-verificada de forma independiente.
- **[cumplido]** **agy** ejecutado headless sobre un worktree con resultado evaluado: VAL-102, `q-verify` passed + `q-review` approve; el dato importa y fue positivo.
- **[cumplido]** Este archivo existe con inventario completo (§2–4) y veredicto GO explícito.
- **[cumplido]** Lista de sorpresas que obligan a ajustar docs 02–15/17 (§8, no vacía).

**Decisión abierta del doc 01 resuelta:** *"si agy no permite fijar cwd de forma fiable, ¿queda en la flota v1 o se pospone su adapter?"* → **agy queda en la flota v1.** Fija cwd de forma fiable vía el cwd del proceso (`cd` al worktree), sin necesidad de `-C` ni `--add-dir`, y produjo un resultado competente que pasó verify+review. Su adapter (doc 07) procede.

El GO es una recomendación técnica; la ratificación (o veto) de G0 es del humano. **Ratificado el 2026-07-11** — ver adenda §9.

---

## 8. Sorpresas que obligan a ajustar docs 02–17

1. **Trampa de `--print` greedy en agy + falso éxito (exit 0, cero diff, prosa).** Obliga a: (a) invocación con `--print "<prompt>"` como último argumento, y (b) **detección de no-op / falso éxito** en el wrapper (exit 0 + diff vacío + notas vacías ⇒ `noop: true`, ya previsto en doc 06 §5 pero ahora con caso real). → **docs 06 y 07**.

2. **Aislamiento de config de codex.** El adapter debe: usar `-m <model>` explícito (nunca heredar el modelo del `config.toml` interactivo del usuario), fijar `model_reasoning_effort` explícito, valorar `--ephemeral` y `--ignore-user-config` para evitar la fuga de MCP/`CLAUDE.md` global (escrituras autónomas a HSME/linear no instruidas). → **docs 02 y 06**.

3. **Usage ausente en agy (telemetría laxa confirmada).** Para la celda agy no hay `cli_reported`; el schema/telemetría debe aceptar `estimated`/`none` sin degradar el registro. → **doc 02** (y coherente con doc 01 §2).

4. **Gap de schema en `implementation-log` (`changed_files` minItems:1).** Una delegación fallida con diff cero no es representable; el fabricador necesita una forma de registrar "intentado, sin salida". → **doc 08** (y schema de trace).

5. **Semántica de commit forense no determinista.** El estado del worktree tras un attempt (commiteado vs untracked) debe definirlo el wrapper por construcción (ref fuera de rama + `reset --hard` al baseline). → **docs 03 y 06**.

6. **Defaults de aider contra las convenciones del repo.** El adapter debe forzar `--no-auto-commits` y desactivar `--attribute-co-authored-by`. → **doc 17**.

7. **Matriz reasoning-effort / compatibilidad de modelo por tier de auth.** En tier ChatGPT solo `gpt-5.5` fue aceptado, y solo con `model_reasoning_effort=high` (el `"max"` del config es inválido para ese modelo). El adapter/`agents.yaml` necesita una matriz explícita de `{modelo, reasoning_effort, tier}` validada, no valores fijos heredados. → **doc 06** (y `agents.yaml` del doc 02). Correlación completa de efforts en §9.

---

## 9. Adenda post-ratificación (2026-07-11)

**G0 RATIFICADO por el humano.** Decisiones registradas:

1. **GO ratificado** — la serie avanza a **F2** (ADR-A frontera transporte/política, ADR-B attempt/reroute/BLOCKED).
2. **VAL-101 y VAL-102 aceptadas y mergeadas a `main`** por decisión humana (`q-accept`: ready ambas; en VAL-102 se corrigió factualmente el `verify_pending` stale del `04` — hallazgo §6.5).
3. **Corrección humana al §2/§8.7:** `gpt-5.6-sol` NO es un pin obsoleto — es un modelo **temporalmente no disponible**; el pin del config es intencional. La conclusión operativa del §8.7 sigue vigente: el adapter jamás hereda modelo/effort del config interactivo del usuario.

**Correlación reasoning-effort codex ↔ Claude Code (sondeo empírico, gpt-5.5, prompt trivial idéntico por valor):**

La validación de `model_reasoning_effort` en codex tiene **dos etapas**: (1) enum global sintáctico `{none, minimal, low, medium, high, xhigh}` — no existen `max` ni `ultra` como valores globales; (2) **whitelist por modelo** en el backend — gpt-5.5 acepta `{none, low, medium, high, xhigh}` y rechaza `minimal` pese a ser sintaxis válida.

| Claude Code | codex (gpt-5.5) | Nota |
|---|---|---|
| low | low | nombre idéntico |
| medium | medium | nombre idéntico |
| high | high | idéntico; primer tier con reasoning tokens > 0 |
| xhigh | xhigh | idéntico; tier máximo alcanzable en gpt-5.5 |
| max | **GAP** | codex no tiene `max` → **clampear a `xhigh`** (el `"max"` del config era exactamente este bug) |
| **GAP** | none, minimal | sub-piso sin equivalente en Claude Code; `minimal` además rechazado por la whitelist de gpt-5.5 |

Evidencia de ordenamiento (reasoning tokens / latencia con el mismo prompt): `none`≈`low`≈`medium` (0 tokens, efecto piso) < `high` (10–11) < `xhigh` (13; 8.7 s vs ~4 s). **Implicación doc 06 / `agents.yaml` (doc 02):** el adapter traduce el effort por transporte (clamp `max`→`xhigh`) y valida contra la whitelist **por modelo**, nunca contra el enum global.
