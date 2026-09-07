# 23 — Capacidad declarada por celda para consumidores externos de `fleet run` (`.agents/policies/model-capability.yaml`, no en `agents.yaml`)

**Fecha:** 2026-09-06
**Origen:** sesión de análisis del skill `~/.claude/skills/think-cheap` (2026-09-06). `think-cheap` y `fleet-delegate` eligen `--agent`/`--model` para `quorum fleet run` con una tabla escrita a mano que se desfasa con cada cambio de catálogo: apagado de codex por kill-switch (2026-07-27), retiro de Gemini 3.5 (2026-09-03, `ideas/13-catalogo-de-modelos-obsoleto-en-config-de-flota.md`, serie general), alta de `opencode_go` con cinco celdas (2026-09-04, FLEET-038). El usuario pidió que "el skill lea `agents.yaml` y según la tarea escoja modelo y esfuerzo", y agregar a `agents.yaml` un dato de "qué tan fuerte es el modelo". Borrador revisado adversarialmente el mismo día (dos revisores; hallazgos incorporados en §2, §4 y §6).
**Estado:** Propuesta — sin implementar. Requiere decisión del humano sobre §8 y una adenda de una frase al ADR 0010.

## Resumen

El dato de capacidad **no puede ir en `agents.yaml`**: la cabecera del archivo, el schema y el ADR 0010 lo prohíben de forma explícita, y dos tests con archivos reales fallarían. Tampoco encaja en `.agents/fleet/`: el ADR define `agents.yaml` como "transporte puro" y trata incluso `quota_class` como "hecho de la cuenta, no juicio"; por extensión, una clase de capacidad, que es un juicio, no pertenece a ese lado. La salida coherente con el ADR es un **archivo de política** (`.agents/policies/model-capability.yaml`) que sólo leen los consumidores externos, con dos tests aditivos: uno que impide que se desfase de `agents.yaml` y otro que impide que el código de Quorum lo lea. Cero cambios en `agents.schema.json`, `core.Route`, `dispatch` ni `catalog`; una adenda de una frase al ADR 0010 que nombre el tercer archivo de política.

Qué resuelve y qué no: hace legible por máquina el trío catálogo + clase + estado de evidencia, con cobertura total del catálogo. Con eso, una celda nueva (hoy, las cinco de `opencode_go`) queda **visible y elegible para tareas LOW** sin editar ningún skill. Lo que la promueve a MEDIUM sigue siendo la campaña de smoke que `docs/fleet-run-for-agents.md` §7.8 deja pendiente. Este archivo no sustituye esa campaña; la hace visible como dato.

---

## 1. Problema

Los skills externos que invocan `quorum fleet run` necesitan, por celda, cuatro datos para escoger: **modo** (one-shot o agéntico), **cuota** (suscripción o api), **esfuerzo** y **capacidad** (barato, estándar, fuerte). `agents.yaml` ya expone los tres primeros:

| Dato | Dónde vive hoy | Legible por máquina |
|---|---|---|
| modo | `transports.<t>.mode` (`agents.yaml`, campo de la idea 21) | sí |
| cuota | `transports.<t>.quota_class` | sí |
| esfuerzo | depende del transporte: codex declara `reasoning_effort` y `effort_whitelist` por modelo; agy, agy_edit y opencode_go no, y el único indicio es el sufijo del nombre canónico (`-none|-low|-medium|-high|-xhigh`); celdas de un solo nivel sin sufijo | sí: campo si existe, si no sufijo, si no un solo nivel |
| capacidad | **en ningún archivo**; tablas a mano en `think-cheap/SKILL.md:131-135` y `fleet-delegate/SKILL.md:116-117`; prosa en `docs/fleet-run-for-agents.md` §4.1/§7.2 | no |

Consecuencia observada: las cinco celdas de `opencode_go` existen desde el 2026-09-04 y ningún skill puede considerarlas, porque no hay forma de saber si `deepseek-v4-pro` es "barato" o "fuerte" sin que un humano edite dos SKILL.md. Cada cambio de catálogo obliga a sincronizar tres lugares.

Un segundo hallazgo, lateral pero relevante para el mismo consumidor: `quorum fleet run` **no consulta** ni `active:` ni el kill-switch. `dispatch` (`cmd/fleet_dispatch.go:109-110`), `smoke` (`cmd/fleet_smoke.go:73`) y `core.Route` (`internal/core/fleet_route.go:282`) honran `active:`; `route` y los comandos `fleet status/enable/disable` leen `.ai/fleet-control.json` (`cmd/fleet_route.go:216-221`, `internal/core/fleet_control.go:16`). `cmd/fleet_run.go` no referencia ninguno de los dos, y `fleet run --schema` lista los modelos de un transporte inactivo (`cmd/fleet_run.go:61-66`). Es un runner "NON-LIFECYCLE" por diseño (`cmd/fleet_run.go:17-22`). Hoy `codex` sigue `active: true` con once modelos (bloque desde `agents.yaml:8`) y está apagado únicamente por `.ai/fleet-control.json` (`"target":"codex"`, 2026-07-27), archivo que además está en `.gitignore:34`. Un skill que lea sólo `agents.yaml` intentará usarlo; un clon limpio ni siquiera ve el kill-switch.

## 2. Por qué el dato NO puede ir en `agents.yaml`, ni en `.agents/fleet/`

La frontera está escrita en tres sitios y protegida por tests:

**Cabecera del archivo** (`.agents/fleet/agents.yaml:1-3`):

```
# Fleet transports (ADR 0010): describes ONLY how to invoke each CLI.
# Never add tier/level/risk/confidence/budget fields here — that policy lives
# exclusively in .agents/config.yaml.levels and .agents/policies/routing.yaml.
```

**Schema** (`.agents/schemas/agents.schema.json:5`, descripción): "additionalProperties:false on every object is the anti-drift guard: a tier/level/risk/confidence/budget field added here fails validation". Los `$defs` `modelSubscription` y `modelApi` cierran el objeto (`:144-146`, `:171-173`); las únicas claves opcionales por modelo son `reasoning_effort`, `effort_whitelist` y, en `modelApi`, `max_cost_per_call_usd`.

**ADR 0010** (`docs/adr/0010-frontera-transporte-politica.md:52-63`): `agents.yaml` "es transporte puro", declara `quota_class` como "hecho de la cuenta, no juicio", y "**nunca** contiene `tier`, `level`, `risk`, `confidence` ni ningún campo de presupuesto — esos campos siguen viviendo exclusivamente en `.agents/config.yaml.levels` y en `.agents/policies/routing.yaml`". La clave de lectura es la distinción hecho/juicio: una clase de capacidad es un juicio humano, así que pertenece al lado política de la frontera. Por eso un archivo hermano dentro de `.agents/fleet/` (la v1 de esta idea) tampoco respeta el ADR: cambia el archivo pero no el lado.

**Qué se rompe si se agrega `tier:` a un modelo de `agents.yaml`:**

| Consumidor | Comportamiento | Evidencia |
|---|---|---|
| `cmd/fleet_dispatch.go` | inerte: `Models map[string]map[string]any`, `yaml.Unmarshal` no estricto | `:41`, `:305-311` |
| `cmd/fleet_route.go` | inerte: sólo lee `provider` por modelo | `:36-46` |
| `internal/core/fleet_control.go` | inerte: sólo `provider` | `:65-70`, `:79` |
| `cmd/fleet_catalog.go` | inerte: compara sólo `model_arg` | `:127-133`, `internal/core/fleet_catalog.go:53-88` |
| `core.RunFleetPreflight` | **falla**: valida el YAML crudo contra el schema | `internal/core/fleet_preflight.go:47-63` |
| `TestFleetRouteAgentsSchemaValidatesRealFile` | **falla** (asserta cero errores "schema" con archivos reales) | `cmd/fleet_route_test.go:472-489` |
| `TestRealAgentsYamlValidatesAgainstSchemaAndRejectsUnknownChannel` | **falla** (`len(result.Errors) != 0`) | `cmd/fleet_dispatch_test.go:836-857` |

Ningún escenario del golden master lee `agents.yaml` (`internal/core/golden_master_test.go` no lo referencia; sus escenarios `fleet-preflight` sólo prueban el rechazo de argumentos posicionales), así que no cuenta como rotura. Es decir: para Go el campo sería invisible, pero el contrato del repo lo rechaza a propósito. Meterlo exige enmendar el ADR 0010 en su tesis central, el schema, el header y dos tests. Ese coste no está justificado cuando el consumidor es externo a Quorum.

## 3. Señales de capacidad que ya existen y por qué no bastan

| Señal | Grano | Naturaleza | Quién la consume | Por qué no sirve al skill externo |
|---|---|---|---|---|
| `.agents/config.yaml.levels` 0-3 | por nivel (cadena primary/fallback/secondary) | declarada, decisión humana | `core.Route` para `phase=implement` | cobertura parcial: de las 11 celdas de `agy`/`agy_edit`, 5 no están en ningún nivel (`3.1-pro-low`, `3.6-flash-low`, `3.6-flash-medium`, `3.7-flash-low`, `gpt-oss-120b`), más las 11 de codex y las 5 de `opencode_go`; el orden dentro de cada nivel codifica "quién paga primero" (free-first, comentarios de `config.yaml` 2026-08-09/26), no fuerza pura; acoplar la exploración externa a las rutas internas de implement viola la separación que el usuario pidió ("no quiero que el CLI decida"). Sí sirve como **verificación de consistencia**: las 6 celdas de suscripción ruteadas (10 en total contando las free de OpenRouter, fuera de la cobertura de este archivo) ya tienen una colocación humana con la que la clase declarada no debe chocar (ver §4.1 guarda 4) |
| `docs/fleet-run-for-agents.md` §4.1 y §7.2/7.3 (Tier A/B, pass@10) | por celda | empírica | humanos | prosa y tablas markdown, no legible por máquina; medida como cota superior de fiabilidad del harness, no de capacidad (§7.1); además incompleta: la campaña agéntica del 2026-08-26 sobre 3.7-flash y 3.1-pro sólo está registrada en comentarios de `config.yaml` y en `fleet-delegate/SKILL.md:117`, no en `docs/` |
| `quota_class` | por transporte | declarada | `dispatch`, idea 22 | eje coste, ortogonal a capacidad |
| `provider` | por modelo | declarada | `core.Route` (diversidad de familia) | eje diversidad, ortogonal |
| `quorum fleet stats --group-by level|cell` | por celda/nivel | telemetría de despachos | humanos | mide implement dentro del ciclo; no cubre `fleet run` |
| `~/.claude/skills/fleet-delegate/ledger.jsonl` | por `task_class` × celda | empírica | los skills | vive fuera del repo; escaso para celdas nuevas (por definición cero filas) |

Nada de esto da, por **clave canónica de modelo**, una clase de capacidad más un estado de evidencia legibles por máquina y con cobertura total del catálogo.

## 4. Propuesta: `.agents/policies/model-capability.yaml`

Un archivo de política descriptiva, junto a `routing.yaml`, que **sólo leen consumidores externos**. Dos ejes separados por modelo, porque son cosas distintas: `class` es el prior humano sobre el modelo (siempre presente) y `evidence` es cuánto se ha medido esa celda (puede ser nada).

```yaml
# Capability class per canonical model key, for EXTERNAL consumers of
# `quorum fleet run` (skills). Policy-side data (ADR 0010 addendum 2026-09):
# core.Route, dispatch, catalog and run never read this file. Keys must exist
# in some transports.<t>.models of .agents/fleet/agents.yaml (guarded by test).
version: 1
models:
  # --- agy / agy_edit (subscription) ---
  google/gemini-3.7-flash-high:   { class: strong,   evidence: { status: smoke,    ref: ".agents/config.yaml" } }   # nivel 3 primario
  google/gemini-3.1-pro-high:     { class: strong,   evidence: { status: campaign, ref: "docs/fleet-run-for-agents.md#4.1" } }   # nivel 3 fallback
  google/gemini-3.6-flash-high:   { class: standard, evidence: { status: smoke,    ref: "ideas/fleet/19-agy-modo-agentico.md" } }   # nivel 2 primario
  google/gemini-3.7-flash-medium: { class: standard, evidence: { status: smoke,    ref: ".agents/config.yaml" } }   # nivel 2 fallback
  anthropic/claude-sonnet-4-6:    { class: strong,   evidence: { status: campaign, ref: "docs/fleet-run-for-agents.md#4.1" } }   # nivel 1 primario
  anthropic/claude-opus-4-6:      { class: strong,   evidence: { status: campaign, ref: "docs/fleet-run-for-agents.md#4.1" } }   # nivel 1 fallback
  google/gemini-3.1-pro-low:      { class: strong,   evidence: { status: campaign, ref: "docs/fleet-run-for-agents.md#4.1" } }   # no ruteado
  google/gemini-3.6-flash-medium: { class: standard, evidence: { status: smoke,    ref: "ideas/fleet/19-agy-modo-agentico.md" } }   # no ruteado
  google/gemini-3.6-flash-low:    { class: cheap,    evidence: { status: smoke,    ref: "ideas/fleet/19-agy-modo-agentico.md" } }   # no ruteado
  google/gemini-3.7-flash-low:    { class: cheap,    evidence: { status: smoke,    ref: ".agents/config.yaml" } }   # no ruteado
  openai/gpt-oss-120b:            { class: cheap,    evidence: { status: campaign, ref: "docs/fleet-run-for-agents.md#4.1" } }   # 5/10, rechazos de sandbox
  # --- opencode_go (subscription, FLEET-038) ---
  opencode-go/deepseek-v4-pro:    { class: strong,   evidence: { status: none, ref: "docs/fleet-run-for-agents.md#7.8" } }
  opencode-go/deepseek-v4-flash:  { class: cheap,    evidence: { status: none, ref: "docs/fleet-run-for-agents.md#7.8" } }
  opencode-go/qwen3.7-plus:       { class: standard, evidence: { status: none, ref: "docs/fleet-run-for-agents.md#7.8" } }
  opencode-go/minimax-m3:         { class: standard, evidence: { status: none, ref: "docs/fleet-run-for-agents.md#7.8" } }
  opencode-go/gpt-5.6-luna:       { class: strong,   evidence: { status: none, ref: "docs/fleet-run-for-agents.md#7.8" } }
```

La semilla es **ilustrativa**: las clases de las celdas ruteadas siguen su nivel en `config.yaml` (nivel 3 → `strong`, nivel 2 → `standard`, nivel 1 → `strong`); las de `opencode_go` son priors de proveedor que el humano confirma o corrige. Las `ref` apuntan a archivos que existen hoy; las tres que citan `ideas/fleet/19-agy-modo-agentico.md` son provisionales, porque el índice marca ese doc como pendiente de archivar a `docs/archive/fleet/` con `git mv` (el test de existencia de ruta obligaría a actualizarlas en esa pasada; mejor aún, el prerrequisito §4.2.3 lleva esa evidencia a `docs/`). Reglas:

- **`class`** con vocabulario cerrado `cheap | standard | strong`. Siempre presente. Es el prior humano; no depende de que exista medición.
- **`evidence.status`** con vocabulario cerrado `none | smoke | campaign`. `none` = declarada sin ninguna prueba (caso `opencode_go` hoy); `smoke` = pass@3 o smoke agéntico puntual; `campaign` = pass@10 medido (§4.1/§7.2). Es lo que el skill usa para decidir cuánto riesgo aceptar en esa celda, separado de la clase.
- **`evidence.ref`** obligatorio: ruta a un archivo del repo (opcionalmente con `#sección`) que registra la prueba. El test verifica que la parte de ruta exista. Sin `ref` no hay entrada válida.
- **Clave = nombre canónico** de `agents.yaml`, mismo string exacto (misma convención que el join con `config.yaml.levels`, ADR 0010 §2).
- **Cobertura obligatoria:** todo modelo de todo transporte `active: true` con `quota_class: subscription` tiene entrada. Así el archivo obliga a decidir cuando entra una celda nueva, que es exactamente el momento en que hoy se olvida actualizar los skills. Prerrequisito: ver §4.2.
- **Dueño:** el humano, en los mismos momentos en que ya toca `config.yaml.levels` (campañas de smoke, altas y bajas de catálogo).
- **Lectores:** skills externos vía Bash (`yq`/python), junto con `agents.yaml` (modo, cuota, catálogo) y `.ai/fleet-control.json` (kill-switch). Quorum no lo lee en runtime.

### 4.1 Guardas dentro de Quorum (aditivas, sin tocar contratos existentes)

Todas en un archivo de test nuevo, `cmd/fleet_model_capability_test.go`, por higiene: el lector del archivo y la guarda por literal (3) quedan juntos y aislados de `cmd/fleet_route_test.go`. Nota sobre el precedente: la prohibición de literales existente (`cmd/fleet_route_test.go:341-355`) recorre **todos** los `.go`, tests incluidos, porque sus nombres prohibidos salen de `config.yaml` y no del código; la guarda (3) difiere en que excluye `*_test.go`, ya que el literal `model-capability` tiene que aparecer en su propio test.

1. **`TestModelCapabilityJoinsAgentsYaml`**: (a) toda clave de `model-capability.yaml` existe en algún `transports.<t>.models` de `agents.yaml` → error (mismo criterio que "drift = error" del ADR 0010 §3); (b) todo modelo de un transporte `active: true` con `quota_class: subscription` tiene entrada → error (recomendado; ver §8.3 para la alternativa "advertencia" que el ADR usa para transportes sin uso). Mismo patrón que `TestFleetRouteRealPolicyFilesG1Cell` (`cmd/fleet_route_test.go:318`) y su helper `fleetRouteRepoRoot` (`:19-23`), que ya leen los archivos reales del repo.
2. **Schema propio** `.agents/schemas/model-capability.schema.json` con `additionalProperties:false`, los dos enums y `ref` obligatorio, validado en el mismo test con `santhosh-tekuri/jsonschema` como hace `RunFleetPreflight`. Además el test comprueba que la parte de ruta de cada `ref` existe en el repo. No se toca `agents.schema.json`.
3. **`TestQuorumNeverReadsModelCapability`**: recorre **todo** `cmd/` e `internal/` (mismo recorrido que la prohibición de literales de `cmd/fleet_route_test.go:341-355`), excluyendo `*_test.go`, y falla si aparece el literal `model-capability`. Hace ejecutable la frase "descriptivo, no política de ruteo". Nota: por eso **no** se propone que `fleet catalog` liste este archivo; sería incompatible con la guarda.
4. **Opcional, advertencia:** consistencia con `config.yaml.levels`: una celda ruteada en nivel 3 con `class: cheap`, o en nivel 0 con `class: strong`, se reporta como advertencia (no error), siguiendo el precedente "transporte sin uso = advertencia" del ADR 0010 §3. Evita que el prior declarado contradiga en silencio la colocación humana ya existente.

**Sobre "ningún flujo lo carga por accidente":** no hay lectura en runtime (ningún `ReadDir`/`Glob` sobre `.agents/`, y `agents.yaml` se referencia por ruta exacta en `cmd/fleet_dispatch.go:295` e `internal/core/fleet_control.go:73`), **pero** el árbol `.agents` completo va embebido en el binario (`embed_agents.go:15`, `//go:embed all:.agents`) y `quorum init` copia `schemas/` y `policies/` a cada proyecto consumidor (`internal/core/task_manager.go:829-835`). Consecuencias: el YAML y su schema viajan juntos a los proyectos que hacen `quorum init` (coherente, a diferencia de la v1 en `.agents/fleet/`, que habría enviado el schema sin el YAML porque `fleet/` no está en el mapa de scaffold); la fuente de verdad es la copia del repo de Quorum, que es desde donde los skills invocan `fleet run` (`cd ~/dev/quorum && quorum fleet run ...`); las copias en consumidores son informativas y pueden desfasarse igual que hoy se desfasa la copia manual de `agents.yaml` en hexcell (`CLAUDE.md`).

### 4.2 Prerrequisitos

1. **Adenda al ADR 0010** (una frase en "Decisión §1" o una sección "Adenda 2026-09"): "`.agents/policies/model-capability.yaml` es un tercer archivo de política, descriptivo, para consumidores externos de `fleet run`; `core.Route`, `dispatch`, `catalog` y `run` no lo leen (guardado por test)". Sin la adenda, el "exclusivamente" del ADR contradice el archivo aunque esté del lado correcto de la frontera.
2. **`codex` a `active: false` en `agents.yaml`.** Hoy sigue `active: true` y sólo el kill-switch (gitignored) lo apaga. Con la cobertura obligatoria de §4, sus once modelos exigirían entrada, y `class` sobre una suscripción retirada no tiene sentido. `active: false` es el equivalente versionado, que `dispatch`, `smoke` y `core.Route` ya honran, y que un clon limpio sí ve. La entrada del kill-switch puede quedarse. La idea 18 ("baja codex") se implementó como kill-switch, no como baja del catálogo; este paso la completa. Alternativa si el humano prefiere no tocarlo: lista de exclusión versionada en el test (§8.3).
3. **Registrar en `docs/fleet-run-for-agents.md` §7.x la campaña 3.7/3.1-pro del 2026-08-26 y el pass@3 de la serie 3.6 del 2026-07-30 (FLEET-19).** Hoy la primera sólo existe en comentarios de `config.yaml` y en un skill externo, y la segunda en `ideas/fleet/19`, pendiente de archivo; la semilla apunta ahí por eso. Con la sección escrita, todas las `ref` pasan a `docs/` y dejan de depender de rutas que se mueven.

### 4.3 Contrato para el consumidor externo (fuera del alcance de Quorum, aquí sólo como referencia)

El skill compone tres lecturas en un solo comando Bash y decide con reglas por atributo, no por nombre:

```
agents.yaml               → celdas: transporte, mode, quota_class, modelo, esfuerzo
.ai/fleet-control.json    → excluir targets apagados (o `quorum fleet status`)
model-capability.yaml     → class + evidence.status por modelo
```

Esfuerzo por celda: `models.<k>.reasoning_effort` si existe (codex); si no, sufijo `-(none|low|medium|high|xhigh)` del nombre canónico (agy, agy_edit); si no, un solo nivel (opencode_go). Reglas del skill (viven en el skill, no en Quorum): summarize/analizar con bundle → `mode: oneshot`; localizar/entender → `mode: agentic`; siempre `quota_class: subscription`, activo y no apagado; complejidad LOW → `class: cheap`, MEDIUM → `class: standard`; esfuerzo un rango arriba del piso si la celda tiene niveles; `evidence.status: none` elegible sólo como único intento externo en tareas LOW self-contained, alimentando el ledger. La preferencia de familia ante empate sigue siendo una línea del skill: es decisión, no dato derivable.

## 5. Alternativas consideradas

| Opción | Veredicto | Razón |
|---|---|---|
| **A. Campo `tier:` en `agents.yaml`** (lo pedido) | rechazada | viola ADR 0010 en su tesis central, header y schema; rompe dos tests reales; exige enmendar un ADR aceptado para servir a un consumidor externo |
| **A'. Archivo hermano en `.agents/fleet/`** (v1 de esta idea) | rechazada tras revisión | cambia el archivo pero no el lado de la frontera: el ADR 0010 §1 define `agents.yaml` como transporte puro y trata hasta `quota_class` como hecho, no juicio, así que un juicio de capacidad no cabe en `fleet/`; además `fleet/` no está en el scaffold de `quorum init` y su schema en `schemas/` sí, con lo que viajarían separados |
| **B. Leer `config.yaml.levels` como proxy de clase** | rechazada como fuente; adoptada como verificación | cobertura parcial (5 de 11 celdas agy sin nivel, más codex y `opencode_go` completos), semántica mezclada (free-first ≠ fuerza), acopla la exploración externa a la política interna de implement. Sí es útil como guarda de consistencia (§4.1.4) |
| **C. Derivar de docs §4.1/§7.2** | absorbida | `evidence` es exactamente su forma legible por máquina; `ref` apunta de vuelta a la sección |
| **D. Ranking dentro de cada skill** | rechazada | es el estado actual; duplica en N skills lo que cambia en un archivo |
| **E. Comando `quorum fleet cells --json` que una los tres archivos** | diferida | útil si aparecen más consumidores; hoy tres lecturas YAML/JSON en Bash bastan. Un comando que *lista* no decide, así que no contradice "el skill decide"; pero exigiría abrir una excepción en la guarda §4.1.3, y eso se decide cuando haga falta, no antes |

## 6. Riesgos y mitigaciones

| Riesgo | Mitigación |
|---|---|
| Deriva de claves entre `model-capability.yaml` y `agents.yaml` | test §4.1.1 con archivos reales, en ambas direcciones |
| Que alguien haga que `core.Route` lea el archivo y se convierta en política de ruteo encubierta | test §4.1.3 por literal sobre todo `cmd/` e `internal/` + header del archivo + adenda al ADR |
| Clases subjetivas | `class` es explícitamente un prior humano; `evidence.status` dice cuánto se ha medido; guarda §4.1.4 avisa si contradice la colocación en niveles |
| `ref` que apunta a nada (hallazgo de la revisión sobre la v1) | el test verifica que la ruta exista; prerrequisito §4.2.3 para que la campaña 3.7 tenga sección en `docs/` |
| Confusión con "Tier A/B" de docs §7.2 | nombre `capability`/`class`, no `tier`; §7.2 es fiabilidad medida del harness y aquí se cita como `evidence`, no se reemplaza |
| El archivo viaja a proyectos consumidores por `quorum init` y se desfasa allí | fuente de verdad = repo de Quorum, desde donde corren los skills; misma situación que la copia manual de `agents.yaml` en hexcell; documentar en el header del archivo |
| `fleet run` sigue ignorando kill-switch y `active` | fuera de alcance; el consumidor lee `.ai/fleet-control.json`. Seguimiento posible: `fleet run` emite *warning* (no bloqueo) si el target está apagado o inactivo |
| Las celdas `opencode_go` siguen limitadas a LOW tras implementar esto | es lo correcto: sin smoke no hay promoción; la campaña de §7.8 es el desbloqueo real y §8.4 pregunta quién la corre |

## 7. Relación con otras ideas

- **`ideas/13-catalogo-de-modelos-obsoleto-en-config-de-flota.md`** (serie general; catálogo obsoleto): misma familia de problema, deriva declarado/vivo. El test §4.1.1 es el análogo para este archivo; `fleet catalog` sigue siendo la fuente para lo vivo.
- **21** (`mode` por transporte): precedente de campo descriptivo añadido con tests sin romper la frontera. Aquí no se toca `agents.yaml` porque el dato pedido es un juicio y cae del lado política.
- **22** (`usd_class` en el ledger): eje coste, derivado de `quota_class`. Complementario: `class` × `usd_class` daría al skill capacidad y coste por celda.
- **18** (baja codex): implementada como kill-switch, no como baja del catálogo; §4.2.2 propone completarla con `active: false`.
- **`ideas/9-enrutamiento-runtime-y-tuning.md`** (serie general; enrutamiento runtime): otra capa; no se solapa.

## 8. Decisiones abiertas (para el brief)

1. Vocabulario de `class`: ¿tres valores bastan? El eje coste queda para la idea 22.
2. Vocabulario de `evidence.status`: `none | smoke | campaign`, o añadir `retired` para celdas históricas que se conservan como registro (hoy no hace falta si codex pasa a `active: false`).
3. Cobertura obligatoria (§4.1.1b): error, como se recomienda, o advertencia siguiendo el precedente del ADR para transportes sin uso. Y para codex: `active: false` (recomendado) o lista de exclusión en el test.
4. ¿Quién y cuándo corre la campaña de smoke que promueve las celdas `opencode_go` desde `evidence.status: none`? §7.8 la deja diferida; sin ella este archivo las deja en LOW.
5. ¿Se implementa el seguimiento "warning en `fleet run` para targets apagados o inactivos", o se deja al consumidor?

## 9. Fuera de alcance

Cambios en `agents.schema.json`, `core.Route`, `dispatch`, `catalog` o en la tesis del ADR 0010 (sólo la adenda de §4.2.1). Cambios en los skills externos (`think-cheap`, `fleet-delegate`): son consumidores y se actualizan aparte una vez exista el archivo. La campaña de smoke de `opencode_go` (§7.8) es una tarea propia.
