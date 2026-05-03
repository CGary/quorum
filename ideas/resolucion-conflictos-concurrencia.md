# 🔀 Propuesta Técnica: Validación de Compatibilidad Pre-Merge para Tareas Concurrentes

**Estado:** Diferida — implementar cuando exista pipeline real de runtime/review.  
**Contexto:** Evolución de Quorum v1.2+.  
**Origen:** Refinamiento de la idea original "Resolución de Conflictos y Gestión de Concurrencia (Integrator Protocol)".  
**Veredicto de factibilidad:** El problema es real, pero la versión original era demasiado ambiciosa para el estado actual de Quorum. Lo implementable a futuro es una **validación pre-merge determinística** basada en Git + `verify.commands`, no un agente integrador autónomo.

---

## 1. El Problema Real (acotado)

Quorum ya permite múltiples tareas activas en worktrees aislados. Eso introduce tres riesgos genuinos:

1. **Conflicto de texto al integrar**: dos ramas tocan la misma zona de código.
2. **Conflicto semántico**: una tarea valida contra una API vieja en su worktree, mientras `main` ya cambió.
3. **Obsolescencia del contrato**: `02-contract.yaml` se firmó contra una base que ya no es la actual.

La consecuencia no es que Quorum necesite "más agentes", sino que necesita una forma determinística de responder una pregunta concreta antes del merge humano:

> “¿La rama `ai/<TASK_ID>` sigue siendo compatible con el `main` actual cuando se integra y se re-ejecutan sus `verify.commands`?”

---

## 2. Lo que YA existe en Quorum (NO re-implementar)

Ya documentado en `quorum.md` §🔀 Concurrency & Merge-Gate Governance:

| Componente ya existente | Dónde | Rol |
|---|---|---|
| Worktrees aislados por tarea | `worktrees/<TASK_ID>/` | Base para paralelismo seguro. |
| Rama por tarea | `ai/<TASK_ID>` | Aislamiento lógico frente a `main`. |
| Merge humano obligatorio | Regla #6 | El sistema puede commitear; no mergea. |
| Fase `merge_gate` | `07-trace.json.attempts[].phase` | Hogar natural de la validación de integración. |

La propuesta original describía parte de esto como si faltara. No falta. Ya está.

---

## 3. Qué sí se debería implementar a futuro

### 3.1 Shadow merge + verify como merge gate

Cuando una tarea solicite pasar a review/merge gate, el sistema debe:

1. Crear un worktree temporal desde el `main` actual.
2. Intentar un `git merge --no-commit` de `ai/<TASK_ID>` sobre ese `main`.
3. Ejecutar los `verify.commands` del `02-contract.yaml`.
4. Registrar el resultado en `07-trace.json` como intento con `phase: merge_gate`.

### 3.2 Veredicto de integración

Estados lógicos deseables:

- **compatible**: el shadow merge y `verify.commands` pasan.
- **conflicted**: Git no puede aplicar el merge limpio.
- **out_of_sync**: Git mergea, pero la validación funcional falla sobre el `main` actual.

Estos estados no necesitan hoy un artifact nuevo; pueden vivir primero como `notes` o como metadatos estructurados cuando el schema de trace se extienda explícitamente.

### 3.3 Drift calculado de forma lazy

No monitorear continuamente `main`.

En cambio:

- Registrar el commit base cuando se crea el worktree.
- Comparar contra `main` actual solo cuando:
  1. la tarea pide review/pre-merge;
  2. el humano pide status de sincronización;
  3. exista una rutina periódica explícita en el futuro.

Esto evita trabajo innecesario y no obliga a mutar todas las tareas activas cada vez que `main` avanza.

---

## 4. Lo que NO se implementará de la versión original

| Componente rechazado o diferido | Razón |
|---|---|
| Agente Integrador dedicado | Esto es Git + verificación determinística; no requiere LLM. |
| Monitoreo continuo de `HEAD` de `main` para marcar tareas como stale | Complejidad y costo innecesarios; el drift se puede calcular lazy. |
| Campos `sync_status` y `last_main_sync_commit` añadidos ad hoc a `07-trace.json` | El schema actual tiene `additionalProperties: false`; si se necesitan, deben entrar por cambio deliberado de schema. |
| Auto-rebase automático como política por defecto | Riesgo alto: reescribe ramas y complica auditoría. |
| "Commit de merge listo para humano" | Se acerca demasiado a violar Regla #6. El humano mergea. |
| Renegociación automática de contrato tras conflicto | Depende de runtime + protocolo de renegociación aún diferidos. |

---

## 5. Precondiciones para desbloquear

**No implementar §3 hasta que TODAS estas condiciones se cumplan:**

| # | Precondición | Justificación |
|---|---|---|
| 1 | `task_manager.run_task` ya no es stub. | Sin runtime real, no hay pipeline donde insertar merge gate. |
| 2 | Existe flujo explícito de review/pre-merge en CLI. | Hoy no hay comando operativo para esa transición. |
| 3 | `07-trace.json` ya se usa de forma consistente durante ejecución. | El merge gate debe quedar auditado en trace, no ser una acción invisible. |
| 4 | Hay política clara para qué hacer cuando el merge gate falla. | Sin recovery definido, el gate solo bloquea sin orientar. |
| 5 | El protocolo de renegociación de contrato sigue diferido o está estabilizado. | Si ambos existen, deben coordinarse; si no, mejor mantener el gate independiente. |

---

## 6. Trazabilidad de la decisión

- **Propuesta original:** Integrator Protocol con monitoreo de `main`, shadow merge, estados de sync, auto-rebase y renegociación.
- **Análisis de factibilidad:** el problema es válido, pero el 60-70% del valor práctico viene de un merge gate determinístico; el resto depende de runtime/review inexistentes o agrega automatización riesgosa.
- **Acción inmediata:** actualizar `quorum.md` para documentar lo que ya existe y bloquear sugerencias repetidas.
- **Acción diferida:** este documento — reducir la idea al MVP futuro de shadow merge + verify.
- **Acción NO creada en `3-exec/`:** no hay tarea ejecutable inmediata, porque el sustrato operativo aún no existe.

---

## 7. Próximos pasos para el agente revisor (cuando se desbloquee)

1. Diseñar un comando explícito tipo `agents task premerge <TASK_ID>`.
2. Definir cómo se registra el resultado en `07-trace.json` (`merge_gate` + notes o nueva extensión de schema).
3. Implementar primero el gate sin auto-rebase.
4. Confirmar que el flujo no viola Regla #6 ni introduce merge automático encubierto.
5. Solo después evaluar si vale la pena formalizar metadata extra de drift en el schema.
