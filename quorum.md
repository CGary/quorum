# Flujo definitivo de implementación de features con agentes de IA

## 1. Principio rector

> Un agente nunca recibe "el proyecto". Recibe una misión acotada, un contrato estricto, archivos permitidos, contexto recuperado deterministamente, y un comando que decide si terminó. Toda la ceremonia vive en el pipeline; el modelo solo ve lo mínimo necesario. El humano interviene en checkpoints nombrados, no continuamente. El sistema mide costo por tarea desde el primer día.

```
El humano decide dirección, riesgo y merge.
El código decide validaciones, rutas, límites, worktrees, costos y reintentos.
El LLM ejecuta tareas acotadas bajo contrato.
```

---

## 2. Las cuatro fuentes de verdad

Tabla operativa, no slogan. Cuando haya duda sobre dónde guardar, leer o validar algo, esta tabla la resuelve.

| Pregunta | Fuente de verdad | Artefacto concreto |
|----------|------------------|---------------------|
| ¿Qué código existe? | **Git** | Repositorio + worktrees |
| ¿Funciona? | **Tests / verify** | `verify.commands` + `05-validation.md` |
| ¿Cuánto costó? | **Trace** | `07-trace.json` |
| ¿Qué aprendimos? | **Memoria semántica** | `memory/` (decisions, patterns, lessons) |
| ¿Qué puede hacer el agente? | **Contract** | `01-contract.yaml` |
| ¿Quién aprueba dirección y merge? | **Humano** | `human_gates` declarados |

```
Git           = verdad del código.
Tests         = verdad funcional.
Trace         = verdad económica.
Memoria       = aprendizaje selectivo (no fuente de verdad del código).
Contract      = autoridad operativa del agente.
Humano        = autoridad de dirección.
```

---

## 3. Reglas congeladas (no negociables)

Estas reglas no cambian con iteraciones futuras. Son el núcleo invariante del sistema.

```
1.  El sistema puede commitear. El sistema nunca hace merge a main.
2.  El modelo no decide cuánto gastar. El dispatcher decide cuánto gastar.
3.  El humano aprueba plan Full y merge final, siempre.
4.  El executor nunca recibe el proyecto completo.
5.  El contrato define touch/forbid; el applier los hace cumplir.
6.  La memoria no es fuente de verdad del código. Git lo es.
7.  Si el mismo error aparece dos veces, se escala o se detiene.
8.  Todo intento queda registrado en trace.json.
9.  Contexto truncado sin aviso es un bug del sistema.
```

---

## 4. Perfiles de ceremonia

No todos los features merecen la misma ceremonia. El perfil se decide humanamente en Fase 0 según `complexity × risk`.

| Perfil | Cuándo aplica | Archivos requeridos | Fases activas |
|--------|---------------|---------------------|---------------|
| **light** | `atomic + low risk`. Bugfixes, ajustes UI, renombrados. | `01-contract.yaml`, `07-trace.json` | 1, 2, 5, 7–11, 14, 15 |
| **standard** | `atomic + medium risk` o `compound + low risk`. | `00-brief.md`, `01-contract.yaml`, `02-context.md`, `04-implementation-log.md`, `05-validation.md`, `07-trace.json` | 1–3, 5–14, 15 |
| **full** | `high risk`, `compound + medium/high`, decisión arquitectónica nueva. | Todos los archivos numerados + ADR previo | Todas |

### Reglas de asignación de perfil

```
atomic + low                   → light
atomic + medium                → standard
compound + low                 → standard
high risk (cualquiera)         → full
compound + medium/high         → full
decisión arquitectónica nueva  → full + ADR previo
```

No todo `compound` debe ir a `full`. Un feature que toca varios componentes UI sin riesgo funcional puede ser `standard`.

---

## 5. Estructura de directorios

```
project/
├── .agents/
│   ├── config.yaml                   # routing, presupuestos, modelos por nivel
│   ├── prompts/                      # plantillas versionadas en Git
│   │   ├── architect/default.md
│   │   ├── executor/{bugfix,refactor,feature,test,migration}.md
│   │   └── reviewer/default.md
│   ├── retrievers/
│   │   ├── ast_neighbors.py
│   │   └── import_graph.py
│   ├── policies/
│   │   ├── routing.yaml
│   │   └── risk.yaml
│   └── schemas/
│       ├── contract.schema.json
│       ├── trace.schema.json
│       ├── review.schema.json
│       └── memory.schema.json
│
├── .ai/tasks/
│   ├── inbox/
│   ├── active/
│   │   └── FEAT-001-pos-payment-method/
│   │       ├── 00-brief.md           # humano, prosa libre, NO consume el LLM
│   │       ├── 01-contract.yaml      # máquina, schema estricto, SÍ consume el LLM
│   │       ├── 02-context.md         # contexto curado por humano (perfil full/standard)
│   │       ├── 03-plan.md            # solo perfil full, generado por arquitecto
│   │       ├── 04-implementation-log.md
│   │       ├── 05-validation.md
│   │       ├── 06-review.md
│   │       ├── 07-trace.json         # autogenerado obligatorio
│   │       ├── artifacts/
│   │       │   ├── search-results.txt
│   │       │   ├── context-bundle.txt
│   │       │   ├── context-overflow.md     # solo si excede presupuesto
│   │       │   └── contract-violation.md   # solo si hubo violación
│   │       ├── patches/                    # diffs generados (modo A)
│   │       │   └── attempt-NN.patch
│   │       └── fixes/                      # subtareas de corrección
│   │           └── FIX-001-review-feedback.yaml
│   ├── done/
│   └── failed/
│
├── docs/adr/
├── memory/
│   ├── decisions/
│   ├── patterns/
│   └── lessons/
├── worktrees/                              # en .gitignore
└── src/
```

**Regla**: los agentes no modifican `.agents/` salvo que exista una task explícita de mantenimiento del sistema.

---

## 6. Fases del flujo

Cada fase declara: **quién actúa**, **qué consume**, **qué produce**.

### Fase 0 — Decisión previa (humano, 30 s – 2 min)

**Consume**: la idea de feature.
**Produce**: clasificación mental `complexity × risk × profile`.

Cuatro preguntas obligatorias:

1. ¿Es atomic o compound?
2. ¿El riesgo es low, medium o high?
3. ¿Hay decisión arquitectónica nueva? Si sí → primero ADR.
4. ¿Cómo se verifica que terminó? (Comando shell que retorna 0.)

---

### Fase 1 — Brief y contrato (humano, 2–8 min)

**Consume**: decisiones de Fase 0.
**Produce**: `00-brief.md` (si `profile ≠ light`) y `01-contract.yaml` (siempre) en `tasks/inbox/<ID>-<slug>/`.

`00-brief.md` es prosa humana para tu yo futuro y reviewers humanos. **El LLM no lo consume**.

`01-contract.yaml` es el contrato máquina. Schema estricto. Inglés. Validable.

---

### Fase 2 — Pickup por el orquestador (código, instantáneo)

**Consume**: contrato + decisión humana de mover de `inbox/` a `active/`.
**Produce**: worktree creado, `07-trace.json` inicializado, perfil cargado.

Pasos:

1. Validar `01-contract.yaml` contra `contract.schema.json`. Si falla → `failed/`.
2. `git worktree add worktrees/<ID> -b ai/<ID> origin/main`.
3. Inicializar trace.

Comando equivalente: `agents task start <ID>`.

---

### Fase 3 — Recuperación de contexto (código + humano opcional, 2–10 min)

**Consume**: `read[]`, `goal`, `acceptance`, `touch[]`, `limits`.
**Produce**: `artifacts/search-results.txt`, `artifacts/context-bundle.txt`, opcionalmente `02-context.md` y `artifacts/context-overflow.md`.

Pipeline determinista:

1. Leer archivos de `read`.
2. `rg` con keywords del `goal` y `required_behavior`.
3. Expandir vecinos por imports respetando `max_neighbor_hops`.
4. Buscar tests relacionados.
5. Excluir `vendor/`, `node_modules/`, `dist/`, `build/` y archivos en `forbid.files`.
6. Respetar `max_context_tokens` y `max_tokens_per_file`.

Si excede presupuesto: **generar `artifacts/context-overflow.md` con incluido/excluido/recomendación**. Nunca truncar silenciosamente.

Curaduría humana de `02-context.md`: opcional en `standard`, recomendada en `full`.

---

### Fase 4 — Decisión de ruteo (código, instantáneo)

**Consume**: `profile`, `risk`, `complexity`, `limits`, `review.required`.
**Produce**: ruta asignada y modelos seleccionados.

Rutas:

```
light:    context → executor L0 → verify → before_merge
standard: context → executor L0 → verify → reviewer L1 (si required) → before_merge
full:     context → architect L2 → after_plan gate → subtasks → executor → verify → reviewer → before_merge
```

---

### Fase 5 — Planificación arquitectónica (LLM nivel 2, solo perfil full)

**Consume**: `00-brief.md`, `01-contract.yaml`, `02-context.md`, `context-bundle.txt`, ADR relacionado si existe.
**Produce**: `03-plan.md` con análisis, decisiones, riesgos, sub-tareas con `depends_on`.

Las sub-tareas se extraen del bloque YAML de `03-plan.md`, se validan contra el schema, y se depositan en `tasks/inbox/`.

---

### Fase 6 — Compuerta humana del plan (humano, 1–3 min, solo perfil full)

**Consume**: `03-plan.md`.
**Produce**: aprobación, revisión (una sola revuelta) o aborto.

Sin esta compuerta el sistema puede ejecutar 30 sub-tareas en dirección equivocada.

---

### Fase 7 — Ejecución (LLM nivel 0)

**Consume**: `01-contract.yaml`, `context-bundle.txt`, `02-context.md`, `verify.commands`, `touch`, `forbid`, `limits`.
**Produce**: patch o edición directa, registrada en `04-implementation-log.md`.

Dos modos según `execution.mode`:

**Modo A — `patch_only`** (default). El agente devuelve solo un unified diff aplicable con `git apply`. Sin prosa. Mejor para control.

**Modo B — `worktree_edit`**. El agente edita archivos directamente en el worktree. El sistema inspecciona con `git diff`. Mejor para robustez (varios archivos, archivos nuevos, cuando Modo A falla repetidamente).

Si Modo A falla repetidamente, el sistema cae a `execution.fallback_mode`.

---

### Fase 8 — Aplicación y verificación (código, segundos a minutos)

**Consume**: patch generado o cambios en worktree.
**Produce**: `05-validation.md` con resultado de `verify.commands`.

Pipeline:

```bash
git apply --check patches/attempt-NN.patch   # solo Modo A
git apply patches/attempt-NN.patch           # solo Modo A
git diff --check                             # detecta whitespace
git diff --stat                              # forma del cambio
<verify.commands>                            # verdad funcional
```

Resolución:

- **Verde** → Fase 9.
- **Rojo** → reintento con verify log como contexto.
- `stop_on_same_error_twice` activo → corte temprano y escalada.
- Agotó retries → escala al siguiente nivel.

---

### Fase 9 — Validación de límites del contrato (código, instantáneo)

**Consume**: diff aplicado, `touch`, `forbid.files`, `limits`.
**Produce**: aprobación de límites o `artifacts/contract-violation.md`.

Validaciones (orden estricto):

1. Archivos modificados ⊆ `touch`.
2. Archivos modificados ∩ `forbid.files` = ∅.
3. Cantidad de archivos ≤ `limits.max_files_changed`.
4. Líneas de diff ≤ `limits.max_diff_lines`.
5. `git diff --check` pasa.

Si **alguna falla**, generar `artifacts/contract-violation.md` y bloquear commit. **Esta fase es distinta de Fase 8**: `verify` mide funcionalidad; esta fase mide alcance.

---

### Fase 10 — Review (LLM nivel 1, condicional)

**Consume**: `01-contract.yaml`, diff aplicado, `05-validation.md`.
**Produce**: `06-review.md` con JSON estructurado.

Activado por `review.required: true`, `risk: medium|high`, `profile: full`, `type: migration`, o paths sensibles.

Resolución:

- `approve` → Fase 12.
- `revise` → genera `fixes/FIX-NNN-<slug>.yaml` con scope acotado; vuelve a Fase 7 sobre la fix task. **No abrir un prompt grande**.
- `reject` → Fase 11 (escalamiento).

---

### Fase 11 — Escalamiento controlado (LLM nivel 2, condicional)

**Consume**: contrato, diff, validación, review fallido.
**Produce**: análisis + correcciones mínimas como `fixes/FIX-NNN.yaml`.

Activado solo si:

- `risk: high`, o
- Reviewer rechazó con `functional_risk: medium|high`, o
- L0 y L1 fallaron, o
- Hay ambigüedad arquitectónica.

**Compuerta humana antes de gastar L2** (recomendada u obligatoria según config).

L2 por defecto **no implementa**. Diagnostica y produce fix tasks. Implementa directo solo si el humano lo aprueba explícitamente.

---

### Fase 12 — Commit controlado (código)

**Consume**: estado verde de Fases 8, 9, 10.
**Produce**: commits en la rama del worktree.

Solo commitea si:

```
verify pasó
límites pasaron
review aprobó (si era requerido)
sin violaciones de contrato
```

Mensaje: `[<task_id>] <goal corto>`.

**El sistema puede commitear. El sistema nunca mergea a main.**

---

### Fase 13 — Compuerta humana de merge (humano, 1–5 min, siempre obligatoria)

**Consume**: rama `ai/<ID>` lista.
**Produce**: merge a main o abort.

Checklist mental:

```
[ ] Cumple goal.
[ ] Cumple acceptance.
[ ] verify.commands pasaron.
[ ] No toca archivos prohibidos.
[ ] No hay refactor no pedido.
[ ] Review aprobó si era requerido.
[ ] El cambio tiene sentido como producto.
[ ] Riesgos aceptados.
```

Merge según convención. `git worktree remove worktrees/<ID>`.

---

### Fase 14 — Promoción selectiva a memoria (humano, 30 s, opcional)

**Consume**: artefactos de la task completada.
**Produce**: entrada en `memory/decisions/`, `memory/patterns/` o `memory/lessons/`.

Comando: `agents task promote <ID> --type pattern|decision|lesson`.

**No automatizar.** El criterio humano de "esto vale la pena recordar" es lo que evita que la memoria se llene de ruido.

Se promueven: decisiones arquitectónicas, patrones reutilizables, restricciones importantes, bugs aprendidos, comandos de validación útiles, convenciones.

No se promueven: diffs completos, logs largos, detalles temporales, errores irrelevantes.

---

### Fase 15 — Limpieza (código)

**Consume**: estado final de la task.
**Produce**: archivado en `done/` o `failed/`.

```bash
agents task clean <ID>
```

Si falló, se anexa `FAILURE.md` con motivo, intentos consumidos, modelos invocados, costo total y recomendación accionable.

---

## 7. Schemas de artefactos clave

### `01-contract.yaml`

```yaml
task_id: FEAT-001
type: feature                  # bugfix | refactor | feature | test | migration | doc
profile: standard              # light | standard | full
risk: medium                   # low | medium | high
complexity: atomic             # atomic | compound

goal: >
  Implement quick payment method selection in POS Express sale screen.

required_behavior:
  - "CASH is the default payment method."
  - "User can select QR or CARD."
  - "Selected method is included in the sale payload as enum, not UI label."

acceptance:                    # criterios declarativos
  - "Default payment method is CASH."
  - "QR and CARD can be selected."
  - "Payload contains internal enum value."
  - "No backend changes."

verify:                        # comandos ejecutables
  commands:
    - "yarn test --findRelatedTests src/features/sales"
    - "yarn lint --quiet"

read:
  - src/features/pos-express/PosExpressSale.tsx
  - src/features/sales/createSalePayload.ts
  - src/features/sales/__tests__/createSalePayload.test.ts

touch:
  - src/features/pos-express/**
  - src/features/sales/createSalePayload.ts
  - src/features/sales/__tests__/createSalePayload.test.ts

forbid:
  files:                       # enforcement automático por glob
    - package.json
    - "*.lock"
    - src/features/auth/**
    - src/features/permissions/**
    - src/features/cashbox/**
  behaviors:                   # enforcement semántico, instrucción al modelo
    - "Do not introduce new runtime dependencies."
    - "Do not send UI labels to backend."
    - "Do not refactor unrelated code."

limits:
  max_files_changed: 6
  max_diff_lines: 500
  max_context_tokens: 20000
  max_tokens_per_file: 4000
  max_neighbor_hops: 1

execution:
  mode: patch_only             # patch_only | worktree_edit
  fallback_mode: worktree_edit
  max_attempts_per_mode: 2
  allow_new_files: true

retry_policy:
  level0_max_retries: 2
  level1_max_retries: 1
  level2_max_retries: 1
  stop_on_same_error_twice: true

review:
  required: true
  reason: "Feature touches sale payload behavior."

human_gates:                   # checkpoints obligatorios declarativos
  - before_merge               # siempre
  # - after_plan               # solo profile=full
  # - before_level2_escalation # opcional, recomendado para high risk

promote_to_memory: false       # se decide en Fase 14

# Solo profile=full:
# subtasks: [...] auto-generadas por el arquitecto, con depends_on.
```

### `07-trace.json`

```json
{
  "task_id": "FEAT-001",
  "profile": "standard",
  "risk": "medium",
  "complexity": "atomic",
  "execution_mode": "patch_only",
  "started_at": "2026-04-27T10:00:00Z",
  "completed_at": "2026-04-27T10:42:00Z",
  "outcome": "merged",
  "human_minutes": 14,
  "attempts": [
    {
      "phase": "execute",
      "level": 0,
      "model": "minimax/minimax-m2.7",
      "tokens_in": 8420,
      "tokens_out": 1130,
      "cost_usd": 0.022,
      "duration_s": 18,
      "result": "verify_failed",
      "failure_reason": "default method is not CASH",
      "stop_early": false
    },
    {
      "phase": "execute",
      "level": 0,
      "model": "minimax/minimax-m2.7",
      "tokens_in": 9100,
      "tokens_out": 1180,
      "cost_usd": 0.024,
      "duration_s": 20,
      "result": "verify_passed"
    },
    {
      "phase": "validate_limits",
      "result": "passed"
    },
    {
      "phase": "review",
      "level": 1,
      "model": "openai/gpt-5-mini",
      "tokens_in": 3200,
      "tokens_out": 280,
      "cost_usd": 0.018,
      "duration_s": 7,
      "result": "approve"
    }
  ],
  "total_cost_usd": 0.064,
  "violations": [],
  "context_overflows": []
}
```

### `06-review.md` (output JSON forzado)

```json
{
  "verdict": "approve",
  "contract_compliance": true,
  "forbidden_files_touched": [],
  "unrequested_refactor": false,
  "missing_tests": [],
  "functional_risk": "low",
  "notes": ["The diff complies with touch/forbid and verify passed."]
}
```

### Entrada de memoria (`memory/patterns/PAT-NNN.yaml`)

```yaml
id: PAT-2026-04-27-001
source_task: FEAT-001
type: pattern                  # pattern | decision | lesson
title: "Payment method as internal enum, not UI label"
context: "POS Express sale flow"
content: >
  Use enum CASH | QR | CARD internally. UI labels are presentation only
  and must never be sent to backend.
related:
  - ADR-0007
created_at: "2026-04-27"
```

---

## 8. API CLI objetivo

Diseño completo. **MVP solo implementa los marcados con `[MVP]`.**

```bash
agents task create <ID>            # genera plantillas en inbox/
agents task start <ID>             # [MVP] valida, crea worktree, inicia trace
agents task run <ID>               # [MVP] ejecuta fases según perfil
agents task status <ID>            # [MVP] muestra fase actual y costos
agents task review <ID>            # forza review manual fuera de pipeline
agents task approve-plan <ID>      # libera la compuerta de Fase 6
agents task escalate <ID>          # forza escalamiento a nivel superior
agents task promote <ID> --type T  # promueve a memoria
agents task clean <ID>             # [MVP] cierra worktree y archiva
agents task abort <ID>             # cancelación manual a failed/

agents stats --since 30d           # costos agregados, tasa de éxito por nivel
agents config show routing         # muestra políticas activas
agents config validate             # valida schemas y configs
```

---

## 9. Configuración de routing y modelos

`.agents/config.yaml`:

```yaml
levels:
  0:
    use_for: [atomic_low, atomic_medium, tests, docs, mechanical_refactor]
    primary: minimax/minimax-m2.7
    fallback: deepseek/deepseek-v3.2
    secondary_fallback: qwen/qwen3-coder
    max_cost_per_call_usd: 0.15

  1:
    use_for: [review, diagnose_verify_failure, small_corrections]
    primary: openai/gpt-5-mini
    fallback: anthropic/claude-haiku-4.5
    max_cost_per_call_usd: 0.50

  2:
    use_for: [compound_planning, high_risk_arch, ambiguous_bugs, migrations, security]
    primary: anthropic/claude-opus-4.7
    fallback: openai/gpt-5
    max_cost_per_call_usd: 3.00
    requires_human_gate: true
```

Regla:

```
El modelo no decide cuánto gastar.
El dispatcher decide cuánto gastar.
El humano aprueba escalamientos a L2.
```

---

## 10. Resumen de intervenciones humanas

| Fase | Acción | Tiempo | Obligatoria |
|------|--------|--------|-------------|
| 0 | Decisión previa | 30 s – 2 min | Sí |
| 1 | Brief + contrato | 2 – 8 min | Sí |
| 3 | Curaduría de contexto | 2 – 10 min | Solo full |
| 6 | Aprobar plan | 1 – 3 min | Solo full |
| 11 | Aprobar escalada a L2 | 30 s | Recomendada |
| 13 | Merge final | 1 – 5 min | Sí |
| 14 | Promoción a memoria | 30 s | Opcional |

**Tiempo total humano**:

- **light**: ~5 min/task
- **standard**: ~15 min/task
- **full**: ~25–40 min/task

El resto es código y LLMs ejecutando bajo contrato.

---

## 11. Plan incremental con objetivos cuantitativos

### Semana 1 — Camino `light` funcional

**Implementar**:

```
- contract.schema.json (subset light)
- validación de schema
- creación de worktree
- context-bundle desde read + rg
- executor L0 en modo patch_only
- git apply --check
- verify.commands
- validación de touch/forbid
- 07-trace.json mínimo (modelo, tokens, costo, resultado)
- agents task: start, run, status, clean
```

**Objetivo medible**: ejecutar **5 features atómicos reales** con `outcome: merged` en `07-trace.json`. Métrica: costo total ≤ $1, tiempo humano ≤ 30 minutos acumulados.

---

### Semana 2 — Camino `standard`

**Agregar**:

```
- 00-brief.md y 02-context.md
- curaduría humana opcional
- reviewer L1 con output JSON
- retry_policy completo
- fixes/FIX-NNN.yaml automáticos
```

**Objetivo medible**: ejecutar **10 tareas standard** con trace completo. Métricas: tasa de éxito L0 first-try ≥ 60%, costo promedio por task ≤ $0.20.

---

### Semana 3 — Robustez

**Agregar**:

```
- execution.mode = patch_only | worktree_edit con fallback
- artifacts/context-overflow.md
- artifacts/contract-violation.md
- stop_on_same_error_twice
- agents stats --since 30d con métricas agregadas
```

**Objetivo medible**: cero contextos truncados silenciosamente. Tasa de violación de contrato detectada antes de commit ≥ 95%.

---

### Semana 4 — Camino `full`

**Agregar**:

```
- architect L2 con 03-plan.md
- subtareas con depends_on
- after_plan gate
- ADR linking
- escalamiento controlado a L2
```

**Objetivo medible**: ejecutar **3 features compound + medium/high risk** end-to-end. Métrica: costo total ≤ $15, tiempo humano por task ≤ 40 min.

---

### Semana 5+ — Memoria y aprendizaje

**Agregar**:

```
- promote_to_memory con schema
- indexación semántica de patterns/decisions/lessons
- ajuste de routing basado en métricas reales del trace
- detección de tipos de task que desperdician presupuesto
```

**Objetivo medible**: reducir costo promedio por tipo de task ≥ 20% mediante reasignación de niveles basada en datos.

---

## 12. Siguiente paso: SPECs implementables

El flujo conceptual termina acá. **No producir más versiones narrativas.** Lo que sigue son contratos técnicos implementables:

| SPEC | Contenido | Bloquea |
|------|-----------|---------|
| **SPEC-001** | `contract.schema.json` con validación completa, campos opcionales por perfil | Todo |
| **SPEC-002** | Task lifecycle: estados, transiciones, eventos, archivos esperados por estado | Orquestador |
| **SPEC-003** | CLI commands MVP: signatures, exit codes, formato de output | UX del operador |
| **SPEC-004** | `trace.schema.json` con todos los campos obligatorios y opcionales | Métricas y observabilidad |
| **SPEC-005** | Prompt templates: `executor/feature.md`, `reviewer/default.md`, `architect/default.md` con placeholders y output esperado | Calidad de los agentes |
| **SPEC-006** | Retriever: algoritmo de selección, presupuesto, criterios de overflow | Calidad del contexto |

**Orden recomendado de redacción**: SPEC-001 → SPEC-004 → SPEC-005 → SPEC-006 → SPEC-002 → SPEC-003.

El schema del contrato es el cimiento; sin él, todo lo demás es teoría. El trace schema le sigue porque es lo que valida que el sistema funciona económicamente. Las plantillas de prompts vienen después porque son lo que más vas a iterar con datos reales.

---

## 13. Principio operativo final

```
Lo que ve el humano puede ser markdown rico.
Lo que ve el modelo debe ser estructura mínima.
Lo que decide el flujo debe ser código determinista.
Lo que valida el resultado deben ser tests o checks.
Lo que optimiza el costo debe medirse desde el día uno.
```

El sistema completo se reduce a este mapa:

```
01-contract.yaml         contrato máquina
00-brief.md              contrato humano
worktree                 sandbox aislado
retriever determinista   proveedor de contexto
context-overflow.md      alarma anti-truncado
dispatcher               control de costo y riesgo
executor (modo A o B)    fuerza de implementación
verify.commands          verdad funcional
contract-violation.md    red de scope
reviewer JSON            control de calidad
07-trace.json            verdad económica
human gates              autoridad de dirección
memoria selectiva        aprendizaje sin ruido
```

Listo para traducir a SPECs y luego a código.