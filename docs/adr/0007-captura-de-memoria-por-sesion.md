# ADR 0007: Captura de memoria por sesión

## Estado

Aceptado

## Contexto

En cada sesión de trabajo, humano y agente toman decisiones, aplican patrones y aprenden lecciones que hoy se pierden cuando termina la sesión sin estar asociadas a una tarea del lifecycle. Quorum mantiene memoria curada en SQLite, pero hasta ahora estaba acoplada a una tarea (`00`→`07`). Se propuso un skill de handoff de sesión que capture conocimiento de alta señal de la sesión de trabajo.

## Decisión

Se permite un skill `q-session` human-invoked que persiste `decision`/`pattern`/`lesson` originadas en el diálogo de una sesión (no en una tarea), reusando `quorum memory save` y el schema vigente sin cambios, marcando `source_task = "SESSION-YYYY-MM-DD"`. Reafirma: única ruta de escritura = `quorum memory save`; sin auto-captura; single-phase; sin tipo/tabla nuevos; curación humana obligatoria.

## Consecuencias

La memoria deja de estar acoplada 1:1 a una tarea; `SESSION-*` es un valor de `source_task`, no un nuevo namespace de tarea (no entra a `FindTaskDir`/lifecycle). El visor (ADR 0006) las muestra sin trabajo extra. Riesgo de ruido mitigado por la confirmación humana.
