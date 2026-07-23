# Sistema de enrutamiento de tareas entre múltiples LLMs

> **ARCHIVADO 2026-07-23 — IMPLEMENTADO.** Superado — materializado por la serie ideas/fleet/00-17 y los ADR 0010-0012.

**Fecha:** 2026-06-11
**Estado:** Idea consolidada (v2), pre-ADR. Sustituye a la v1.
**Insumos:** v1 de la idea; `*.analysis.md`; `*.analysis-fable.md`; respuesta de consolidación; `quorum.md` / README; `.agents/config.yaml`; `.agents/policies/routing.yaml`; `.agents/schemas/trace.schema.json`.
**Decisión que destraba todo lo demás:** la frontera transporte/política (§2.1–§2.2, ADR-A).

---

## 0. La idea en una frase

Construir el **dispatcher** que el roadmap de Quorum marca como prioridad #1: Claude Code (frontier) como arquitecto; una flota heterogénea de CLIs de suscripción (`codex`, `gemini`, `opencode`, `claude`) como ejecutores; un **router determinista** gobernado por la política existente (`config.yaml` + `routing.yaml`) más telemetría propia; y la constitución #7 — *"el costo está limitado por política"* — convertida en software auditable.

Esta versión asume además un mandato explícito: **Quorum tiene que evolucionar**. Su constitución está escrita para mejorar el sistema, y evolucionar implica mover límites trazados — siempre en dirección de mejora, nunca para empeorar. La v2 no esquiva los choques constitucionales que la v1 ignoraba: los nombra, propone las enmiendas y define el mecanismo de gobernanza para moverlos (§7).

### Lemas de diseño

1. **Claude propone, la política dispone.** Ningún LLM decide routing; clasifica y sugiere.
2. **Transporte ≠ política.** `agents.yaml` describe *cómo invocar*; `config.yaml` decide *quién ejecuta*.
3. **Trace es la verdad; el ledger es un índice reconstruible.**
4. **Compliance by construction.** El protocolo Quorum lo cumple el wrapper, no el delegado.
5. **Telemetría antes que automatización.** Ningún límite se mueve sin evidencia, criterio de mejora y reversión.
6. **Defensa en profundidad para datos.** Un misroute no puede exfiltrar: el bundler es la última puerta.
7. **La constitución evoluciona por ADR**, solo hacia mejora, con no-regresión declarada y camino de vuelta.

---

## 1. Cambios respecto a v1

Resolución de cada hallazgo de los análisis. Lo no listado se mantiene de v1.

| Hallazgo (análisis) | Resolución en v2 | Sección |
|---|---|---|
| §1.1 Doble registro de modelos (`agents.yaml` ↔ `config.yaml`, `quorum.md:380`) | `agents.yaml` = transporte puro; `config.yaml` = única política nivel→modelo; join validado al arranque | §2.1, §2.2 |
| §1.2 Tabla fase→tier contradice L1 de review sin declararlo | Diversidad de familia **al mismo nivel** (soft); frontier solo con `risk: high`; todo delta de política se propone con su costo recurrente | §4 |
| §1.3 Tres sistemas de retry sin unificar | Semántica única attempt/reroute por estado observable del worktree | §5 |
| §1.4 Dos economías mezcladas (API vs suscripción) | Presupuesto USD para `quota_class: api`; presupuesto requests/ventana para `subscription`; el router evalúa el que corresponda | §2.6 |
| §2.1 Semáforo proactivo con denominador desconocido | Presupuestos auto-impuestos + reactivo a firmas de quota + calibración empírica del techo desde el ledger | §2.6 |
| §2.2 Punto ciego del orquestador sobre su propia cuota | Collector best-effort (transcripts JSONL + `/usage`, a verificar en Fase 0) + margen explícito en el presupuesto de `claude` | §2.6 |
| §2.3 `agy`/Antigravity probablemente muerto | El slot se llama `gemini` (Gemini CLI, producto y cuota propios); Antigravity fuera salvo sorpresa en Fase 0 | §2.1 |
| §3 Riesgo existencial: calidad/compliance de delegación | Compliance by construction (el wrapper fabrica el `04`) + Fase 0 rediseñada como medición con umbrales | §3, §8 |
| §3.1 El contrato no acota blast radius en ejecución | Asumido explícitamente: la red real es git + verify; contrato de fin de dispatch (commit forense XOR reset) | §2.7 |
| §3.2 Riesgo de tarea ≠ sensibilidad de datos | Dimensión `data_classification` ortogonal a `risk`, aplicada en router **y** bundler | §6 |
| §4 Inputs del router sin origen | Cada input con artefacto de origen declarado; complejidad determinista; urgencia operacional registrada en trace | §2.5 |
| §4 `"{prompt}"` interpolado en shell | Entrada por stdin o prompt-file, siempre; prohibida la interpolación | §2.3 |
| §4 Doble vía de logging | Único punto de logging: el wrapper | §2.4 |
| §4 Telemetría perdida en Fases 0–2 | Attempts/reroutes a `07-trace.json` vía `task artifact-save` desde el día 0 (el schema ya lo soporta) | §2.8 |
| §4 Carrera en el semáforo con hijas paralelas | Imprecisión aceptada + margen; reserva con TTL solo si la telemetría de colisiones lo justifica | §2.6 |
| §4 Clases de fallo colapsadas | `outcome` clasificado (quota / wrapper_broken / degraded / no-op) + contract test por wrapper | §2.4 |
| §4 Dashboard sobredimensionado | Degradado: `fleet status --json` + consultas SQLite; visor futuro = `quorum serve`; overrides solo con ADR (Fase 4) | §2.8 |
| Edge cases 1–6 del análisis | Resolución normativa explícita | §9 |
| **Nuevo (mandato humano):** la constitución debe poder evolucionar | Marco de evolución constitucional: mapa regla×efecto, enmiendas E1–E3, criterio de no-regresión | §7 |

---

## 2. Arquitectura

```
00/01/02 ──► [context bundler] ──► [router] ──► [wrapper/adapter] ──► CLI delegado (worktree)
                  │ data gate         │ política + semáforo      │ contrato de dispatch
                  ▼                   ▼                          ▼
             bundle (hash)      decisión → trace          result.json → 04 fabricado,
                                                          trace (attempt/event), ledger
```

Cinco piezas: **bundler** (arma el contexto), **router** (decide destino), **wrapper** (ejecuta y normaliza), **semáforo/ledger** (gobierna presupuestos), **trace** (verdad). El dispatch runtime (§2.7) las encadena bajo un contrato de estados del worktree.

### 2.1 `agents.yaml` — transporte puro

Describe **cómo invocar** cada CLI. Nada más.

Contiene: binario, plantilla argv, vía de entrada (`stdin` | `prompt_file`), formato de salida, timeouts, flags de sandbox, firmas de fallo (quota/auth), `quota_class` (`subscription` | `api`; es un hecho de la cuenta, no un juicio), y la ruta de su contract test.

**Prohibido aquí** (es política y vive en `config.yaml`): tiers, niveles, `max_risk`, confianza de datos, presupuestos. La v1 violaba esta frontera; la v2 la declara dura.

```yaml
schema: fleet/agents.v2
agents:
  codex:
    bin: codex
    invoke: ["exec", "--json", "-m", "{model}"]     # flags a verificar en Fase 0
    input: stdin                                     # jamás interpolación shell
    cwd: "{worktree}"
    output: { format: json, usage: cli_reported }
    sandbox: { network: off_by_default }
    quota_class: subscription
    failure_signatures: { quota: ["429", "rate limit"], auth: ["401"] }
    contract_test: tests/wrappers/codex_smoke.sh
  gemini:                                            # reemplaza al slot "agy"
    bin: gemini
    invoke: ["-m", "{model}", "--output-format", "json", "-p", "@{prompt_file}"]
    input: prompt_file
    quota_class: subscription
    contract_test: tests/wrappers/gemini_smoke.sh
  opencode:
    bin: opencode
    invoke: ["run", "-m", "{provider}/{model}"]
    input: stdin
    quota_class: api
    contract_test: tests/wrappers/opencode_smoke.sh
  claude:
    bin: claude
    invoke: ["-p", "--model", "{model}", "--output-format", "json"]
    input: stdin
    quota_class: subscription
    contract_test: tests/wrappers/claude_smoke.sh
```

**Join transporte↔política:** la clave es el nombre del modelo. Preflight obligatorio al arranque: todo modelo referido por `config.yaml.levels` debe resolver a un agente de transporte. El drift entre ambos archivos es un **error ruidoso de arranque**, nunca silencioso.

### 2.2 `config.yaml` + `routing.yaml` — la política (se extiende, no se duplica)

Los niveles existentes siguen siendo la **única** traducción nivel→modelo (según lo verificado en el repo: L0 minimax/deepseek/qwen; L1 gpt-5-mini/haiku; L2 opus/gpt-5). `quorum.md:380` se respeta: no aparece una segunda tabla.

Extensiones propuestas (entran por ADR-A y ADR-C):

```yaml
# .agents/config.yaml — extensiones (la traducción nivel→modelo NO se toca)
budgets:                          # presupuestos AUTO-IMPUESTOS (el techo real es desconocido)
  codex:  { window: 5h,  max_requests: 40 }
  claude: { window: 5h,  max_requests: 60, margin: 0.30 }   # margen por punto ciego (§2.6)
  gemini: { window: 24h, max_requests: 100 }
  opencode: { window: 24h, max_usd: 5.00 }                  # economía API: USD, no requests
provider_trust:                   # juicio de datos (§6) — política, no transporte
  claude: first_party
  codex: external_standard
  gemini: external_standard
  opencode: external_low
review_diversity: soft            # §4: diversidad de familia al mismo nivel
complexity_bands: { S: ..., M: ..., L: ... }   # cortes calibrados con datos de Fase 0
```

`max_cost_per_call_usd` (ya existente) queda como tope marginal para `quota_class: api`. Las suscripciones se gobiernan por requests/ventana. El router evalúa la economía que corresponda al candidato — las dos conviven, no se mezclan.

### 2.3 Context bundler — componente con nombre propio

Constructor **determinista** del paquete de entrada al delegado: `00` + `01` + `02` + prompt de protocolo mínimo + slices de archivos derivados del blueprint. Mismo input ⇒ mismo bundle; su hash se registra en trace (regla constitucional #2 encarnada para delegación).

- **Entrada al CLI por stdin o `@prompt_file`, siempre.** El payload real es multi-KB con quoting hostil; la interpolación shell queda prohibida por construcción.
- **Data gate adentro (§6):** si la `data_classification` de la tarea excede el `provider_trust` del destino, el bundler **se niega a empaquetar**. Defensa en profundidad: aunque el router se equivoque, un misroute no exfiltra.
- **Protocolo mínimo, no protocolo Quorum completo:** al delegado se le pide lo que todo modelo de código sabe hacer — diff aplicado en el worktree + notas libres. La fabricación del artefacto válido es trabajo del wrapper (§3).
- **Prompt injection desde el repo:** el protocolo enmarca el contenido de archivos como datos, no instrucciones; sandbox de red del CLI cuando exista; y las compuertas post-hoc (contrato, verify, review) siguen siendo la red. Mitigación parcial y declarada (§11.7).

### 2.4 Wrapper / adapter — el contrato del adapter

Un wrapper por agente. Es el **único punto de logging** del sistema (los hooks de Claude Code quedan como redundancia opcional, nunca como vía primaria: solo disparan si la delegación sale de Claude Code).

Contrato obligatorio de todo wrapper:

1. **Timeout real** con kill del process group completo (no solo el proceso padre).
2. **Lock por tarea**: un dispatch activo por worktree.
3. `cwd` = worktree de la tarea; jamás la raíz del repo.
4. **Salida normalizada** (JSON único, ver abajo) escrita siempre, incluso en fallo.
5. **Detección de no-op**: exit 0 + diff vacío + notas vacías ⇒ `noop: true`.
6. **Clasificación de outcome**: `attempt_done | attempt_failed | reroute_quota | reroute_timeout | reroute_wrapper_broken`. *Wrapper roto* (vendor cambió flags), *cuota agotada* y *modelo degradado* son fallos distintos y se tratan distinto.
7. **Firmas de quota** (`failure_signatures`) ⇒ notificación inmediata al semáforo (rojo).
8. **Usage honesto**: `cli_reported` cuando el CLI lo da; `estimated` (caracteres/requests) cuando no. Nunca inventado.
9. **Contract test** por wrapper (smoke: "¿`codex exec --json` sigue parseando?"). Corre como preflight de sesión; su fallo es `wrapper_broken`, no quota — sin esto el sistema se pudre en silencio.

```json
{
  "dispatch_id": "FEAT-001-a.d3",
  "task_id": "FEAT-001-a",
  "phase": "implement",
  "agent": "codex",
  "model": "gpt-5-medium",
  "started_at": "...", "ended_at": "...",
  "exit_code": 0,
  "outcome": "attempt_done",
  "noop": false,
  "diff_stat": { "files": 3, "insertions": 120, "deletions": 8 },
  "usage": { "source": "cli_reported", "tokens_in": 41200, "tokens_out": 3800, "requests": 1 },
  "forensic_commit": "attempt-2-gpt-5-medium",
  "notes_path": ".ai/tasks/active/FEAT-001-a/dispatch/d3-notes.txt"
}
```

### 2.5 Router determinista

Función pura: `(inputs, snapshot del semáforo, exclusiones) → (agent, model, level, reroute_budget)`. Cada decisión se registra completa (inputs + output) en trace: **reproducible por construcción** (requisito de la enmienda E1, §7).

Inputs con origen declarado — sin artefacto de origen, no hay input (anti-vibes):

| Input | Origen |
|---|---|
| `phase` | Estado de la tarea (CLI Quorum) |
| `risk` | `00-spec.yaml` + `risk.yaml` (el riesgo humano nunca se pisa) |
| `complexity_band` | Función **determinista** del blueprint: `|touch|`, símbolos, deps cross-módulo, flags de migración/API pública. Mismo patrón que `risk-score`; candidato `quorum analyze complexity-score` (Fases 0–2 vive como script espejo; migra al core en Fase 3) |
| `urgency` | Operacional, no contractual: flag del dispatch (futuro: tabla de control, Fase 4 + ADR). Jamás en `00-spec`. Siempre registrada en trace |
| `data_classification` | `00-spec` u override; default por repo en `.quorumrc` (§6) |
| `semaphore` | Snapshot del ledger (estado por proveedor) |
| `exclusions` | Candidatos agotados/caídos del dispatch en curso (reroute) |

**Fallback dinámico:** la cadena de fallback **no existe como dato estático**. Reroute = volver a ejecutar el router con el candidato fallido en `exclusions`. Todos los gates (riesgo, datos, presupuesto) se re-evalúan en cada salto **por construcción**, no por disciplina. (Resuelve el edge "Codex agotado → deepseek con tarea `risk: medium`": el gate corta en el salto.)

**El LLM propone, la política dispone:** Claude (arquitecto) puede sugerir nivel o urgencia como input; el router resuelve y puede denegar. Una alucinación no quema cuota.

### 2.6 Semáforo / ledger

Dos economías separadas, nunca mezcladas:

- `quota_class: subscription` → **presupuesto auto-impuesto** de requests por ventana. El humano fija N; el sistema **no pretende conocer** el denominador real (los proveedores no lo publican y lo cambian en silencio). El "70% de la cuota" de la v1 era incomputable y queda retirado: amarillo/rojo se calculan contra el presupuesto propio.
- `quota_class: api` → presupuesto en USD por ventana + `max_cost_per_call_usd` existente.

Mecanismos:

1. **Reactivo (la única señal confiable):** firma de quota en la salida ⇒ rojo inmediato hasta el reset de ventana (o probe manual).
2. **Proactivo:** verde / amarillo (sobre el umbral del presupuesto auto-impuesto ⇒ el router prefiere alternativas) / rojo (presupuesto agotado o firma de quota).
3. **Calibración empírica:** el ledger acumula "requests acumulados al momento de cada 429" y estima el techo real ⇒ **sugiere** ajustar el presupuesto. Sugerencia al humano, nunca auto-ajuste (regla E3, §7).
4. **Punto ciego del orquestador, asumido:** el consumo de la sesión de Claude Code no pasa por wrappers, y es la cuota más cara del sistema. Mitigación best-effort: collector de los transcripts JSONL locales (usage por mensaje, el mismo insumo que parsean herramientas tipo ccusage) + `/usage`. Se verifica en Fase 0; funcione o no, el presupuesto de `claude` lleva `margin` explícito (p. ej. 30%) mientras la visibilidad sea parcial. El semáforo se diseña asumiendo visión incompleta.
5. **Concurrencia (hijas paralelas):** dos dispatches pueden leer verde a la vez y ambos consumir. v2 **acepta la imprecisión** (presupuestos con margen) y la mide (contador de colisiones en el ledger). Reserva con TTL solo si la telemetría lo justifica — automatizar la solución antes de medir el problema violaría el lema 5.

### 2.7 Dispatch runtime — contrato de fin de dispatch

Invariantes de estado del worktree (la red real es git + verify, no el contrato — asumido):

1. Todo dispatch **parte de worktree limpio**.
2. Todo dispatch **termina en exactamente uno de dos estados**: (a) commit forense `attempt-N-<model>` si hubo diff; (b) reset duro si no lo hubo. Sin estados intermedios que sobrevivan al dispatch.
3. El fallback **siempre parte de limpio**; nunca retoma el parcial de otro modelo.
4. El commit forense es material de diagnóstico para el arquitecto (y para la telemetría) sin ensuciar la rama: se squashea o descarta antes del merge humano. Nunca se mergea — regla #6 intacta.

**Dónde corre:** Fases 0–2, invocado por el humano o por un subagente dispatcher de Claude Code (`model: haiku`, solo Bash) — orquestación mecánica sin gastar frontier. **No hay daemon**: diseñar el router como si existiera un runtime que no existe produce orquestación fantasma. El dispatcher como subcomando/proceso de Quorum llega en Fase 3, por ADR.

### 2.8 Observabilidad — trace como verdad, ledger como índice

- **Desde el día 0**, cada attempt y cada reroute se anexa a `07-trace.json` vía `quorum task artifact-save` — el schema **ya soporta** `model`, `tokens_in/out`, `cost_usd` por attempt y `events` libres (verificado). Vivir en repo aparte no excusa perder telemetría: esa historia es exactamente el insumo de las Fases 4–5.
- El SQLite del semáforo es **vista materializada reconstruible** desde los traces. Test de reconstrucción en Fase 2 (borrar ledger ⇒ reconstruir ⇒ mismo estado). Nunca segunda fuente de verdad.
- Consumo humano: `fleet status --json` + consultas SQLite directas cubren el 90%. Visor: reusar `quorum serve` cuando exista, read-only primero. **No se construye mini-API propia.** Overrides en caliente (tabla de control) recién en Fase 4 y con ADR propio.

---

## 3. Compliance by construction — atacar la causa del riesgo existencial

El análisis es correcto: pedirle a un CLI delegado que produzca `04-implementation-log.yaml` válido contra schema, en inglés, con BLOCKED estructurado, hunde a los modelos baratos en fallos de compliance. La v2 elimina la exigencia en lugar de medir su fracaso:

- **Al delegado se le pide lo que sabe hacer:** aplicar un diff en el worktree y dejar notas libres.
- **El wrapper fabrica el `04` válido, determinísticamente:** `files` sale de `git diff --stat`; `decisions`/`blockers` del parseo de notas; si el texto libre no alcanza, un modelo L0 actúa como formateador con el schema en mano (uso mecánico y barato).
- **Consecuencias:** colapsa la clase entera de "fallo de compliance" (idioma, schema, estructura); el edge "artefacto en español o schema-inválido" casi desaparece — diff bueno con log feo es trabajo del wrapper, no un attempt quemado; y la tasa de éxito de delegación pasa a medir **lo único que importa: pasar `q-verify`** (regla #4 elevada a métrica única).

La ecuación económica queda abierta a propósito y se mide en Fase 0: puede que *barato + N reintentos + diagnóstico frontier* pierda contra *mid directo*. Si pierde, la política lo refleja — la tesis no se defiende, se mide.

---

## 4. Política de routing v2 (fase → nivel, alineada a `config.yaml`)

| Fase | Nivel | Regla |
|---|---|---|
| `q-brief`, `q-decompose`, `q-blueprint`, `q-accept` | L2 | El arquitecto. Aquí se decide todo; ahorrar aquí sale caro |
| `q-implement` | Por riesgo × complejidad: `low×S → L0`; `medium ∨ M → L1`; `high ∨ L → L2` | Bandas calibradas con datos de Fase 0, no a priori |
| `q-verify` | **Sin LLM** (ejecuta `verify.commands`); L0 solo como auxiliar de lectura de logs | Es mecánico |
| `q-review` | **Nivel vigente (L1)** + diversidad de familia al mismo nivel, como restricción blanda: si implementó familia X, revisa familia Y (haiku implementa ⇒ gpt-5-mini revisa: costo extra cero) | Escala a L2 **solo** con `risk: high`. Si no hay familia alternativa disponible: misma familia + event `review_family_degraded` en trace. Nunca bloquea |
| Fabricador de `04`, formateo, parsing | L0 | Tareas mecánicas: el nicho natural de los modelos baratos |

Dos notas de gobernanza:

1. **Ningún cambio de política entra de contrabando** (lección §1.2 del análisis): la v1 ponía review en frontier contradiciendo la L1 vigente sin declararlo. La v2 parte de la política vigente; cualquier delta futuro se propone como cambio explícito con su costo recurrente y su tradeoff.
2. **Router maduro (Fase 4+):** con historia de éxito por modelo×fase×banda de riesgo en trace, el routing evoluciona de tier fijo a **costo esperado** = intentos esperados × costo marginal + overhead de diagnóstico. La política deja de ser estática porque la telemetría existe — no antes.

---

## 5. Semántica unificada de reintentos: attempt vs reroute

Hoy conviven `routing.yaml.max_attempts` (por riesgo) y `02-contract.yaml.retry_policy`; la "cadena de fallback" de la v1 era un tercer sistema sin relación declarada. La v2 unifica con **un criterio observable** — clasifica el estado del worktree/salida, no una interpretación:

| Señal observable al terminar el dispatch | Clase | Consume | Registro |
|---|---|---|---|
| Diff no vacío (trabajo completo o parcial), pase o no verify | **attempt** | `max_attempts` del contrato | `trace.attempts[]` + commit forense |
| Exit 0 + diff vacío + notas vacías (no-op) | **attempt** (con flag `noop`) | `max_attempts` | `trace.attempts[]` |
| Diff inválido/inaplicable tras reparación del wrapper | **attempt** | `max_attempts` | `trace.attempts[]` |
| Firma de quota (429) **con diff vacío** | **reroute** | `reroute_budget` del dispatch (default: 2) | `trace.events[]` + semáforo rojo |
| Timeout/crash del CLI **con diff vacío** | **reroute** | `reroute_budget` | `trace.events[]` |
| Salida imparseable (vendor cambió flags) | **reroute** + alerta `wrapper_broken` | `reroute_budget` | `trace.events[]` + contract test flag |

Jerarquía: el **contrato manda** dentro de la tarea (`retry_policy` y `max_attempts` gobiernan attempts); el `reroute_budget` pertenece al dispatch y **no toca** el contador del contrato. Así un 429 no quema los reintentos que el contrato creía limitados — y a la inversa, el trabajo malo no se disfraza de mala suerte de infraestructura.

¿Por qué el no-op es attempt y no reroute? Porque es evidencia de incapacidad del modelo sobre la tarea (lo "hizo" y no produjo nada), no un fallo de infraestructura. Tratarlo como reroute produciría spinning gratuito contra el mismo nivel.

---

## 6. Datos y privacidad: `data_classification` ⊥ `risk`

`risk` mide el impacto del cambio; **no protege datos**. Una tarea `risk: low` puede llevar código propietario en el bundle a un modelo externo barato. Dimensión nueva, ortogonal:

- `data_classification: open | internal | sensitive`. Default por repo en `.quorumrc`; override por tarea en `00-spec.yaml`.
- `provider_trust` en política (§2.2): `first_party` (claude) / `external_standard` (codex, gemini) / `external_low` (opencode y modelos de bajo costo externos). Valores ilustrativos: el ADR-C los fija.
- Mapa de gates (ilustrativo): `sensitive → solo first_party`; `internal → ≥ external_standard`; `open → cualquiera`.
- **Doble aplicación:** el router no elige destinos prohibidos, y el bundler se niega a empaquetar (§2.3). El gate del router es eficiencia; el del bundler es seguridad. Un misroute no exfiltra.

---

## 7. Evolución constitucional

**Mandato (humano):** la constitución de Quorum está escrita para mejorar el sistema. Evolucionar implica mover límites trazados — en pos de mejora, nunca para empeorar, al servicio de la evolución del producto. Este sistema es el primer caso de prueba serio de ese mandato, y la v2 lo trata como ciudadano de primera clase en lugar de esquivarlo.

**Operacionalización de "nunca para empeorar":** toda enmienda constitucional exige un ADR con tres elementos, sin los cuales no hay enmienda:

1. **Métrica que mejora** (qué evidencia del trace la justifica).
2. **Invariantes de no-regresión** (qué anti-objetivos del §10 declara intactos).
3. **Mecanismo de reversión** (cómo se deshace — el análogo constitucional de `task back`).

### Mapa regla × efecto de esta idea

| Regla | Efecto |
|---|---|
| 1. Git es la verdad | **Reforzada**: commits forenses, reset duro, fallback desde limpio (§2.7) |
| 2. Contexto determinista | **Reforzada**: el bundler es su implementación para delegación; hash del bundle a trace (§2.3) |
| 3. Sin parches fuera del contrato | **Preservada**: validación post-hoc; se asume explícitamente que la red en ejecución es git + verify (§2.7) |
| 4. La validación es la finalidad | **Elevada**: pasar `q-verify` es LA métrica de éxito de delegación (§3) |
| 5. Artefactos machine-first | **Preservada** vía compliance by construction: el wrapper garantiza el artefacto válido (§3) |
| 6. Commitea, nunca mergea | **Intocable** (anti-objetivo §10) |
| 7. Costo limitado por política | **Implementada y enmendada** (E1) |
| 8. Tests son la única prueba | **Preservada** |
| 9. Skills de fase única, sin routing | **Preservada para skills; extendida** con una nueva autoridad: el dispatcher (E2) |

### Enmiendas propuestas (entran juntas en ADR-D, Fase 3)

**E1 — Regla 7 evoluciona: política con estado de runtime.**
*Texto candidato:* "El costo está limitado por política **evaluada sobre telemetría registrada**: presupuestos por proveedor/ventana, semáforo y — cuando exista por ADR — tabla de control. Toda decisión de routing es reproducible desde sus inputs registrados en trace."
*Mejora:* los límites dejan de ser estáticos y ciegos; responden al estado real de cuotas y a la urgencia operacional.
*No-regresión:* el determinismo y la auditabilidad se mantienen — la decisión deja de ser solo un archivo, pero cada decisión queda registrada con sus inputs y es reproducible. La política sigue disponiendo; ningún LLM decide.
*Reversión:* desactivar semáforo/tabla de control devuelve el routing a `config.yaml` estático sin pérdida de datos (trace conserva la historia).

**E2 — Regla 9 evoluciona: nace la autoridad de encadenamiento.**
*Texto candidato:* "Los skills siguen siendo de fase única y no deciden routing. Existe una única autoridad de encadenamiento y enrutamiento: el **dispatcher**, que es política ejecutable — no un skill ni un LLM. El dispatcher nunca ejecuta `task back` ni merge; las tres auto-transiciones de skills y el rollback exclusivamente humano quedan intactos."
*Mejora:* habilita el roadmap #1 (dispatcher automático) sin erosionar lo que la regla 9 protegía: que los skills no se arroguen routing. La prohibición no se borra — se reubica la autoridad en una pieza diseñada para ejercerla bajo política.
*No-regresión:* `back` y merge siguen siendo humanos; los skills no cambian; cada encadenamiento queda en trace.
*Reversión:* apagar el dispatcher devuelve el despacho manual fase a fase, que sigue funcionando (Fases 0–2 lo prueban por construcción).

**E3 — Regla nueva (candidata #10): telemetría antes que automatización.**
*Texto candidato:* "Ningún límite constitucional se mueve sin evidencia registrada en trace, un criterio de mejora explícito y un camino de reversión."
*Por qué:* codifica el mecanismo evolutivo mismo. Es la regla que legitima E1/E2 y la que disciplina las futuras (auto-retry, renegociación de contrato, merge-gate, tabla de control): primero medir, después automatizar. La constitución gana la capacidad de evolucionar sin perder la capacidad de decir que no.

**Secuencia de adopción:** las enmiendas entran recién en Fase 3 — las Fases 0–2 son un experimento externo que no toca la constitución. Si la Fase 0 mata la tesis, no hay enmienda. La constitución no se mueve por una idea: se mueve por evidencia. Eso también es parte del mandato.

---

## 8. Roadmap por fases (con umbrales de calidad)

### Fase 0 — Medición. El entregable es un número, no flags verificados

Sobre **5–10 hijas reales representativas** (mezcla de complejidad S/M y riesgo low/medium), con el mismo bundler y wrapper mínimos, dos celdas: un modelo barato (L0) y uno mid (L1).

Métricas a producir:

- `pass@1`: % de implementaciones delegadas que pasan `q-verify` sin intervención humana.
- `pass@≤1-reroute`: ídem admitiendo un reroute.
- Tasa de no-op por modelo.
- % de `04` que el wrapper tuvo que fabricar/reparar (mide cuánto compró el §3).
- Costo y tiempo de diagnóstico del arquitecto cuando el delegado falla (el otro lado de la ecuación económica).

**Criterio de decisión explícito (umbral propuesto, a ratificar):** si el modelo barato da `pass@1 < 50%` y `pass@≤1-reroute < 70%`, L0 queda fuera de `q-implement` (se conserva para tareas mecánicas §4) y la tesis "implement en barato" se revisa **antes de escribir una línea del router**.

Subproductos: flags headless reales por CLI; autopsia de `agy` / confirmación del slot `gemini`; verificación del collector de auto-observabilidad de Claude Code (transcripts JSONL + `/usage`); preflight de join `config.yaml ↔ agents.yaml` implementado.

### Fase 1 — Transporte completo

Bundler + wrappers con el contrato del §2.4 + routing estático leyendo los niveles vigentes de `config.yaml`. Sin semáforo.
**DoD:** una hija end-to-end por cada agente del registro con artefactos válidos; ≥80% de dispatches sin intervención humana de compliance (el wrapper fabrica el `04`); cero prompts por interpolación shell (test); contract tests de los cuatro wrappers en verde.

### Fase 2 — Presupuestos y reroute

Ledger + presupuestos auto-impuestos + semáforo reactivo + reroute dinámico + clasificación de fallos.
**DoD:** simulacro de agotamiento de un proveedor ⇒ continuidad automática por alternativa **re-evaluando gates en el salto**; 100% de decisiones de routing con registro completo en trace; test de reconstrucción del ledger desde traces en verde; colisiones de concurrencia medidas (no necesariamente resueltas).

### Fase 3 — Integración formal con Quorum

ADR-A (frontera transporte/política), ADR-B (attempt/reroute), ADR-C (`data_classification`), ADR-D (enmiendas E1–E3). `complexity-score` migra a `quorum analyze`. El dispatch runtime se vuelve subcomando con las mismas garantías.
**DoD:** ADRs aceptados; la prueba ácida del CI del core sigue verde **sin la flota presente** (misma frontera de compilación que HSME: el core no sabe que la flota existe).

### Fase 4 — Optimización

Routing por costo esperado (historia modelo×fase×banda desde trace) activable por política, con comparación A/B contra tier fijo registrada. Visor read-only vía `quorum serve`. Tabla de control de overrides en caliente — con su propio ADR.
**DoD:** decisión documentada con datos: costo esperado ¿sí o no?; overrides auditables en trace.

### Fase 5 — Escalación propuesta (no automática)

Si un modelo falla verify N veces, el sistema **propone** subir de nivel y el humano confirma. Coherente con el diferimiento vigente de auto-retry, y disciplinado por E3: solo telemetría acumulada puede justificar el siguiente paso de autonomía.

---

## 9. Edge cases — resolución normativa

| # | Caso | Resolución (regla que lo cubre) |
|---|---|---|
| 1 | Delegado cuelga a mitad de edición; timeout lo mata | Kill de process group (§2.4) + contrato de fin de dispatch (§2.7): diff parcial ⇒ commit forense + attempt; sin diff ⇒ reset. El worktree nunca queda sucio entre dispatches |
| 2 | Exit 0 pero no hizo nada | Detección de no-op en el wrapper (§2.4) ⇒ attempt con flag `noop` (§5) |
| 3 | 429 a mitad de tarea con ediciones parciales | Diff no vacío ⇒ **attempt** + commit forense + semáforo rojo. El siguiente candidato parte de worktree limpio; nunca retoma el parcial de otro modelo (§2.7) |
| 4 | Fallback cruza gates (Codex agotado ⇒ siguiente es deepseek, pero `risk: medium` y deepseek no califica) | No hay cadena estática: reroute re-ejecuta el router con exclusiones y **todos los gates se re-evalúan en el salto** (§2.5) |
| 5 | Artefacto en español o schema-inválido | Compliance by construction (§3): el wrapper fabrica el `04` desde diff + notas. Si el *diff* es válido, no hay fallo. Solo diff inválido/inaplicable cuenta como attempt (§5) |
| 6 | Review cruzado no disponible (familia alternativa caída o en rojo) | Degrada a misma familia con event `review_family_degraded` en trace. La diversidad es soft: nunca bloquea (§4) |
| 7 | Dos hijas paralelas leen "verde" a la vez y ambas consumen | Imprecisión aceptada + margen en presupuestos; colisiones medidas en el ledger; reserva con TTL solo si la telemetría lo justifica (§2.6) |
| 8 | El vendor cambia flags y el wrapper parsea basura | Outcome `wrapper_broken` ≠ quota ≠ degradación (§2.4); contract test como preflight; alerta en vez de fallback silencioso |

---

## 10. Anti-objetivos — límites que esta evolución declara intactos

Estos son los invariantes de no-regresión que toda enmienda de esta línea (E1–E3 y futuras) referencia. Moverlos requeriría su propio ADR con su propia evidencia — no vienen incluidos en esta idea:

- **No merge automático a `main`.** Jamás. El commit forense se squashea o descarta; el merge es humano.
- **No auto-retry ni escalación sin confirmación humana** hasta Fase 5, y aun ahí: propuesta, no acción.
- **No proxy API en el medio** (LiteLLM y similares no aplican a suscripciones): la medición vive en el wrapper.
- **No segunda fuente de verdad:** el ledger es reconstruible desde trace, siempre.
- **No nuevos slots de artefactos** (`08+`): todo cabe en `04`/`05`/`07` + memoria curada.
- **No memoria automática:** `q-memory` sigue siendo exclusivamente human-invoked.
- **No routing decidido por LLM:** propone, no dispone.
- **No daemon fantasma:** ningún componente simula un runtime que no existe; la autonomía llega por fases y por ADR.

---

## 11. Riesgos abiertos y preguntas para la siguiente ronda de análisis

1. **Umbrales de Fase 0** (50% / 70%): propuestos a ojo razonado; ratificar o ajustar antes de correr la medición.
2. **Reserva vs imprecisión** en el semáforo: diferido a la telemetría de colisiones de Fase 2 (E3 aplicada a una decisión interna).
3. **Formato de transcripts de Claude Code** es interno e inestable: el collector es best-effort por diseño; el `margin` del presupuesto `claude` compensa mientras tanto. ¿Hay señal más estable que valga la pena pedir/esperar?
4. **Cadencia de contract tests** por wrapper: ¿preflight de cada sesión, cron diario, o ambos?
5. **Cortes de las bandas S/M/L** de `complexity-score`: calibrar con los datos de Fase 0; no fijar a priori.
6. **¿HSME como señal de complejidad?** `blueprint-context` podría enriquecer el input del router. Opcional y subordinado: HSME informa, no decide — y la flota debe funcionar igual sin él.
7. **Prompt injection desde archivos del repo** hacia delegados: mitigación parcial (framing de datos + sandbox de red + gates post-hoc). ¿Justifica un escaneo del bundle, o el costo supera el riesgo en un repo propio?
8. **Ecuación económica del L0** (barato + reintentos + diagnóstico vs mid directo): es la pregunta que la Fase 0 responde con datos; mantenerla abierta hasta entonces.

---

## 12. Borradores de ADR (título + decisión en una línea)

- **ADR-A — Frontera transporte/política.** `agents.yaml` describe invocación (binario, argv, entrada, salida, sandbox, firmas, quota_class, contract test); `config.yaml` decide ejecución (niveles, presupuestos, trust, diversidad). Join por nombre de modelo, validado en preflight; el drift es error de arranque.
- **ADR-B — Semántica unificada de reintentos.** attempt (consume `max_attempts` del contrato; criterio: diff no vacío, no-op, o diff inválido) vs reroute (consume `reroute_budget` del dispatch; criterio: quota/timeout/parseo con diff vacío). El contrato manda; todo a trace.
- **ADR-C — `data_classification` ortogonal a `risk`.** Clasificación por repo/tarea + `provider_trust` en política; aplicada en router (eficiencia) y bundler (seguridad).
- **ADR-D — Enmiendas constitucionales E1–E3.** Regla 7 con telemetría; autoridad dispatcher preservando skills de fase única y rollback humano; regla nueva "telemetría antes que automatización". Cada una con métrica de mejora, invariantes de no-regresión (§10) y reversión.
