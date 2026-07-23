# 02 — ADR-A: frontera transporte/política + `agents.yaml` + preflight del join

> **ARCHIVADO 2026-07-23 — IMPLEMENTADO.** docs/adr/0010-frontera-transporte-politica.md + .agents/fleet/agents.yaml.

**Tipo:** ADR + schema + código de preflight.
**Depende de:** 01 (gate G0; el inventario alimenta el agents.yaml real).
**Riesgo sugerido:** medium (toca política y convenciones del core; cambio conceptual, diff pequeño).

## Contexto

`quorum.md:380` rechaza cualquier propuesta que duplique la traducción nivel→modelo. La v1 de la idea violaba esto (un `agents.yaml` con tiers y `max_risk` competía con `config.yaml`). La resolución de la v2: **`agents.yaml` describe cómo invocar; `config.yaml` decide quién ejecuta**. Además, la flota real (codex + agy) no alcanza los modelos L0 actuales de `config.yaml` (minimax/deepseek/qwen vía API): hay que reconciliar la política con la realidad sin romper la abstracción de niveles.

## Objetivo

1. **ADR-A** en `docs/adr/`: frontera transporte/política, join por nombre de modelo, drift = error ruidoso de arranque.
2. **`agents.yaml`** (transporte puro) con la flota verificada.
3. **Reconciliación de `config.yaml`**: niveles apuntando a modelos alcanzables.
4. **Preflight** en Go: valida el join al inicio de cualquier comando `fleet`.

## Diseño propuesto (el blueprint lo refina)

`agents.yaml` (ubicación propuesta: `.agents/fleet/agents.yaml`) contiene SOLO: binario, plantilla argv, vía de entrada (`stdin`|`prompt_file`), formato de salida, timeouts default, flags de sandbox, firmas de fallo (quota/auth), `quota_class` (`subscription`|`api` — hecho de la cuenta, no juicio), modelos que expone (nombre canónico → string que el CLI espera), ruta del contract test. **Prohibido**: tiers, niveles, riesgo, confianza, presupuestos.

Flota v1 según inventario: `codex` (`exec --json -C {worktree} --sandbox workspace-write -o {out}`, stdin) y `agy` (`--print --model {model} --sandbox`, según lo que la Fase 0a confirme de cwd/salida). `claude` (`-p --model {model} --output-format json`, stdin) se declara como transporte **[Opcional]** — ya está instalado y comparte cuota con el orquestador (punto ciego aceptado por decisión 2 del índice).

Preflight: todo modelo referido por `config.yaml.levels` debe resolver a exactamente un transporte. Falla con mensaje accionable (qué modelo, qué archivo, qué falta). Sin transporte alcanzable → el nivel queda inválido y se dice fuerte.

## Reconciliación de `config.yaml` (decisión de política, va dentro del ADR)

Propuesta inicial — **a ratificar en el brief, con el dato de Fase 0a**:

- L0 (mecánico/barato): `gemini-3.5-flash-low` o `gpt-oss-120b` (vía agy).
- L1 (review/correcciones): `gpt-5-medium` (codex) / `gemini-3.1-pro-low` (agy).
- L2 (arquitectura/alto riesgo): se mantiene Claude frontier (el orquestador mismo) / `gpt-5-high`.
- `max_cost_per_call_usd` queda solo con sentido para `quota_class: api` (hoy: ninguno); para suscripciones no aplica y el ADR lo documenta.

## Criterios de aceptación

- [ ] ADR-A aceptado en `docs/adr/` (numeración siguiente disponible).
- [ ] `agents.yaml` validado por schema propio (`.agents/schemas/` o embebido) con codex y agy reales.
- [ ] `config.yaml.levels` solo referencia modelos alcanzables; la abstracción de niveles intacta (cero nombres de modelo en lógica Go).
- [ ] Preflight con tests: join válido pasa; modelo sin transporte falla ruidoso; transporte sin uso no es error (solo advertencia).
- [ ] `go test ./...` verde con `CGO_ENABLED=0`.

## Decisiones abiertas para el brief

- Ubicación final de `agents.yaml` (`.agents/fleet/` vs `.agents/policies/`) — transporte no es política, de ahí la propuesta de carpeta propia.
- ¿`claude` entra como transporte desde v1 o se difiere?
- Los nombres canónicos de modelo (¿se adopta `provider/model` como ya usa `config.yaml`?).
- Si esta tarea resulta grande, `/q-decompose`: (a) ADR+schema, (b) reconciliación config, (c) preflight Go.
- ¿Se añade aider como tercer transporte (quota_class: api)? Sería el primer consumidor real de max_cost_per_call_usd, hoy documentado sin instancia — valida el diseño del schema contra un caso concreto. Ver 17-adapter-aider.md.
