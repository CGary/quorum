# 06 — `quorum fleet dispatch`: motor de despacho + adapter codex

**Tipo:** comando core con control de procesos. La pieza más delicada de la serie.
**Depende de:** 02 (agents.yaml), 03 (taxonomía/eventos), 05 (bundle).
**Riesgo sugerido:** high (ejecuta procesos externos que escriben en worktrees; primer código de flota con efectos).

## Contexto

El wrapper es la frontera de fiabilidad: normaliza la ejecución de CLIs ajenos e inestables, es el ÚNICO punto de logging del sistema, y garantiza los invariantes de estado del worktree. Todo lo que el wrapper no garantice por construcción, alguien lo tendrá que garantizar por disciplina — o sea, nadie.

## Objetivo

`quorum fleet dispatch` — request JSON por stdin: `{task_id, agent, model, bundle_path, timeout_s?, dispatch_id}` → ejecuta el CLI delegado en el worktree y escribe SIEMPRE un `result.json` normalizado, pase lo que pase.

## Contrato del wrapper (innegociable, del v2 §2.4 + analysis-v2)

1. **Timeout real con kill del process group completo** (setpgid + kill negativo), no solo el proceso padre.
2. **Lock por tarea**: un dispatch activo por worktree (archivo lock con pid+ttl; lock huérfano detectable).
3. `cwd` = worktree de la tarea (codex: `-C {worktree}`); jamás la raíz del repo.
4. **Invariantes de estado del worktree** (resuelve el hallazgo "commit forense ≠ baseline limpio"):
   - Todo dispatch parte de worktree limpio (precondición verificada, aborta si no).
   - Al terminar: si hubo diff → snapshot forense como **ref fuera de la rama** (`refs/fleet/<task>/attempt-N-<model>`) y luego `reset --hard` al baseline; si no hubo diff → `reset --hard` directo. La rama de tarea NUNCA queda apuntando al intento: el siguiente candidato parte de baseline real, no hereda parciales. El forense queda accesible para diagnóstico y se descarta antes del merge humano (regla #6 intacta). Excepción: si el attempt va a `q-verify` inmediato (flujo normal), el diff se deja aplicado en el worktree y el snapshot forense se toma igual — el reset aplica cuando el dispatch falla o se reroutea.
5. **Detección de no-op**: exit 0 + diff vacío + notas vacías ⇒ `noop: true`.
6. **Clasificación de outcome** (taxonomía de la tarea 03): `attempt_done | attempt_failed | reroute_quota | reroute_timeout | reroute_wrapper_broken | blocked`.
7. **Firmas de fallo** de `agents.yaml` (quota/auth) detectadas en salida ⇒ outcome correspondiente + notificación al kill-switch (tarea 11; hasta que exista, solo evento `quota_red` en trace).
8. **Usage honesto**: `cli_reported` (codex `--json` reporta usage en eventos JSONL — confirmar formato con inventario 0a) o `estimated`/`none`. Jamás inventado.
9. **Telemetría a trace SIEMPRE**: evento `dispatch_started` (bundle_hash) y `dispatch_finished` (result), y si es attempt, entrada en `attempts[]` con `phase: "execute"` — todo vía `quorum task artifact-save` (preserva append-only y validación).

## Adapter codex (incluido en esta tarea)

`codex exec` con: prompt por **stdin**, `-C {worktree}`, `--sandbox workspace-write`, `--json` (eventos JSONL parseados para usage/progmarkers), `-o {result_dir}/last-message.txt`, `-m {model}`. Evaluar `--output-schema` para estructurar el mensaje final (notas + señal BLOCKED) — si funciona, simplifica el parseo del fabricador del 04 y del protocolo BLOCKED.

## Criterios de aceptación

- [ ] `result.json` se escribe siempre: éxito, fallo, timeout, kill, salida imparseable (test por cada vía).
- [ ] Test de timeout: proceso hijo que ignora SIGTERM muere igual (process group).
- [ ] Test de invariantes: dispatch sobre worktree sucio aborta; tras dispatch fallido el worktree quedó en baseline y existe el ref forense; tras no-op no hay ref.
- [ ] Lock: segundo dispatch concurrente sobre la misma tarea falla claro.
- [ ] Eventos y attempt registrados en `07-trace.json` de una tarea real con el formato de la tarea 03.
- [ ] El adapter codex implementa el delegado con un **fake binario** en tests (script que simula salidas: éxito/429/timeout/basura) — sin gastar cuota real en CI.
- [ ] `go test ./...` verde con `CGO_ENABLED=0`.

## Decisiones abiertas para el brief

- ¿El dispatch de un attempt exitoso deja el diff aplicado (para `q-verify` inmediato) o siempre resetea y `q-verify` corre sobre el ref? Propuesta: deja aplicado (flujo natural actual) + snapshot; ratificar.
- Formato exacto de `result.json` (partir del borrador v2 §2.4).
- ¿`reroute_budget` se gestiona acá o en el router (tarea 13)? Propuesta: el dispatch solo clasifica; el presupuesto lo administra quien re-rutea.
