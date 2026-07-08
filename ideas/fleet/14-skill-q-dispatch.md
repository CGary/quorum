# 14 — Skill `/q-dispatch`: la interfaz humana de la flota

**Tipo:** skill (`.agents/skills/q-dispatch/SKILL.md`), capa fina sobre los comandos `fleet`.
**Depende de:** 13 (route), 06/07 (dispatch), 12 (protocolo BLOCKED), 11 (status).
**Riesgo sugerido:** medium (es un skill: debe cumplir el protocolo de skills y la Regla #9 a rajatabla).

## Contexto

El skill es la cara; el código es el músculo. `/q-dispatch` NO calcula rutas ni controla procesos: invoca `fleet route` y `fleet dispatch`, traduce resultados al humano en español, y administra la conversación BLOCKED. Restricción constitucional verificada: Regla #9 — fase única, sin auto-encadenar, sin routing decidido por el skill (el routing lo decide `fleet route`, que es política ejecutable). Los invariantes de protocolo de skills tienen test: `internal/core/skill_protocol_test.go` — el skill nuevo debe pasarlo. Alcance explícito: el mandato de /q-dispatch es la flota EXTERNA (fleet route / fleet dispatch). Invocar subagentes de Claude Code (definiciones .claude/agents/) queda fuera de alcance y vive en la capa de andamiaje del orquestador, no en este skill.

## Objetivo

Skill que, dado un task ID en fase de implementación (alcance v1: SOLO la fase implement), ejecuta un ciclo de delegación y se detiene.

## Flujo del skill

1. Precondiciones: tarea en `active/` con worktree (`task start` ya corrido), `01`/`02` presentes. Si falta algo → informar y terminar (sin auto-correr transiciones: no está en la tabla de auto-transiciones autorizadas).
2. `quorum fleet status --json` → si todo rojo, informar y terminar.
3. `quorum analyze complexity-score` + `quorum fleet route` → **mostrar la decisión al humano ANTES de despachar**: agente, modelo, nivel, por qué (señales), y costo de oportunidad. Confirmación humana en v1 (`[Obligatorio] ¿Despachar? (sí / elegir otro / cancelar)`) — la confirmación se relaja a opt-in cuando haya confianza, no antes.
4. `quorum fleet bundle` + `quorum fleet dispatch` → reportar outcome:
   - `attempt_done` → resumen del diff + `[Obligatorio] Ejecutar /q-verify` (lo corre el humano u orquestador — **jamás auto-encadenado**).
   - `reroute_*` → informar causa, mostrar siguiente candidato de `fleet route --exclusions`, pedir confirmación para re-despachar.
   - `blocked` → formatear la pregunta rica (tarea 12) en español: contexto, evidencia, opciones con consecuencias, recomendación, opción abierta. Terminar con `ESPERANDO RESPUESTA DEL USUARIO...`.
   - `attempt_failed`/`noop` → evidencia + opciones (`[Opcional] reintentar con otro modelo` / `[Opcional] implementar localmente` / referencia a `quorum task back <ID>` como rollback humano).
5. Fin de turno. Siempre.

## Protocolo de skill (obligatorio, del CLAUDE.md del repo)

Salida en español; campos persistidos en inglés (los persiste el código, no el skill); `ESPERANDO RESPUESTA DEL USUARIO...` SOLO en turnos que esperan; acciones marcadas `[Obligatorio]`/`[Opcional]`; sin auto-activar otros skills; sin `task back`.

## Criterios de aceptación

- [ ] `skill_protocol_test.go` pasa con el skill nuevo incluido.
- [ ] El SKILL.md no contiene lógica de routing (solo invocaciones a `fleet route`) — review estricto contra Regla #9.
- [ ] Ensayo end-to-end documentado: una hija real delegada vía `/q-dispatch` de punta a punta, incluyendo un caso BLOCKED real o simulado.
- [ ] El flujo reroute respeta `reroute_budget` y lo comunica (cuántos saltos quedan).
- [ ] Caso "todo rojo" y caso "sin candidato viable" producen mensajes accionables, no errores crípticos.

## Decisiones abiertas para el brief

- ¿Confirmación humana pre-dispatch siempre (propuesta v1) o configurable por riesgo (low auto-confirma)?
- Nombre: `/q-dispatch` vs `/q-delegate`. (`dispatch` alinea con la terminología constitucional del dispatcher.)
- ¿El skill muestra estimación de "qué haría el router para las próximas fases" (informativo) o estrictamente la fase actual? Propuesta: solo la actual — menos superficie, menos tentación de encadenar.
