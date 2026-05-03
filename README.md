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

- CLI `quorum task ...` para inicializar, listar, activar, crear worktrees, consultar estado y limpiar tareas.
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

## 🚀 Inicio Rápido

Quorum se instala como una herramienta global mediante `uv` para que puedas usarlo en cualquier proyecto de forma aislada.

### 1. Instalación Global

Clona el repositorio y utiliza `uv tool install`:

```bash
git clone https://github.com/usuario/quorum.git
cd quorum
uv tool install --editable .
```

Esto registrará el comando `quorum` en tu PATH. Al ser una instalación `--editable`, cualquier mejora que descargues o hagas en el código de Quorum se reflejará instantáneamente sin necesidad de re-instalar.

### 2. Inicializar un Proyecto

Ve a tu proyecto de software (ej. `hsme`) y prepara la estructura de Quorum:

```bash
cd /ruta/a/tu/proyecto
quorum init
```

Esto creará automáticamente:
- Directorios de tareas: `.ai/tasks/{inbox,active,done,failed}`.
- Directorios de memoria curada: `memory/{decisions,patterns,lessons}`.
- Configuración de `.gitignore` para proteger tus worktrees y artefactos temporales.

---

## 🛠 Flujo de Trabajo Operativo

### 1. Especificación (El "Qué")

```bash
quorum task specify FEAT-001
```
Esto crea `00-spec.yaml`. Usa el skill `/q-brief FEAT-001` para definir invariantes y criterios de aceptación.

### 2. Blueprint y Contrato (El "Cómo")

```bash
quorum task blueprint FEAT-001
```
Mueve la tarea a `active/`. Usa `/q-blueprint FEAT-001` para que el agente explore el código y genere el `02-contract.yaml`.

### 3. Aislamiento y Ejecución

```bash
# Crea un Git Worktree aislado
quorum task start FEAT-001
```

Luego invoca los agentes de ejecución:
- `/q-implement FEAT-001`: Implementa cambios respetando el contrato.
- `/q-verify FEAT-001`: Ejecuta comandos de validación y genera `05-validation.json`.
- `/q-review FEAT-001`: Genera el veredicto `06-review.json`.

### 4. Merge Humano y Memoria

Cuando la revisión sea positiva (`approve`):
1. El humano inspecciona el diff en el worktree.
2. El humano mergea la rama `ai/FEAT-001` a `main`.
3. Limpieza y captura de conocimiento:

```bash
# Archiva la tarea y elimina el worktree
quorum task clean FEAT-001

# Captura patrones y lecciones durables
/q-memory FEAT-001
```

---

## 🧪 Política de testing

```text
Agent loop:  unit tests + lint       objetivo: <60s
Human gate:  BDD / acceptance suite  objetivo: <10min
```

- El agente ejecuta solo `verify.commands` definidos en el contrato.
- El humano ejecuta la suite de aceptación completa antes del merge.
- Ningún reporte reemplaza la validación determinística.

---

Para validar el propio framework:

```bash
uv run pytest -v
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
│   ├── config.yaml        # niveles de modelos y límites
│   └── skills/            # skills q-* y spec-kitty.*
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
