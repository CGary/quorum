# 11 — Kill-switch manual + semáforo reactivo

**Tipo:** estado de control + subcomandos CLI + integración con dispatch.
**Depende de:** 03 (evento `quota_red`), 06 (el dispatch notifica). No depende de G1.
**Riesgo sugerido:** medium.

## Contexto

Decisión tomada: NO se calcula la cuota de los proveedores (denominador desconocido, en cuyo cálculo la v1 tropezó). El humano monitorea el usage en las consolas de cada proveedor y apaga modelos manualmente; el sistema solo reacciona automático ante la única señal confiable: la firma de quota (429) en la salida del delegado. Esto reemplaza TODO el aparato de presupuestos auto-impuestos/calibración/reserva de la v2 §2.6 — queda una pieza chica y honesta.

## Objetivo

1. **Estado de control** legible/escribible: qué agentes/modelos están deshabilitados, por quién y por qué.
2. **CLI**: `quorum fleet disable <agent[/model]> [--reason "..."]`, `quorum fleet enable <agent[/model]>`, `quorum fleet status [--json]`.
3. **Reactivo**: outcome `reroute_quota` en un dispatch ⇒ auto-disable del agente con `reason: quota_signature` + evento `quota_red` en el trace de la tarea que lo detectó.

## Diseño propuesto

- Estado en `.ai/fleet-control.json` (runtime, gitignorado como el resto de `.ai/` operativo): `{disabled: [{target, reason, by: "human"|"quota_signature", at}], updated_at}`. Escritura atómica (rename) para tolerar lecturas concurrentes de dispatches paralelos.
- **Semántica en vuelo:** deshabilitar NO mata dispatches en curso; el próximo `fleet route` (tarea 13) lo respeta. Documentado, sin sorpresas.
- **Re-habilitación: SOLO humana** (`fleet enable`), sin TTL ni auto-recovery — coherente con "el humano lidera". El `status` muestra hace cuánto está rojo para que el humano decida.
- Mientras el router no existe (pre-tarea 13), `fleet status` ya sirve al flujo manual: el humano/skill consulta antes de despachar.
- `fleet status --json` es la fuente que consumirá el dashboard (tarea 15) — diseñar el JSON pensando en ese consumidor (estado por agente/modelo, último dispatch conocido, razón y antigüedad del disable).

## No-objetivos

Presupuestos por ventana, contadores de requests, estimación de cuota restante, calibración: **nada de eso existe**. Si el día de mañana la telemetría acumulada justifica presupuestos, será otra tarea con su propio ADR (lema: telemetría antes que automatización).

## Criterios de aceptación

- [ ] `disable`/`enable`/`status` con tests (incluido target inexistente en `agents.yaml` → error claro).
- [ ] Dispatch con firma de quota (fake binario que emite 429) produce auto-disable + evento `quota_red` (test de integración con la tarea 06).
- [ ] Escrituras concurrentes no corrompen el archivo (test con escrituras paralelas).
- [ ] Deshabilitar durante un dispatch en vuelo no lo interrumpe (test).
- [ ] `fleet status` legible para humanos y `--json` estable para máquinas.
- [ ] `go test ./...` verde con `CGO_ENABLED=0`.

## Decisiones abiertas para el brief

- Granularidad del target: ¿agente entero, modelo específico, o ambos? Propuesta: ambos (`codex` apaga todo codex; `codex/gpt-5-high` solo ese modelo).
- ¿El auto-disable por quota apaga el agente o solo el modelo que dio 429? Propuesta: el agente (la cuota de suscripción suele ser por cuenta, no por modelo) — ratificar con lo aprendido en Fase 0a.
- Ubicación del estado: `.ai/fleet-control.json` (por proyecto) vs `~/.quorum/` (global por máquina, las cuotas son por cuenta no por proyecto). Argumento fuerte para global; decidir.
