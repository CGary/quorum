# 13 — `quorum fleet route`: el router determinista

**Tipo:** función pura en core + shim CLI.
**Depende de:** 02 (agents.yaml + config reconciliado), 03 (taxonomía), 04 (complexity-score), 11 (estado de control). **GATE G1 (tarea 10): no iniciar sin la decisión sobre la celda barata y las bandas calibradas** — sin G1, este doc puede necesitar retoque (rol de L0).
**Riesgo sugerido:** high (es la política hecha software; un bug acá rutea mal TODO).

## Contexto

"Claude propone, la política dispone": el LLM puede sugerir inputs, pero quién ejecuta lo decide una función determinista, reproducible y auditable. La política vigente ya existe (`routing.yaml`: risk → executor_level; `config.yaml`: level → modelos); el router la lee — **no la duplica, no la hardcodea** (regla de `quorum.md:380`: cero nombres de modelo en lógica Go).

## Objetivo

`quorum fleet route` — stdin: `{task_id, phase, exclusions: []}` → `{agent, model, level, reroute_budget, inputs_snapshot}`.

## Diseño propuesto

**Inputs con origen declarado (anti-vibes — sin artefacto de origen no hay input):**

| Input | Origen |
|---|---|
| `phase` | estado de la tarea (CLI) |
| `risk` | `00-spec.yaml` + `risk.yaml`; el riesgo humano JAMÁS se pisa |
| `complexity_band` | `quorum analyze complexity-score` (tarea 04) |
| control | `.ai/fleet-control.json` o global (tarea 11): deshabilitados fuera del conjunto candidato |
| `exclusions` | candidatos ya fallidos del dispatch en curso (reroute) |

**Resolución:** `routing.yaml` (risk → level, overrides por tipo) → `config.yaml.levels` (level → modelos primary/fallback) → filtrar por control + exclusions → join con `agents.yaml` (¿hay transporte?) → primer candidato viable. Mapa fase→nivel de partida (política vigente, sin contrabando): brief/decompose/blueprint/accept = L2 (las hace el orquestador, no se delegan en v1); **implement = por riesgo×banda** (low×S→L0 *solo si G1 lo ratificó*; medium∨M→L1; high∨L→L2); verify = sin LLM; review = L1 con diversidad de familia *soft* (si implementó familia X, prefiere revisor de familia Y; sin alternativa → misma familia + evento `review_family_degraded`, nunca bloquea).

**Reroute = re-ejecutar el router con el fallido en `exclusions`.** No existe cadena de fallback estática: todos los gates se re-evalúan en cada salto por construcción. Sin candidato viable → `{blocked: "no_viable_candidate", reasons: [...]}` — decisión humana, no error silencioso.

**Reproducibilidad (requisito del análisis v2 §5.2):** `inputs_snapshot` incluye hashes de `config.yaml`/`routing.yaml`/`agents.yaml`, snapshot del control, risk, banda, exclusions y versión del router. Todo va al evento `routing_decision` en trace: la decisión es re-derivable años después.

## No-objetivos

- Routing por costo esperado (necesita historia modelo×fase×banda: horizonte, tarea 16).
- Urgencia como input (sin artefacto de origen todavía; entra por la tabla de control del dashboard si algún día se justifica — horizonte).
- Encadenar fases: el router decide UN destino para UNA fase; quién lo invoca (humano/skill) manda.

## Criterios de aceptación

- [ ] Función pura con tests por tabla: cada celda riesgo×banda; deshabilitado salta al fallback; exclusión + control combinados; sin candidato → blocked estructurado; review con y sin familia alternativa.
- [ ] Cero nombres de modelo en el código Go (test estructural o review estricto); cambiar `config.yaml` cambia el resultado sin recompilar.
- [ ] Evento `routing_decision` completo y re-derivable (test: con el snapshot se reconstruye la decisión).
- [ ] El riesgo humano de `00-spec.yaml` nunca es sobreescrito (test espejo del invariante de `risk.go`).
- [ ] Documentado en CLAUDE.md.
- [ ] `go test ./...` verde con `CGO_ENABLED=0`.

## Decisiones abiertas para el brief

- ¿`effort` (gpt-5-high/medium; Gemini High/Low) es parte del nombre de modelo en `config.yaml` (propuesta: sí, son entradas distintas de nivel) o dimensión aparte?
- Diversidad de familia: ¿se infiere de `agents.yaml` (provider del modelo) o se declara en `config.yaml`? Propuesta: metadato `family` en el transporte.
- ¿El router corre el complexity-score internamente o recibe la banda ya calculada? Propuesta: la recibe (función más pura, composición en el caller).
