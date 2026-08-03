# 20 — Registro persistente de ejecuciones en `quorum fleet run`

**Fecha:** 2026-07-31
**Origen:** sesión de trabajo sobre el skill `fleet-delegate` (`~/.claude/skills/fleet-delegate/SKILL.md`). Al evaluar endurecer el umbral HIGH del capability estimate (para favorecer delegación externa aceptando más riesgo de sobre-trabajo), se detectó que ese riesgo solo es aceptable si los fallos se REGISTRAN — y hoy no se registran en ningún lado de forma sistemática.
**Estado:** ⛔ DIFERIDA tras revisión adversaria (2026-07-31) — NO trabajar aún. Ver veredicto abajo.

## Veredicto de revisión (2026-07-31)

Un agente de quorum revisó la propuesta y la rechazó con razones verificadas
contra el código; la sesión origen CONFIRMÓ el hallazgo factual clave:

1. **El punto de inserción propuesto era inalcanzable en TIMEOUT.**
   `cmd/fleet_run.go:191-194` hace return temprano en `res.TimedOut`, ANTES de
   la línea 196 — el fallo más caro jamás se habría registrado. El estimado
   SMALL estaba subestimado (falta timestamp/duración en `RunDelegateResult`;
   `fleet stats` sería un segundo pipeline, no una extensión).
2. **Rompe el contrato stateless documentado en tres sitios** (código,
   `--schema`, CLAUDE.md), con appends concurrentes no atómicos (líneas >
   PIPE_BUF por `data.output` inline) justo en campañas pass@10 paralelas.
3. **La alternativa gratis es estrictamente mejor:** `quorum fleet run --json |
   tee -a runs.jsonl` desde el invocador captura el mismo envelope INCLUIDO el
   TIMEOUT (bajo `--json` los errores también salen como JSON por stdout), sin
   tocar el CLI. El invocador es un skill, no un humano: la disciplina se paga
   una vez en su plantilla de comando.

**Gate de reactivación:** 2–3 semanas de evidencia con `tee` desde el skill
`fleet-delegate` (que ya lo incorpora en su plantilla desde 2026-07-31). Si
resulta insuficiente en la práctica, la versión mínima viable es `--log <ruta>`
opt-in explícito — sin default global, sin `--tag`, sin tocar `fleet stats`,
escrito en TODAS las salidas incluida timeout. Nada más.

**Nota de higiene para la evidencia con `tee`:** con `data.output` inline el
envelope supera fácilmente PIPE_BUF, y varios `tee -a` concurrentes al MISMO
fichero pueden entremezclar líneas. Mitigación: un fichero por campaña, o
`--output <file>` para desviar el payload grande y dejar el envelope pequeño.

---

# ANEXO HISTÓRICO — SUPERSEDED por el veredicto de arriba (2026-07-31)

> Todo lo que sigue es la propuesta ORIGINAL, conservada solo como registro.
> NO usarla como plan de trabajo ni alimentar un `/q-brief` con ella: contiene
> afirmaciones refutadas (marcadas inline). La única versión viable es la
> mínima descrita en el gate de reactivación.

## Problema

`quorum fleet run` es deliberadamente stateless (comentario de diseño en
`cmd/fleet_run.go:17-22`: "NO task, no worktree, no git operation, no forensic
ref, no 07-trace append, and no result.json"). El envelope JSON a stdout es el
único output; en cuanto el orquestador que lo invocó termina su sesión, la
evidencia de qué se ejecutó, con qué modelo y con qué resultado desaparece.

Consecuencia práctica: la calibración empírica de la escalera de capacidad del
skill `fleet-delegate` (¿qué clase de tarea falla en qué transporte/modelo?) es
artesanal — vive en notas incrustadas en el SKILL.md y en memorias HSME que
dependen de la disciplina del orquestador. Ejemplo real que motivó esto:
"opencode Tier A produjo diff vacío 2× en la clase insertar-bloque-markdown"
solo existe porque alguien lo anotó a mano.

`quorum fleet dispatch` NO tiene este problema: es task-bound y escribe
`result.json` + append a `07-trace.json`, y `quorum fleet stats` ya agrega esa
telemetría. El hueco es exclusivo del runner non-lifecycle.

## Propuesta

Append-only JSONL de cada ejecución de `fleet run`, escrito por el propio CLI
(automático, sin depender de disciplina del invocador), más un flag opcional de
etiquetado semántico.

1. **Registro automático.** En `runFleetRun` (`cmd/fleet_run.go`), tras
   `core.RunDelegate(...)` y antes de `emit.success`/`emit.failure`
   (punto de inserción **REFUTADO**: `cmd/fleet_run.go:196-213` es inalcanzable
   en TIMEOUT por el return temprano de la línea 191 — ver veredicto; el mapa
   `data` ya tiene en scope `agent`, `model`, `cwd`, `exit_code`, `killed`,
   `timed_out`), append de una línea JSON a un archivo de runs. Falta capturar
   `timestamp` (hoy no hay `time.Now()` en la función). Campos mínimos:
   `{ts, agent, model, cwd, exit_code, timed_out, killed, duration_s, tag?}`.
2. **Flag `--tag <string>` (opcional).** Etiqueta libre para que el invocador
   clasifique el run (p. ej. la clase de tarea: `insert-markdown-block`,
   `single-file-go-edit`). Sin `--tag`, el run se registra igual sin etiqueta.
3. **Ubicación del log:** decisión abierta (ver abajo). Candidatos:
   `.agents/fleet/runs.jsonl` en el `--cwd` del proyecto invocante, o un
   estado global XDG (`~/.local/state/quorum/fleet-runs.jsonl`). Nota: `fleet
   run` opera fuera del lifecycle y a menudo fuera de un proyecto Quorum, lo
   que inclina hacia el estado global.
4. **Lectura/agregación (alcance opcional de esta tarea o tarea posterior):**
   extender `quorum fleet stats` para leer también este log (hoy solo agrega
   telemetría de dispatch vía `core.CollectDispatchRecords` en
   `internal/core/fleet_stats.go`), agrupando por agent/model/tag.

## Qué NO cubre (frontera con el consumidor)

El CLI solo puede registrar lo MECÁNICO. El juicio semántico — "exit 0 pero el
diff quedó vacío", "el review Level 1 lo rechazó", capability estimada — es del
orquestador que invoca (skill `fleet-delegate`), que seguirá registrándolo por
su lado referenciando estos runs. SSOT: el CLI es dueño del hecho "qué se
ejecutó"; el invocador es dueño del veredicto "qué tan bien salió". No añadir
campos de veredicto semántico al log del CLI.

Tampoco toca `fleet dispatch` ni `07-trace.json` (ya cubiertos), ni introduce
gobernador de presupuesto (decisión 2 del índice: telemetría laxa).

## Coherencia con las decisiones de la serie

- "Trace es la verdad, el ledger es índice" — este log es índice/telemetría,
  nunca fuente de verdad de estado.
- "Telemetría antes que automatización" — registrar primero; cualquier
  auto-desactivación de celdas por tasa de fallo sería tarea futura y gated
  (el kill-switch manual de 11 sigue mandando).
- Comandos de flota bajo `quorum fleet <verbo>`; lógica en `internal/core` con
  shim fino en `cmd/` si la agregación lo amerita.
- `go test ./...` verde con `CGO_ENABLED=0`; JSONL plano, sin SQLite ni deps.

## Esfuerzo estimado

~~SMALL~~ **REFUTADO — subestimado** (ver veredicto): el punto de inserción no
cubre TIMEOUT, `RunDelegateResult` carece de timestamp/duración, y la extensión
de `fleet stats` sería un segundo pipeline (no hay `DispatchRecord` ni group-by
cell/level/band para runs task-less), no una extensión.

## Decisiones abiertas (para `/q-brief`)

1. Ubicación del log: `.agents/fleet/runs.jsonl` por-proyecto vs estado global
   XDG. (Recomendación de la sesión origen: global, porque `fleet run` corre
   con `--cwd` arbitrarios que pueden no ser proyectos Quorum.)
2. ¿La extensión de `quorum fleet stats` entra en esta tarea o es tarea aparte?
3. ¿Registrar también los runs `--dry-run` y los fallos de validación previos
   al proceso (INVALID_ENUM, FILE_NOT_FOUND)? (Propuesta: no — solo runs que
   arrancaron proceso.)
4. ¿Rotación/tope de tamaño del JSONL, o append ilimitado? (Propuesta: append
   ilimitado; es texto plano y el volumen real es bajo.)
5. Nombre del flag: `--tag` vs `--label` vs `--task-class`.

## Seguimiento

Cuando esta tarea esté implementada y verificada, la sesión origen retoma el
lado skill: redefinir HIGH sobre la ejecución-con-receta (no sobre la tarea
completa) en `fleet-delegate/SKILL.md`, y que el skill registre veredictos
semánticos referenciando los runs de este log.
