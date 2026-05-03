# 📂 Propuesta Técnica: Gobernanza de Artefactos de Cierre de Ciclo

**Estado:** Mayormente rechazada — conservar como decisión de diseño para evitar reintroducción.  
**Contexto:** Evolución de Quorum v1.2+.  
**Origen:** Refinamiento de la propuesta original "Estructura de Artefactos Expandida (Lifecycle v1.2)".  
**Veredicto de factibilidad:** No implementar `08-post-mortem.json` ni `09/10-impact-report.json`. El aprendizaje post-fallo y post-éxito ya tiene hogares naturales en artefactos existentes y en `q-memory`.

---

## 1. Problema real detectado

La propuesta original buscaba cerrar el ciclo de aprendizaje después de:

1. una tarea fallida;
2. una tarea exitosa;
3. un merge a `main`;
4. una divergencia entre intención inicial y resultado final.

El problema es válido: Quorum debe aprender de fallos y éxitos sin repetir errores ni contaminar la memoria.

El error de diseño estaba en resolverlo con más archivos numerados.

---

## 2. Lo que YA existe en Quorum (NO re-implementar)

Ya documentado en `quorum.md`:

| Necesidad | Hogar existente | Razón |
|---|---|---|
| Comando fallido, exit code y output | `05-validation.json.commands[]` | Captura factual del fallo. |
| Causa, notas de revisión y fix tasks | `06-review.json.notes` + `fix_tasks` | El reviewer ya analiza el fallo. |
| Paso, coste, intento y resultado global | `07-trace.json.attempts[]` | Observabilidad y auditoría económica. |
| Lecciones duraderas de fallos | `memory/lessons/` vía `q-memory` | Aprendizaje curado, no log automático. |
| Patrones confirmados tras éxito | `memory/patterns/` vía `q-memory` | Conocimiento reutilizable. |
| Decisiones arquitectónicas | `memory/decisions/` o `docs/adr/` | Decisión durable, trazable. |
| Anti-patrones | `memory/*[].anti_patterns` | Dead-ends preservados sin artefacto nuevo. |

Conclusión: el contenido de `08` y `09/10` ya tiene ubicación canónica.

---

## 3. Qué NO se implementará

| Componente original | Veredicto | Motivo |
|---|---|---|
| `08-post-mortem.json` | Rechazado | Duplica `05-validation.json`, `06-review.json`, `07-trace.json` y `memory/lessons/`. |
| `09-impact-report.json` | Rechazado | Duplica `q-memory`; crea un paso intermedio sin consumidor determinístico. |
| Agente "Diagnostic Agent" | Rechazado | Duplica `q-review` y `q-memory`. |
| Agente "Integrator Agent" para reportes | Rechazado | El integrador determinístico futuro debe validar, no generar narrativa semántica. |
| Indexación inmediata en HSME | Rechazado | Quorum es local-first; HSME puede consumir `memory/*.json`, pero no es dependencia del framework. |
| Generación automática al mover a `done/failed/` | Rechazado por ahora | Violenta la curation gate de `q-memory` y depende de un runtime aún incompleto. |

---

## 4. Qué sí podría implementarse a futuro

No se recomienda crear artefactos nuevos. Si en el futuro aparece una necesidad real, debe implementarse como una de estas opciones, en este orden:

### 4.1 Mejorar productores existentes

- `q-verify`: enriquecer `05-validation.json` con campos mínimos y opcionales si hay un consumidor claro.
- `q-review`: mejorar `notes` y `fix_tasks` si falta causalidad.
- `q-memory`: destilar lecciones/patrones/decisiones desde tareas `done/` y `failed/`.
- `07-trace.json`: añadir eventos o fases cuando sea observabilidad de runtime.

### 4.2 Crear memoria curada, no reportes intermedios

Para aprendizaje durable:

- éxito repetible → `memory/patterns/`;
- decisión de arquitectura → `memory/decisions/`;
- causa de bug, fallo o anti-patrón → `memory/lessons/`.

No generar un `impact-report` previo salvo que exista un consumidor determinístico que lo necesite.

### 4.3 Extender schema solo con ADR

Si algún día se propone un nuevo artefacto numerado, debe cumplir:

1. no duplicar campos existentes;
2. tener schema oficial antes de que se escriba;
3. tener un comando o skill que lo consuma;
4. tener una política de retención clara;
5. pasar por ADR en `docs/adr/`.

---

## 5. Precondiciones para reabrir esta idea

No reabrir la expansión de artefactos hasta que exista evidencia de al menos una de estas condiciones:

| Señal | Qué demostraría |
|---|---|
| `05/06/07` se vuelven demasiado grandes o ambiguos | Necesidad de separación estructural real. |
| `q-memory` no puede destilar patrones desde artefactos existentes | Falta un input específico, no necesariamente un archivo nuevo. |
| Un consumidor automático necesita datos que no pueden derivarse | Justificación para nuevo schema. |
| ≥10 tareas muestran pérdida repetida de causalidad post-fallo o post-éxito | Evidencia empírica de ROI. |

---

## 6. Trazabilidad de la decisión

- **Propuesta original:** añadir `08-post-mortem.json` y `09-impact-report.json` como cierre de ciclo.
- **Análisis de factibilidad:** ambos duplican mecanismos existentes y aumentarían ruido operativo.
- **Acción inmediata realizada:** `quorum.md` ahora incluye §Artifact lifecycle boundary para bloquear futuras sugerencias falsas.
- **Acción ejecutable ahora:** ninguna. No se requiere `3-exec` porque no hay código que implementar; es una decisión de arquitectura/documentación.
- **Acción futura permitida:** mejorar `q-memory`, `q-review`, `q-verify` o `07-trace.json` solo cuando exista consumidor y evidencia.

---

## 7. Próximos pasos si alguien insiste en un nuevo artefacto

1. Demostrar qué dato no puede vivir en `05`, `06`, `07` o `memory/*`.
2. Definir el consumidor determinístico antes que el productor.
3. Escribir JSON Schema con `additionalProperties: false`.
4. Crear ADR en `docs/adr/`.
5. Añadir tests que prueben que el nuevo artefacto evita duplicación, no solo que valida.
