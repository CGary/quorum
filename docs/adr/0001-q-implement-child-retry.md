# ADR 0001: Retry Autorizado de Tareas Hijas en /q-implement

## Estado
Propuesto (Autorizado por FEAT-005-a)

## Contexto
En el modelo de descomposición de Quorum, las tareas grandes se dividen en tareas hijas independientes. Actualmente, si una tarea hija falla en su fase de validación (`q-verify`) o revisión (`q-review`), el flujo se detiene. Para mejorar la resiliencia operativa en tareas complejas sin violar la Regla Inmutable #9 (Modularidad de Skills), es necesario formalizar cómo y cuándo un skill de implementación puede reintentar una tarea fallida.

## Decisión
Se autoriza al skill `/q-implement` (y al despachador que lo invoca) a ejecutar reintentos sobre tareas hijas que hayan fallado previamente, siempre que se cumplan las siguientes condiciones normativas:

1. **Iniciación Externa**: El retry debe ser solicitado por el orquestador o despachador (humano o runtime), nunca decidido autónomamente por el skill `/q-implement` en medio de una sesión.
2. **Preservación de Trazas**: El archivo `07-trace.json` DEBE permanecer como append-only. Cada intento de retry debe añadirse al array `attempts[]` sin borrar, compactar ni sobreescribir los intentos fallidos anteriores.
3. **Frontera de Autoridad**: El retry no autoriza el merge automático. El merge a `main` sigue siendo una acción exclusivamente humana (Regla #6).
4. **Rollback Manual**: El retry no automatiza `quorum task back`. Si un retry requiere volver a un estado limpio de la rama o el worktree, el humano debe decidir y ejecutar la reversión.
5. **Visibilidad de Fallos**: El log de implementación (`04-implementation-log.yaml`) debe reflejar que se está trabajando sobre un intento previo fallido, referenciando los hallazgos de `05-validation.json` y `06-review.json` del intento anterior.

## Consecuencias
*   **Positivas**: Mayor autonomía del sistema para corregir errores de implementación o fallos ambientales en tareas granulares.
*   **Negativas**: Riesgo de bucles de retry infinitos si no se respetan los límites de `max_attempts` en la política de ruteo.
*   **Neutrales**: Se mantiene la Regla #9 ya que el skill sigue siendo de una sola fase; el reintento es simplemente un nuevo despacho de la misma fase sobre el mismo artefacto en estado fallido.
