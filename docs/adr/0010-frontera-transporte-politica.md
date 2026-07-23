# ADR 0010: Frontera transporte/política para la flota de CLIs delegados

## Estado

Aceptado

## Contexto

La serie `ideas/fleet/*` explora delegar `q-implement` (y, más adelante, otras
fases mecánicas) a CLIs externos de IA (`codex exec`, `agy --print`, `aider`,
`claude -p`) en vez de ejecutarlas siempre dentro de la sesión del orquestador.
`quorum.md:380` rechaza cualquier propuesta que duplique la traducción
nivel→modelo en más de un lugar del sistema. Una v1 de esta idea proponía un
`agents.yaml` con `tiers`, `max_risk` y campos de confianza que competían
directamente con `.agents/config.yaml.levels` — eso es exactamente la
duplicación que el manifiesto prohíbe.

Fase 0a (`docs/archive/fleet/resultados-fase-0a.md`) validó manualmente, con dos
hijas reales de dogfooding (VAL-101 vía `codex exec`, VAL-102 vía
`agy --print`), que un CLI externo invocado headless dentro de un worktree de
Quorum con el contexto de `00`+`01`+`02` puede producir un diff que pasa
`q-verify`. Ese gate G0 fue ratificado por el humano el 2026-07-11. La fase
también dejó evidencia concreta que esta ADR debe fijar como política:

- La flota real (`codex`, `agy`) no alcanza hoy los modelos de
  `config.yaml.levels` (minimax/deepseek/qwen vía API); hace falta un
  mecanismo de reconciliación que no rompa la abstracción de niveles.
- `agy --print`/`-p` es un flag estilo Go, greedy: si no es el último token de
  argv, absorbe el siguiente flag como si fuera el prompt y produce un falso
  éxito (exit 0, diff vacío, respuesta prosaica sobre el flag absorbido).
- `codex` hereda por defecto la config interactiva del usuario
  (`~/.codex/config.toml`, `~/.claude/CLAUDE.md` global) y, sin aislamiento
  explícito, hace escrituras autónomas a estado externo no instruido por la
  tarea (MCP HSME, intento de MCP `linear`).
- La validación de `model_reasoning_effort` de codex tiene dos etapas: un enum
  sintáctico global y una whitelist real por modelo en el backend (gpt-5.5
  acepta `{none, low, medium, high, xhigh}` y rechaza `minimal` pese a ser
  sintaxis válida; no existe `max` como valor global, y el config previo que
  usaba `"max"` era inválido para ese modelo).
- `aider` es de otra especie (editor-CLI configurable por backend litellm, no
  un agente autónomo con modelo fijo) y trae defaults que violan convenciones
  del repo: `--auto-commits` y `--attribute-co-authored-by` activos por
  defecto.

Esta ADR fija la frontera arquitectónica antes de declarar la flota real en
`.agents/fleet/agents.yaml` (tarea hermana FLEET-002-a) y antes de que
`config.yaml.levels` se reconcilie contra modelos alcanzables (FLEET-002-b) o
que exista un preflight en Go que valide el join (FLEET-002-c).

## Decisión

### 1. `agents.yaml` describe CÓMO invocar; `config.yaml` decide QUIÉN ejecuta

`.agents/fleet/agents.yaml` es transporte puro: por cada transporte declara
únicamente binario, plantilla de argv, canal de entrada, formato de salida,
timeouts, flags de sandbox, firmas de fallo, `quota_class` (hecho de la
cuenta, no juicio), el mapa de modelos que expone, la ruta del contract test,
y si el transporte está activo. `agents.yaml` **nunca** contiene `tier`,
`level`, `risk`, `confidence` ni ningún campo de presupuesto — esos campos
siguen viviendo exclusivamente en `.agents/config.yaml.levels` y en
`.agents/policies/routing.yaml`. La abstracción de niveles (0/1/2) permanece
intacta y ningún nombre de modelo se hardcodea en lógica Go de ruteo o
scoring.

### 2. Join por nombre canónico de modelo (`provider/model`)

`config.yaml.levels` referencia modelos por su nombre canónico
(`provider/model`, la convención que `config.yaml` ya usa hoy, p. ej.
`openai/gpt-5.5-medium`). `agents.yaml` expone, por transporte, un mapa de
esos mismos nombres canónicos hacia el string exacto que el CLI subyacente
espera (`model_arg`) más, opcionalmente, el sufijo de esfuerzo de
razonamiento. El join entre ambos archivos es por ese nombre canónico —no hay
un tercer identificador, no hay traducción implícita.

El sufijo de esfuerzo (`-low`, `-medium`, `-high`, ...) es **provisional**:
se valida contra una whitelist por-modelo (`effort_whitelist` en la entrada
del modelo dentro de `agents.yaml`), no contra un enum global único, porque
Fase 0a demostró que la whitelist real vive en el backend del proveedor y
difiere por modelo (gpt-5.5 rechaza `minimal` y no tiene `max`). La política
definitiva de naming de efforts queda diferida a una tarea F7 posterior; esta
ADR sólo fija que la validación es por-modelo y nunca global.

### 3. Preflight ruidoso al arranque

Todo comando `fleet` corre, al inicio, un preflight en Go (implementado en
FLEET-002-c, fuera del alcance de esta declaración) que valida el join
completo:

- **Drift = error.** Un modelo referenciado por `config.yaml.levels` que no
  resuelve a exactamente un transporte declarado en `agents.yaml` es un error
  de arranque ruidoso y accionable: nombra el modelo, el archivo, y qué falta
  (transporte inexistente, modelo ausente del mapa de ese transporte, o
  ambigüedad si resuelve a más de uno).
- **Transporte sin uso = advertencia.** Un transporte declarado en
  `agents.yaml` que ningún nivel de `config.yaml` referencia no es un error;
  es una advertencia informativa. Declarar un transporte no obliga a usarlo.

Esta ADR fija la regla; su implementación mecánica (el preflight en sí, con
tests) es responsabilidad de FLEET-002-c y no se ejecuta como parte de esta
tarea.

### 4. `max_cost_per_call_usd` es exclusivo de `quota_class: api`

El campo `max_cost_per_call_usd` en una entrada de modelo de `agents.yaml`
sólo tiene sentido, y sólo se permite, cuando el transporte que lo declara
tiene `quota_class: api` — es decir, cuando cada llamada consume cuota
facturable por request. Para transportes con `quota_class: subscription`
(codex y agy en v1, ambos de cuenta con cupo fijo, no facturación por
llamada) el campo está prohibido: no aplica, y declararlo sería sugerir un
control de costo que no existe para ese tipo de cuenta. `aider`, declarado en
esta ADR como transporte `quota_class: api` sin adapter implementado
(diferido a `docs/archive/fleet/17-adapter-aider.md`), es el primer consumidor real
de este campo, con un valor provisional a validar contra el diseño del
schema.

### 5. `claude -p` inactivo en v1 — y la frontera aclaratoria con el orquestador L2

`agents.yaml` declara `claude` como transporte con `active: false`. Ningún
nivel de `config.yaml` rutea a él en v1. Esto es deliberado: `claude -p`
comparte cuota con el propio orquestador (el agente Claude Code que ejecuta
`/q-*`), y activarlo como transporte de despacho duplicaría gasto de la misma
cuota que ya consume la sesión del orquestador.

Se deja fijada explícitamente una distinción que de otro modo es ambigua:
la referencia a "Claude frontier" como primario de L2 en
`config.yaml.levels` (`anthropic/claude-opus-4-7`) señala al **orquestador
humano-en-el-loop mismo** — la sesión de Claude Code que decide, revisa y
aplica el gate humano — y NO al transporte inactivo `claude -p` declarado
aquí. L2 no despacha a un CLI externo `claude`; sigue siendo la arquitectura
que ya existe hoy (Constitución regla #7, cost bounded by policy, y regla #6,
merges humanos). FLEET-002-b, al reconciliar `config.yaml.levels`, no debe
rutear ningún nivel al transporte `claude` ni al transporte `aider`.

## Consecuencias

- **Positivo.** `agents.yaml` y `config.yaml` quedan en capas ortogonales:
  cambiar cómo se invoca un CLI (nuevo flag, nuevo binario, nuevo canal de
  entrada) nunca toca política de ruteo, y cambiar qué modelo cubre un nivel
  nunca toca cómo se invoca un transporte. El join por nombre canónico hace
  el acoplamiento explícito y verificable en un solo lugar (el preflight de
  FLEET-002-c), en vez de disperso en lógica Go ad hoc.
- **Positivo.** El preflight ruidoso convierte el drift model↔transporte en
  un fallo de arranque detectable de inmediato, no en un fallo silencioso a
  mitad de un dispatch real.
- **Positivo.** Fijar la frontera "Claude frontier de L2 = orquestador, no
  transporte `claude -p`" en esta ADR evita que una futura tarea (FLEET-002-b
  u otra) rutee accidentalmente un nivel al transporte inactivo y duplique
  gasto de cuota del orquestador.
- **Negativo / diferido.** El preflight en sí (FLEET-002-c), la reconciliación
  de `config.yaml.levels` (FLEET-002-b), el adapter de `aider`
  (`docs/archive/fleet/17-adapter-aider.md`) y la política definitiva de naming de
  efforts (F7) quedan fuera de esta ADR y de esta tarea; esta ADR únicamente
  fija el contrato arquitectónico que esas tareas deben respetar.
- **Negativo.** Mientras el preflight de FLEET-002-c no exista, el
  cumplimiento del join descrito aquí depende de disciplina del implementador
  y de `q-review`, no de un gate automático.
