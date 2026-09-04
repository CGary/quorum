# 🐞 Defecto: el catálogo de modelos de la flota anuncia celdas que el transporte ya no ofrece

**Estado:** **Implementado.** Revisado contra el código el 2026-09-03 (8 ajustes aprobados por el humano); ejecutado como FLEET-035 / FLEET-036 / FLEET-037 y mergeado en `main` el 2026-09-04 (merge `88fe9ff`). Quedan fuera de alcance: propagación a hexcell, campaña de smoke de Gemini 3.8, e idea 14 (ambigüedad del preflight).
**Contexto:** Quorum, `.agents/config.yaml` (niveles 1 y 2), `.agents/fleet/agents.yaml`, `quorum fleet dispatch`, `quorum analyze fleet-preflight`, `docs/adr/0011-attempt-reroute-blocked-trace.md`.
**Origen:** Detectado el 2026-09-02 en el repositorio `hexcell`, tarea HEX-060, en el primer despacho del implement.
**Veredicto:** Dos celdas de las rutas apuntan a un modelo que el proveedor retiró. Una es el **primario del nivel 2**; la otra es una celda inalcanzable del nivel 1 con un comentario obsoleto. Además, el rechazo del transporte se clasifica como `attempt` (attempt_failed) en vez de `reroute/wrapper_broken`, con lo que un defecto de configuración **detiene el ciclo en el humano y envenena la estadística de la celda**.

---

## 1. Lo que pasó

```
$ quorum fleet dispatch  # nivel 2, primario del catálogo
Error: invalid model selection (--model "Gemini 3.5 Flash (High)" --effort ""):
  model Gemini 3.5 Flash (High) is not recognized as a known model
Available models:
  Gemini 3.8 Flash (High/Medium/Low)
  Gemini 3.7 Flash (High/Medium/Low)
  Gemini 3.6 Flash (High/Medium/Low)
  Gemini 3.1 Pro (High/Low)
  Claude Sonnet 4.6 (Thinking) · Claude Opus 4.6 (Thinking) · GPT-OSS 120B (Medium)
```

`exit_code` 1, duración 2,88 s, diff vacío, worktree intacto. **La serie 3.5 desapareció por completo del catálogo del transporte**, y apareció la 3.8, que la flota todavía no conoce.

El histórico confirma que no es un error de tipeo: `agy_edit/google/gemini-3.5-flash-low` acumula 4 éxitos de 4 en el ledger de despachos. Esos modelos **existieron**; el proveedor los retiró. Verificado el 2026-09-03 invocando el binario `agy` con un modelo inválido: el catálogo vivo es 3.8 Flash ×3, 3.7 ×3, 3.6 ×3, 3.1 Pro ×2, Sonnet 4.6, Opus 4.6, GPT-OSS 120B. Declarado y muerto: el trío 3.5 (en `agy` y en `agy_edit`, `agents.yaml:186-192` y `280-293`). Vivo y no declarado: el trío 3.8.

## 2. Las dos celdas muertas (revisado 2026-09-03)

**`.agents/config.yaml:173` — primario del nivel 2:**

```yaml
    primary: google/gemini-3.5-flash-high
    fallback: google/gemini-3.6-flash-high
```

El nivel 2 sirve `{high,S}`, `{high,M}`, `{low,L}` y `{medium,L}`. Tiene **exactamente dos celdas**. El propio archivo dejó escrito el riesgo antes de que ocurriera:

> `RISK ON RECORD, now without margin: 3.5-flash-high is the primary of FOUR matrix rules and still has NO agentic smoke evidence` … `Run the 3/3 smoke on 3.5-flash-high before trusting level 2.`

Ese smoke nunca se corrió. El comentario predijo el incidente y quedó como advertencia sin acción.

**`.agents/config.yaml:111` — secondary del nivel 1.** Corrección respecto a la primera versión de esta idea: **no es la única garantía cruzada de transporte.** El secondary del nivel 1 es `[google/gemini-3.5-flash-medium, nvidia/nemotron-3-ultra-550b-a55b-free]`; el escape real de la suscripción antigravity es nemotron (opencode/OpenRouter). La celda 3.5-medium nunca lo fue: resuelve a `agy_edit`, la misma suscripción que sonnet/opus, y en `agy` sería oneshot, excluida del implement por `core.Route`. El comentario de `config.yaml:112-125` ("three cells above", "ONLY escape from the antigravity subscription"; el test `TestFleetRouteLevel1DegradesWhenCodexDisabled` vive en `cmd/fleet_route_test.go:166` y debe seguir en verde con nemotron como cola) quedó obsoleto tras la poda del 27-08. Hoy la celda muerta cuesta un salto inútil del `reroute_budget` antes de llegar a nemotron en el escenario de kill-switch.

## 3. Por qué el coste es mayor que "un intento perdido" (semántica corregida)

`quorum fleet dispatch` clasificó el rechazo como `outcome.class: attempt` con `diff.empty: true`, que por ADR 0011 es **attempt_failed**. Debería ser `reroute` con causa `wrapper_broken`: el delegado nunca llegó a razonar sobre la tarea.

Corrección: `attempt_failed` **no** consume `reroute_budget` (ADR 0011 §"Jerarquía": el presupuesto de reroute pertenece a la capa de dispatch y nunca decrementa `max_attempts`). Lo que ocurre es peor:

- consume `max_attempts` del contrato y **detiene el ciclo** en el humano (`q-dispatch` presenta evidencia y remite a `quorum task back`), en lugar de auto-reroutear a la siguiente celda;
- cuenta como `Failure` en `quorum fleet stats` (`success_rate = success / (success + failure)`; los `reroute` van en contador aparte y no la degradan). La celda muerta envenena el ledger y, con n ≥ 4 y < 50 %, `q-orchestrate` la excluye: el nivel 2, con dos celdas, **bloquea** → implement interno, la fase más cara.

Bien clasificado, el defecto costaría un salto de `reroute_budget` (= `max_attempts` de la regla en `routing.yaml`: 2 en niveles 1-3, 4 en nivel 0) y no envenenaría ninguna estadística.

## 4. Baseline: decisión humana del 27-08 y deriva documental

El config actual es una **decisión humana del 2026-08-27** (commit `39e493a`, razonada en `config.yaml:97-107` y `157-166`) que revirtió la escalera del 26-08: nivel 1 = sonnet-4-6 → opus-4-6 → [3.5-medium, nemotron-ultra]; nivel 2 = 3.5-high → 3.6-high; nivel 3 = 3.7-high → 3.1-pro-high. `AGENTS.md` (CLAUDE.md es un symlink) aún describe la escalera del 26-08 (nivel 2 = 3.6-high → 3.7-low → 3.7-medium, sin celdas 3.5). El párrafo "2026-08-26 ladder rebalance" debe actualizarse con el estado real.

## 5. Corrección aprobada (2026-09-03) — tres tareas

- **FLEET-035 — retirar las celdas 3.5 y poner la doc al día.** `config.yaml`: nivel 2 = `google/gemini-3.6-flash-high` (primario; 13 éxitos de 16 en el ledger) → `google/gemini-3.7-flash-medium` (fallback: cruza generación, respeta "un modelo, un hogar" y el rechazo del 27-08 a repetir modelo; la cita "Pending human decision: add a third cell (3.7-flash-medium is unrouted)" está en el bloque del nivel 3, `config.yaml:204`, y sigue siendo válida). Nivel 1: retirar `3.5-flash-medium` del secondary, nemotron queda como cola cruzada, corregir el comentario obsoleto. `agents.yaml`: retirar las 6 entradas 3.5 de `agy` y `agy_edit`. `AGENTS.md`: párrafo de la escalera actualizado. Alternativa descartada: restaurar `3.6-high → 3.7-low → 3.7-medium` (repite modelo, lo que el 27-08 rechazó).
- **FLEET-036 — `wrapper_broken` declarativo.** Sin strings hardcodeados en core: campo `wrapper_signatures` por transporte en `agents.yaml` + `agents.schema.json` (`additionalProperties: false`, hay que declararlo), plumb en `DispatchSpec` (`fleet_dispatch.go:43/215/301`, mismo camino que `failure_signatures`) y nuevo `case` en `classifyOutcome` (`fleet_dispatch.go:339`) antes de `!outputParseOK` → `reroute` / `wrapper_broken`; test en `fleet_dispatch_test.go`. Firmas para `agy` y `agy_edit`: `invalid model selection`, `is not recognized as a known model`.
- **FLEET-037 — `quorum fleet catalog <transporte>`.** `quorum analyze fleet-preflight` es puro por diseño (`fleet_preflight.go:28-31`: sin filesystem, red ni exec) y la convención `analyze` es JSON por stdin sin efectos; **no se extiende**. El nuevo comando vive en el grupo `fleet` (que ya ejecuta procesos: smoke/run/dispatch), es manual-only como `smoke` (nunca CI), invoca el binario con un modelo centinela, parsea `Available models:` y devuelve `{declared_dead, live_undeclared}` cruzando los display names del mapa `models` de `agents.yaml`. El formato del error no es contrato estable: si no parsea, el resultado es `unknown`, nunca `fail`.

## 6. Fuera de alcance de estas tareas

- **Propagación:** hexcell tiene la misma copia (commit `6651519`, 27-08). Arreglar Quorum no arregla hexcell: copiar `config.yaml`, `agents.yaml` y el schema tras el merge.
- **Gemini 3.8:** vivo pero sin smoke; por proven-before-new va en una campaña de smoke aparte antes de entrar en rutas.
- **Mitigación inmediata:** las dos celdas muertas quedaron deshabilitadas por kill-switch el 2026-09-03 (`quorum fleet disable agy_edit/google/gemini-3.5-flash-{high,medium}`) para que el pipeline no tropiece con ellas mientras se implementa. Limpiadas con `quorum fleet enable` tras el merge (2026-09-04).

## 7. Regla operativa mientras tanto

Ante un despacho con `exit_code` 1 y duración de pocos segundos, **leer `notes.txt` antes de sacar cualquier conclusión sobre la capacidad del modelo**. Un fallo de tres segundos es configuración, no razonamiento. Confundirlos lleva a excluir por evidencia una celda que nunca tuvo la oportunidad de fallar por mérito propio, y a envenenar el ledger que gobierna el ruteo futuro.

## 8. Relación con otras ideas

La idea 12 documenta otro fallo silencioso del mismo CLI (`quorum task start` escribiendo un localizador de directorio en un campo de identidad). El patrón compartido: **una capa detecta el problema y lo reporta, pero la clasificación río abajo lo trata como si fuera otra cosa**, y el proceso continúa con un estado degradado que nadie declaró.
