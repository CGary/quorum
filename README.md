# Quorum ⚖️

**Constraints in. Verified diffs out. Costs bounded. Humans only where humans matter.**

Quorum es un framework **AI-first** para ejecutar funcionalidades complejas mediante contratos verificables. Convierte una intención humana en artefactos machine-first (`00` → `07`), limita el contexto que recibe cada agente y exige que el resultado se pruebe con comandos reales antes de revisión humana.

> Estado actual: **MVP de orquestación y artefactos**. Quorum ya incluye schemas, skills, CLI de tareas, worktrees aislados, políticas de riesgo/routing, scoring de riesgo y lookup de fallos relacionados. El dispatcher/runtime automático completo todavía está diferido: `task_manager.run_task()` sigue siendo stub.

---

## 🧠 Filosofía

- **Spec-Driven Contracts (SDC):** el flujo lógico es Spec → Blueprint → Contract → Verified Diff.
- **Machine-first, formato según audiencia:** YAML para planificación; JSON para captura del sistema.
- **Contexto determinista:** el agente no recibe “todo el proyecto”; recibe archivos y restricciones derivados del blueprint.
- **Ejecución aislada:** cada tarea corre en su propio Git worktree y rama `ai/<TASK_ID>`.
- **Merge humano:** Quorum puede preparar y commitear trabajo en rama, pero nunca mergea a `main`.
- **Memoria curada:** `memory/*.json` guarda conocimiento durable solo cuando `q-memory` se invoca explícitamente.

### Lo que Quorum NO es

- No es un chatbot general del repo.
- No es una herramienta para cambios triviales de 5 líneas.
- No es un generador de documentación narrativa.
- No es un sistema de merge automático.
- No depende de HSME/vector DBs: sistemas externos pueden consumir `memory/*.json`, pero Quorum es local-first.

---

## ✅ Estado actual del proyecto

### Implementado

- CLI `./agents task ...` para inicializar, listar, activar, crear worktrees, consultar estado y limpiar tareas.
- Schemas JSON para artefactos:
  - `spec`, `blueprint`, `contract`, `validation`, `review`, `trace`, `memory`.
- Skills operativos:
  - `q-brief`, `q-blueprint`, `q-analyze`, `q-implement`, `q-verify`, `q-review`, `q-accept`, `q-memory`, `q-status`.
- Worktrees por tarea en `worktrees/<TASK_ID>/`.
- Políticas de riesgo/routing en `.agents/policies/`.
- `risk_scorer.py` para sugerir riesgo desde `01-blueprint.yaml`.
- `failure_lookup.py` para consultar tareas fallidas relacionadas durante blueprint.
- `05-validation.json.error_category` opcional:
  - `logic | dependency | environment | flaky | unknown`.
- Gobernanza documentada para evitar propuestas duplicadas:
  - memoria curada,
  - routing/risk,
  - failure handling,
  - concurrency/merge-gate,
  - límite de artefactos `00`-`07`.

### Diferido / no implementado aún

- Dispatcher automático de ejecución.
- `task_manager.run_task()` real.
- Auto-retry y re-blueprint automático tras fallo.
- Renegociación automática de contrato.
- Shadow merge / pre-merge gate automático.
- Auto-rebase.
- Nuevos artefactos `08-post-mortem.json` o `09/10-impact-report.json` — rechazados por duplicar `05/06/07` y `q-memory`.

---

## 📜 Constitución: reglas inmutables

1. **Git es la verdad del código.** La memoria semántica es para patrones; Git es para código.
2. **Contexto determinista.** Los agentes reciben contexto derivado del blueprint, no el repo completo.
3. **Sin parches fuera del contrato.** Tocar archivos fuera de `02-contract.yaml.touch` rechaza la tarea.
4. **La validación es la finalidad.** Nada está terminado hasta que `verify.commands` pase.
5. **Artefactos machine-first.** YAML/JSON para operación; Markdown solo para docs/ADR.
6. **El sistema commitea, nunca mergea.** El merge a `main` es humano.
7. **El costo está limitado por política.** Routing/retries/escalaciones son política, no confianza.
8. **Los tests son la única prueba.** Specs y blueprints no prueban funcionalidad.

---

## 📂 Artefactos canónicos

Quorum usa `00` a `07` más memoria curada. No se agregan slots nuevos sin ADR, schema y consumidor determinístico.

| Archivo | Formato | Quién lo produce | Propósito |
|---|---|---|---|
| `00-spec.yaml` | YAML | Humano + `q-brief` | Qué se quiere lograr, invariantes y aceptación. |
| `01-blueprint.yaml` | YAML | `q-blueprint` | Ruta técnica: archivos, símbolos, dependencias, estrategia. |
| `02-contract.yaml` | YAML | `q-blueprint` / Gatekeeper | Qué puede tocar el agente, qué no, comandos de verificación y límites. |
| `04-implementation-log.yaml` | YAML | `q-implement` | Cambios realizados, blockers e intentos. |
| `05-validation.json` | JSON | `q-verify` | Comandos ejecutados, exit codes, output y resultado global. |
| `06-review.json` | JSON | `q-review` | Revisión del diff contra contrato y validación. |
| `07-trace.json` | JSON | Sistema/skills | Intentos, coste, fases, violaciones y resultado. |
| `memory/*.json` | JSON | `q-memory` | Decisiones, patrones y lecciones durables. |

### Boundary de artefactos

- No crear `08-post-mortem.json`: los datos del fallo viven en `05`, `06`, `07` y `memory/lessons`.
- No crear `09/10-impact-report.json`: el aprendizaje exitoso va directo a `q-memory`.
- Routing, merge-gate y eventos operativos deben registrarse en `07-trace.json` salvo ADR que justifique otra cosa.

---

## 🚀 Cómo usar Quorum en un proyecto de software

> Suposición: estás en la raíz del repo de software y Quorum ya está instalado/configurado ahí con `./agents`, `.agents/`, `skills/`, `.ai/` y `memory/`.

### 0. Preparar el entorno

```bash
uv sync
chmod +x agents
./agents task list
```

Verifica que existan estos directorios:

```bash
.ai/tasks/inbox
.ai/tasks/active
.ai/tasks/done
.ai/tasks/failed
worktrees
memory
```

---

### 1. Crear la especificación

```bash
./agents task specify FEAT-001
```

Esto crea una carpeta en `.ai/tasks/inbox/` con `00-spec.yaml` inicial.

Luego invoca el skill:

```text
/q-brief FEAT-001
```

Objetivo: completar `00-spec.yaml` con:

- `task_id`
- `summary`
- `goal`
- `invariants`
- `acceptance`
- `risk` si el humano ya lo sabe
- restricciones o non-goals si aplica

Regla práctica: si no puedes escribir invariantes verificables, la tarea aún no está lista para blueprint.

---

### 2. Generar blueprint y contrato

```bash
./agents task blueprint FEAT-001
```

Esto mueve la tarea a `.ai/tasks/active/`.

Luego invoca:

```text
/q-blueprint FEAT-001
```

`q-blueprint` debe producir:

- `01-blueprint.yaml`
- `02-contract.yaml`

Durante esta fase Quorum puede:

- calcular riesgo sugerido con `risk_scorer.py`;
- consultar `.ai/tasks/failed/` con `failure_lookup.py` para detectar fallos relacionados;
- registrar divergencias de riesgo en `07-trace.json` cuando aplique.

---

### 3. Analizar consistencia antes de ejecutar

Opcional pero recomendado:

```text
/q-analyze FEAT-001
```

Este skill revisa coherencia entre:

- `00-spec.yaml`
- `01-blueprint.yaml`
- `02-contract.yaml`

Busca gaps como:

- archivos necesarios ausentes del contrato;
- escenarios de test débiles;
- invariantes no cubiertas;
- límites demasiado amplios o demasiado estrechos.

---

### 4. Crear worktree aislado

```bash
./agents task start FEAT-001
```

Esto crea:

```text
worktrees/FEAT-001/
branch: ai/FEAT-001
```

El agente implementador debe trabajar dentro de ese worktree, no en el checkout principal.

---

### 5. Implementar siguiendo el contrato

Invoca:

```text
/q-implement FEAT-001
```

Reglas del implementador:

- Solo puede modificar archivos listados en `02-contract.yaml.touch`.
- Debe respetar `forbid.files` y `forbid.behaviors`.
- Si el contrato está incompleto, debe bloquear, no improvisar.
- Si necesita tocar un archivo fuera del contrato, reporta `BLOCKED` para futura renegociación manual o automatizada.

Salida esperada:

- cambios en el worktree;
- `04-implementation-log.yaml` actualizado;
- commit en rama de tarea cuando el flujo lo permita.

---

### 6. Verificar funcionalmente

```text
/q-verify FEAT-001
```

Este skill ejecuta `02-contract.yaml.verify.commands` dentro del worktree y escribe `05-validation.json`.

Resultados posibles:

- `passed`
- `failed`
- `blocked`

Si falla, `q-verify` puede añadir:

```json
"error_category": "logic|dependency|environment|flaky|unknown"
```

Esto permite distinguir bugs reales de fallos transitorios o de entorno.

---

### 7. Revisar el diff

```text
/q-review FEAT-001
```

El reviewer produce `06-review.json` con:

- `verdict`: `approve | revise | reject`
- cumplimiento del contrato;
- archivos prohibidos tocados;
- tests faltantes;
- riesgo funcional;
- `fix_tasks` si hay que iterar.

Si el verdict es `revise`, vuelve a:

```text
/q-implement → /q-verify → /q-review
```

---

### 8. Acceptance y merge humano

Cuando `q-review` aprueba:

1. El humano inspecciona el diff.
2. El humano ejecuta la suite lenta si existe:

```bash
# ejemplo; depende del proyecto
pytest tests/bdd -v
```

3. El humano mergea la rama `ai/FEAT-001` a `main`.

Quorum no hace merge automático.

---

### 9. Cerrar tarea y capturar memoria

Después del merge humano:

```bash
./agents task clean FEAT-001
```

Esto remueve el worktree y archiva la tarea en `done/`.

Luego, si hay aprendizaje durable:

```text
/q-memory FEAT-001
```

Captura como máximo conocimiento útil en:

- `memory/decisions/`
- `memory/patterns/`
- `memory/lessons/`

No uses memoria como log. Para logs ya existe `07-trace.json`.

---

### 10. Consultar estado

```bash
./agents task status FEAT-001
./agents task list
```

También puedes usar:

```text
/q-status FEAT-001
```

---

## 🧪 Política de testing

```text
Agent loop:  unit tests + lint       objetivo: <60s
Human gate:  BDD / acceptance suite  objetivo: <10min
```

- El agente ejecuta solo `verify.commands`.
- El humano ejecuta aceptación lenta antes del merge.
- Ningún reporte reemplaza tests verdes.

Para validar el propio framework:

```bash
uv run pytest -v
```

---

## 🛠 Instalación / adopción en un repo

Para trabajar dentro de este repo:

```bash
git clone <repo>
cd quorum
uv sync
chmod +x agents
uv run pytest -v
```

Para adoptar Quorum en otro proyecto, copia/adapta:

```text
agents
.agents/
skills/
.ai/tasks/{inbox,active,done,failed}/.gitkeep
memory/{decisions,patterns,lessons}/.gitkeep
```

Y agrega a `.gitignore`:

```gitignore
worktrees/
.ai/tasks/active/*
.ai/tasks/done/*
.ai/tasks/failed/*
.ai/tasks/inbox/*
!.ai/tasks/active/.gitkeep
!.ai/tasks/done/.gitkeep
!.ai/tasks/failed/.gitkeep
!.ai/tasks/inbox/.gitkeep
```

---

## 📂 Estructura del sistema

```bash
project/
├── agents                 # wrapper CLI; configura PYTHONPATH=.agents
├── .agents/
│   ├── cli/               # CLI y helpers core
│   │   └── core/
│   │       ├── task_manager.py
│   │       ├── risk_scorer.py
│   │       └── failure_lookup.py
│   ├── schemas/           # JSON Schemas para YAML/JSON artifacts
│   ├── policies/          # risk.yaml y routing.yaml
│   ├── retrievers/        # import graph / AST neighbors
│   └── config.yaml        # niveles de modelos y límites
├── skills/                # skills q-*
├── .ai/tasks/
│   ├── inbox/
│   ├── active/
│   ├── done/
│   └── failed/
├── docs/adr/              # decisiones arquitectónicas
├── memory/
│   ├── decisions/
│   ├── patterns/
│   └── lessons/
└── worktrees/             # worktrees gitignored por tarea
```

---

## 🧭 Roadmap resumido

Prioridad antes de automatización avanzada:

1. Convertir `task_manager.run_task()` en runtime real.
2. Consolidar escritura consistente de `07-trace.json` durante ejecución.
3. Añadir flujo explícito de review/pre-merge en CLI.
4. Implementar merge-gate determinístico mediante shadow merge + `verify.commands`.
5. Solo con telemetría: evaluar auto-retry, renegociación de contrato o re-blueprint automático.

Rechazado por arquitectura actual:

- post-mortem dedicado `08`;
- impact report `09/10`;
- agente integrador LLM para resolver Git;
- memoria automática sin `q-memory`;
- merge automático a `main`.

---

## ⚖️ Licencia

MIT
