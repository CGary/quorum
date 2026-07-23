# Análisis de factibilidad v2: sistema de enrutamiento de tareas entre múltiples LLMs

> **ARCHIVADO 2026-07-23 — IMPLEMENTADO.** Superado — materializado por la serie ideas/fleet/00-17 y los ADR 0010-0012.

**Fecha:** 2026-06-11  
**Documento analizado:** `ideas/0-sistema-inteligente-enrutamiento-tareas-entre-múltiples-llms.md`  
**Tipo:** análisis pre-ADR, sin implementación  
**Veredicto corto:** la v2 es mucho más sólida que la v1; ya resuelve varios riesgos arquitectónicos importantes, pero todavía necesita recortar ambigüedades antes de convertirse en plan implementable.

---

## 1. Resumen ejecutivo

La idea es **factible** como evolución de Quorum, pero sigue siendo una iniciativa de **alto impacto arquitectónico**. La versión nueva mejora mucho porque:

- separa transporte (`agents.yaml`) de política (`config.yaml` / `routing.yaml`);
- prohíbe interpolar prompts en shell;
- trata el ledger como índice reconstruible, no como verdad;
- introduce `data_classification` separada de `risk`;
- define `attempt` vs `reroute`;
- mueve el cumplimiento de artefactos al wrapper;
- reconoce que los cambios constitucionales deben ir por ADR.

El principal riesgo ya no es conceptual sino de **frontera y semántica de ejecución**:

1. El core aún no tiene `task run` ni dispatcher; incluso hay test que confirma que `task run` no existe.
2. `07-trace.json` soporta eventos libres, pero `attempts[]` es estrecho y su append-only actual solo protege `attempts`, no `events`.
3. La semántica de “commit forense” es peligrosa: un commit deja el worktree limpio, pero no necesariamente vuelve al baseline limpio para el siguiente modelo.
4. `data_classification` no existe en `spec.schema.json`; agregarlo implica cambio de schema, skills y templates.
5. “Regla nueva candidata #10” choca con la Regla #10 actual de soberanía de datos; debería ser #11 o una sección de gobernanza, no reemplazarla.

**Recomendación:** avanzar, pero no directo a implementación core. Primero convertir la idea en un set de ADRs pequeños y una Fase 0 de medición con un runtime externo mínimo.

---

## 2. Estado real del sistema

### 2.1 Lo que ya existe y favorece la idea

- `README.md` declara como prioridad #1: implementar dispatcher automático de ejecución.
- `quorum.md` ya establece que routing, retries y escalaciones pertenecen al dispatcher, no al agente.
- `.agents/config.yaml` ya define niveles 0/1/2 y modelos primario/fallback.
- `.agents/policies/routing.yaml` ya mapea riesgo a `executor_level`, `reviewer_required` y `max_attempts`.
- `.agents/policies/risk.yaml` ya contiene señales de riesgo y `sensitive_paths`.
- `internal/core/risk.go` ya implementa scoring determinístico por señales.
- `07-trace.json` ya tiene `attempts[]` con `model`, `tokens_in`, `tokens_out`, `cost_usd`, `duration_s` y `notes`.
- `trace.schema.json` ya permite `events[]` con objetos libres.
- `quorum task artifact-save` valida artefactos antes de escribir.
- `EnsureTraceAppendOnly()` protege `07-trace.json.attempts[]` contra borrado/reordenamiento/mutación.

### 2.2 Lo que no existe todavía

- No hay `quorum task run`.
- No hay dispatcher automático en el core.
- No hay router de modelos.
- No hay ledger de presupuestos.
- No hay `agents.yaml`.
- No hay `data_classification` en `00-spec.yaml`.
- No hay `provider_trust` ni presupuestos en `config.yaml`.
- No hay `complexity-score`.
- No hay enforcement determinístico centralizado de `touch`/`forbid` fuera de las skills/review.

Dato relevante: `internal/core/task_manager_test.go` incluye un test que falla si `task run` aparece como subcomando, lo que confirma que la ausencia de runtime automático es intencional en el estado actual.

---

## 3. Puntos fuertes de la v2

### 3.1 Separación transporte/política

La frontera `agents.yaml` = transporte y `config.yaml` = política es correcta. Evita duplicar listas de modelos y respeta la regla de Quorum: los niveles son estables, los nombres de modelos cambian.

**Mantener.**

### 3.2 Router determinista

La idea de que Claude sugiera pero el router determinístico decida está alineada con la Regla #7.

**Mantener.**

### 3.3 Context bundler

Nombrar el bundler como componente propio es un avance. Es la pieza que encarna la Regla #2: contexto determinista.

**Mantener, pero exigir hash del bundle + policy hash en trace.**

### 3.4 Wrapper como frontera de compliance

Mover la fabricación de `04-implementation-log.yaml` al wrapper es una mejora grande. Reduce fallos por idioma, formato y schema.

**Mantener.**

### 3.5 `data_classification` ortogonal a `risk`

Este es probablemente el aporte más importante de la v2. `risk` mide impacto del cambio; no mide sensibilidad del dato.

**Mantener, pero requiere ADR + schema.**

### 3.6 Attempt vs reroute

Separar trabajo real (`attempt`) de fallo de infraestructura (`reroute`) es correcto. Evita quemar intentos contractuales por 429 o wrapper roto.

**Mantener, pero ajustar cómo se registra en trace.**

---

## 4. Gaps y riesgos técnicos

### 4.1 `07-trace.json` no puede registrar todo lo que la v2 quiere en `attempts[]`

El schema actual de `attempts[]` tiene `additionalProperties: false`. Solo permite:

- `phase`
- `result`
- `model`
- `tokens_in`
- `tokens_out`
- `cost_usd`
- `duration_s`
- `notes`

No permite campos como:

- `dispatch_id`
- `agent`
- `provider`
- `outcome`
- `noop`
- `bundle_hash`
- `forensic_commit`
- `quota_class`
- `reroute_budget`
- `usage.source`
- `diff_stat`

Además, `phase` no acepta `implement`; acepta `execute`. La idea debe mapear explícitamente:

- fase Quorum `q-implement` → `trace.attempts[].phase = "execute"`;
- decisiones de router, bundle hash, quota, no-op y dispatch metadata → `trace.events[]` o una extensión de schema.

**Riesgo:** la Fase 0 promete registrar “todo en trace desde el día 0”, pero con el schema actual eso solo es cierto si usa `events[]` para casi todo.

**Mitigación:** definir una convención de eventos antes de implementar:

- `routing_decision`
- `dispatch_started`
- `dispatch_finished`
- `reroute`
- `wrapper_broken`
- `quota_red`
- `review_family_degraded`
- `bundle_created`

Luego, si la convención se estabiliza, convertirla en extensión formal de `trace.schema.json`.

### 4.2 Append-only actual protege `attempts[]`, no `events[]`

`EnsureTraceAppendOnly()` solo compara `attempts[]`. Si el wrapper reescribe `events[]`, la validación actual no lo detecta.

**Riesgo:** la idea declara “trace como verdad”, pero su canal principal de metadata (`events[]`) puede ser reescrito accidentalmente sin romper validación.

**Mitigación:** antes de depender de `events[]` como auditoría fuerte, agregar una política de append-only para eventos o un helper específico para anexar eventos.

### 4.3 “Commit forense” no garantiza baseline limpio

La v2 dice que todo dispatch termina en:

1. commit forense si hubo diff, o
2. reset duro si no hubo diff.

Pero un commit forense deja el worktree limpio solo en el sentido de `git status`. El código del intento fallido sigue en `HEAD`. Si el siguiente modelo arranca desde ahí, no parte del baseline limpio sino del intento anterior.

**Riesgo crítico:** el fallback puede heredar cambios parciales de otro modelo y contaminar el resultado.

**Mitigación:** precisar la semántica:

- Opción A: crear commit/branch/ref forense y luego resetear la rama de tarea al pre-attempt.
- Opción B: guardar patch forense fuera de la rama y hacer reset duro.
- Opción C: permitir herencia explícita, pero entonces ya no es “fallback desde limpio”.

La recomendación es **A o B**, no dejar el commit forense como `HEAD` de la rama de tarea.

### 4.4 `data_classification` requiere cambios reales de schema

`spec.schema.json` tiene `additionalProperties: false`. Hoy `00-spec.yaml` no puede incluir `data_classification`.

**Riesgo:** la idea parece lista, pero su campo de seguridad central no es persistible.

**Mitigación:** ADR-C debe incluir:

- cambio additive en `spec.schema.json`;
- actualización de template `00-spec.yaml`;
- actualización de `q-brief`;
- actualización de `q-decompose` para herencia padre→hijos;
- default por `.quorumrc` o `.agents/config.yaml`;
- reglas de validación en `q-analyze`.

### 4.5 `config.yaml` no parece tener lector/validador core para routing

El core copia `.agents/config.yaml`, pero el runtime actual no lo consume para seleccionar modelos porque no hay dispatcher.

**Riesgo:** extender `config.yaml` sin un loader/validator dedicado deja drift silencioso.

**Mitigación:** ADR-A debe incluir preflight determinístico:

- parsear `config.yaml`;
- parsear `routing.yaml`;
- parsear `agents.yaml`;
- resolver cada modelo de `config.yaml` a un transporte;
- fallar ruidosamente si hay drift.

### 4.6 Join por nombre de modelo está subdefinido

`config.yaml` usa nombres tipo `anthropic/claude-opus-4-7`. `agents.yaml` define agentes (`claude`, `codex`, `gemini`, `opencode`) y comandos. La v2 dice “join por nombre de modelo”, pero no muestra dónde vive la lista modelo→agente.

**Riesgo:** ambigüedad entre proveedor, agente CLI y nombre de modelo.

**Mitigación:** agregar uno de estos diseños:

- `agents.yaml.agents.<agent>.models[]`;
- tabla explícita `model_routes`;
- resolver por prefijo `provider/model` con reglas declarativas.

Sin esto, el preflight no puede probar el join.

### 4.7 Wrapper fabricando `04` debe respetar schema actual

`implementation-log.schema.json` permite:

- `task_id`
- `summary`
- `entries[]`
- opcional `tdd_red_runs[]`

Cada entry exige:

- `changed_files`
- `notes`
- `verify_pending`

No permite campos extra como `blockers`, `dispatch_id`, `agent`, `model`, etc.

**Riesgo:** si el wrapper intenta persistir metadata rica en `04`, romperá schema.

**Mitigación:** `04` debe quedar mínimo y válido. Metadata de flota va a `07-trace.json.events[]`.

### 4.8 q-verify “sin LLM” cambia la semántica actual

Hoy `q-verify` es una skill: la AI ejecuta comandos y escribe `05-validation.json`. La v2 propone que verify sea mecánico y sin LLM.

**Riesgo:** esto es deseable, pero es un cambio de producto: requiere un verificador determinístico en CLI/runtime, no solo routing.

**Mitigación:** tratarlo como subpropuesta independiente: `quorum verify-run <TASK>` o equivalente dentro del dispatcher.

### 4.9 Enforcement de `touch`/`forbid` debe ser determinístico

Hoy `q-implement` instruye al agente a revisar `git diff --name-only`; `q-review` y `q-accept` revisan después. No hay todavía un gate determinístico centralizado que impida al wrapper aceptar cambios fuera de contrato.

**Riesgo:** el delegado puede modificar fuera de `touch`, y el wrapper solo lo descubriría si implementa su propio checker.

**Mitigación:** Fase 1 debe incluir un checker determinístico de diff contra contrato:

- changed files ⊆ `02-contract.yaml.touch`;
- changed files ∩ `forbid.files` = ∅;
- diff lines ≤ `limits.max_diff_lines`;
- file count ≤ `limits.max_files_changed`.

---

## 5. Riesgos de gobernanza

### 5.1 E3 no puede ser “Regla #10”

La idea propone una regla nueva candidata #10: “telemetría antes que automatización”. Pero Quorum ya tiene una Regla #10: soberanía de datos del usuario.

**Riesgo:** pisar una regla constitucional existente.

**Mitigación:** E3 debe ser:

- Regla #11, o
- principio de gobernanza de ADR, o
- sección “mecanismo de evolución constitucional”.

No debe reemplazar ni renumerar la soberanía de datos sin una migración explícita.

### 5.2 E1 introduce política con estado runtime

Esto es razonable, pero cambia el modelo mental: la política ya no es solo archivos versionados; también depende del ledger/semáforo.

**Riesgo:** reproducibilidad incompleta si no se registran todos los inputs dinámicos.

**Mitigación:** cada `routing_decision` debe registrar:

- hash de `config.yaml`;
- hash de `routing.yaml`;
- hash de `agents.yaml`;
- snapshot de semáforo;
- exclusiones activas;
- `data_classification`;
- `risk`;
- `complexity_band`;
- versión del router.

### 5.3 Ledger reconstruible desde trace no cubre toda la cuota de Claude

La idea reconoce que Claude Code puede consumir cuota fuera de wrappers. Si ese consumo entra al ledger por transcripts locales, entonces no es reconstruible solo desde traces de tareas.

**Riesgo:** contradicción con “ledger reconstruible desde traces”.

**Mitigación:** declarar dos clases:

1. **ledger de dispatch**, reconstruible desde `07-trace.json`;
2. **ledger de sesión/orquestador**, best-effort y no reconstruible desde task traces.

O registrar consumo de orquestador en un evento operativo separado, no fingir que viene de una tarea.

### 5.4 Dashboard/overrides sigue siendo de alto riesgo

La v2 ya difiere overrides a Fase 4 y ADR propio, lo cual es correcto.

**Riesgo residual:** si la tabla de control afecta routing, debe ser parte de la política reproducible.

**Mitigación:** cualquier override debe registrar:

- autor/humano;
- timestamp;
- razón;
- alcance;
- TTL;
- evento en trace de cada decisión afectada.

---

## 6. Edge cases adicionales no completamente cerrados

### 6.1 Fallback después de intento parcial con tests rotos

La v2 clasifica diff parcial como attempt. Correcto. Pero falta decir si el siguiente attempt parte del pre-attempt o del intento fallido.

**Decisión necesaria:** fallback siempre desde pre-attempt, salvo política explícita de “continuar sobre intento previo”.

### 6.2 No-op por restricciones demasiado estrictas

Un no-op puede significar incapacidad del modelo, pero también contrato insuficiente, contexto incompleto o bundler demasiado pobre.

**Riesgo:** penalizar al modelo cuando el problema era del bundle/contrato.

**Mitigación:** `noop` debe registrar causa inferida:

- `model_incapable`
- `insufficient_context`
- `contract_too_narrow`
- `wrapper_prompt_failure`
- `unknown`

La causa puede ser advisory, pero ayuda a no ajustar mal la política.

### 6.3 `sensitive` no basta para secretos

`data_classification` es necesaria, pero no detecta secretos incrustados en archivos.

**Riesgo:** una tarea `internal` puede incluir accidentalmente secretos.

**Mitigación:** considerar escaneo liviano del bundle:

- patrones de `.env`;
- tokens conocidos;
- claves privadas;
- archivos en `forbid.files`;
- rutas sensibles.

El escaneo no debe ser un LLM gate.

### 6.4 Contract tests de wrappers pueden ser caros o inestables

Si cada preflight llama CLIs reales, puede consumir cuota.

**Mitigación:** separar:

- smoke local sin modelo cuando sea posible (`--help`, parse flags);
- smoke real manual/diario;
- smoke real antes de una sesión crítica.

### 6.5 Paralelismo sobre la misma tarea

La v2 menciona lock por tarea, pero debe especificar alcance:

- lock por worktree;
- lock por task dir;
- lock por trace write;
- timeout/TTL del lock.

Sin esto, dos wrappers podrían anexar trace con race o pisarse diffs.

---

## 7. Análisis de enfoques

### Enfoque A: Fleet externo primero

Construir `llm-fleet` fuera del core con wrappers, bundler, ledger y trace events.

**Pros:**

- bajo riesgo para el core;
- permite medir Fase 0;
- no requiere mover constitución todavía;
- permite matar la tesis si L0 no rinde.

**Contras:**

- integración menos elegante;
- más disciplina manual;
- riesgo de drift si no se validan schemas.

**Esfuerzo:** medio.

**Recomendado para Fase 0–2.**

### Enfoque B: Dispatcher core directo

Implementar `quorum task run` o equivalente ya dentro del Go core.

**Pros:**

- integración limpia;
- trace y artifact-save más controlados;
- menos tooling externo.

**Contras:**

- alto riesgo constitucional;
- requiere ADRs previos;
- requiere diseñar config loaders, schema extensions y runtime;
- aumenta superficie del core.

**Esfuerzo:** alto.

**No recomendado todavía.**

### Enfoque C: Skill wrapper que orquesta todo

Crear un skill que ejecute fases y delegue a modelos.

**Pros:**

- rápido de prototipar.

**Contras:**

- choca con Regla #9;
- mezcla routing con skill;
- difícil de auditar;
- reproduce el anti-patrón que Quorum intenta evitar.

**Esfuerzo:** bajo al inicio, alto después.

**No recomendado.**

---

## 8. Ajustes recomendados al documento de idea

Antes de pasar a ADR o implementación, conviene editar la idea para incluir estas correcciones:

1. Cambiar “Regla nueva candidata #10” por “Regla #11” o “principio de evolución constitucional”.
2. Declarar que `q-implement` se registra como `trace.attempts[].phase = "execute"`.
3. Definir una convención mínima de `trace.events[]` para routing/dispatch.
4. Aclarar que `events[]` todavía no es append-only y que Fase 2/3 debe reforzarlo.
5. Resolver la semántica de commit forense: branch/ref/patch + reset a baseline.
6. Definir el modelo→agente del join `config.yaml ↔ agents.yaml`.
7. Separar ledger de dispatch vs consumo del orquestador Claude Code.
8. Especificar que `data_classification` requiere schema y cambios en `q-brief`/`q-decompose`.
9. Agregar el checker determinístico de diff contra contrato como requisito Fase 1.
10. Tratar `q-verify` sin LLM como subpropuesta de verificador determinístico.

---

## 9. Recomendación final

La v2 está **cerca de ser una propuesta ADR-ready**, pero todavía no está lista para implementación. El siguiente paso no debería ser escribir el router completo, sino partir la idea en decisiones pequeñas:

1. **ADR-A:** frontera transporte/política + join validado.
2. **ADR-B:** attempt/reroute + trace events.
3. **ADR-C:** `data_classification` + provider trust + bundler gate.
4. **ADR-D:** autoridad del dispatcher + evolución constitucional sin pisar Regla #10.
5. **Spike Fase 0:** medición externa con 5–10 hijas reales.

**Go / no-go sugerido:**

- Go para Fase 0 externa.
- No-go para core runtime todavía.
- No-go para dashboard mutable.
- No-go para auto-retry.
- No-go para cambiar constitución sin métricas.

---

## 10. Ready for proposal

**Sí, parcialmente.**

Está lista para convertirse en **propuesta de exploración/ADR split**, no en tarea de implementación directa.

La instrucción al orquestador debería ser:

> Convertir esta idea en una serie de ADRs pequeños y una Fase 0 de medición externa. No implementar dispatcher core hasta cerrar trace events, data classification, semántica de commit forense y join transporte/política.

