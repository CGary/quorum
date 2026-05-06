# Quorum ⚖️ — The AI-First Orchestration Manifesto v1.1

**Constraints in. Verified diffs out. Costs bounded. Humans only where humans matter.**

No hedging tokens. No filler. Direct constraints. Expected outcomes.

Quorum is NOT a documentation manager. It is a **State-Driven Execution Engine**. It treats AI agents as focused engineering units that consume structured constraints and produce verified code.

---

## 🧠 Core Philosophy: "Agents Process Invariants, Not Stories"

Humans think in stories; agents think in constraints. Quorum eliminates the human-to-agent translation tax by replacing prose-heavy briefs with machine-readable artifacts.

### What Quorum IS

- **Surgical**: It targets specific files and symbols. No project-wide context dumping.
- **Contractual**: The agent is bound by a strict YAML contract.
- **Traceable**: Every token, retry, validation result, and cent is logged for economic audit.
- **Decoupled**: The codebase remains pure. Quorum lives in `.ai/` and `.agents/`; source code does not know Quorum exists.
- **Feature-Oriented**: It is built for complex features where structure pays back.

### What Quorum IS NOT

- **A General Assistant**: It does not chat about the project. It executes missions.
- **A Documentation Tool**: It does not generate stakeholder reports. It generates specs, blueprints, contracts, validation, review, and trace artifacts.
- **A Human-Centric UI**: Operational artifacts are machine-first; humans inspect YAML planning files directly when needed.
- **A Triage Tool for Trivial Changes**: Quorum is built for complex features where structure pays back. Bugfixes, typos, and 5-line edits are out of scope. Use direct CLI tools for those.

---

## 🛠 Sources of Truth

| Question | Source of Truth | Format | Authority |
| :--- | :--- | :--- | :--- |
| **What** do we want? | Specification | `00-spec.yaml` | Logical Architect |
| **How** will we do it? | Blueprint | `01-blueprint.yaml` | Surgical Cartographer |
| **What** can we touch? | Contract | `02-contract.yaml` | The Gatekeeper |
| **What** was done? | Implementation Log | `04-implementation-log.yaml` validated by `implementation-log.schema.json` | Surgical Executor |
| **Did** it work? | Tests / verify | `verify.commands` + `05-validation.json` | Functional Verifier |
| **What** changed? | Git | Repo + worktrees | Code Truth |
| **What** did it cost? | Trace | `07-trace.json` | Economic Verifier |

---

## 📐 Schema Discipline

### Format by audience

| Artifact | Format | Writer | Reader | Change frequency |
| :--- | :--- | :--- | :--- | :--- |
| `00-spec.yaml` | YAML | Human + Logical Architect | Human + downstream agents | Low |
| `01-blueprint.yaml` | YAML | Surgical Cartographer | Human + Executor | Low |
| `02-contract.yaml` | YAML | Derived from Blueprint | Executor + system | Low |
| `04-implementation-log.yaml` | YAML | System, manual append | Human + reviewer | Append per commit |
| `05-validation.json` | JSON | System stdout capture | System + reviewer | Write-only |
| `06-review.json` | JSON | Reviewer agent | System + human | Write-only |
| `07-trace.json` | JSON | System append-only | System + dashboards | Continuous append |
| `memory/*.json` | JSON | System + agents | System + semantic tools | Append per task |

YAML is used for planning artifacts because humans and designers inspect them directly and because they are repeatedly injected into agent context. JSON is used for system-captured artifacts because capture, dashboards, and observability tools need rigid parsing.

Both formats validate against JSON Schema. YAML parsers produce native structures that are validated against the same schema definitions as JSON.

### Flat schemas

Schemas MUST stay shallow. Maximum intended nesting depth is three levels. Prefer lists of objects over deeply nested objects. A YAML artifact must be readable without invoking an LLM.

### `summary` convention

Every task artifact MUST include `summary` as the second document key after `task_id`. `summary` is factual, dense, and no longer than 200 characters. It powers task listings, efficient context injection, and human triage.

Example:

```yaml
task_id: FEAT-001
summary: Add internal payment-method enum (CASH|QR|CARD) to POS Express sale flow. Touches 3 files. Risk medium.
```

### Artifact lifecycle boundary

The canonical task lifecycle is `00` through `07` plus curated `memory/*.json`. New numbered lifecycle artifacts are rejected by default unless they satisfy all of these conditions:

1. The information cannot live in an existing artifact without duplication.
2. A runtime or skill consumes the artifact deterministically.
3. The schema is defined before producers write it.
4. The artifact does not bypass `q-memory` as the human curation gate.

Current reserved meanings:

| Slot | Status | Guidance |
| :--- | :--- | :--- |
| `08-post-mortem.json` | Rejected | Failure data already lives in `05-validation.json`, `06-review.json`, `07-trace.json`, and `memory/lessons/`. |
| `09/10-impact-report.json` | Rejected | Successful learning should go directly through `q-memory` into curated `memory/*`; no intermediate report is needed. |
| Additional integration/routing artifacts | Rejected by default | Use `07-trace.json` events or existing contract fields unless a future ADR proves a separate artifact is necessary. |

This prevents artifact sprawl: observability goes to trace, validation evidence goes to validation/review, durable learning goes to memory.

---

## 🚀 The AI-First Lifecycle (SDC: Spec-Driven Contracts)

### Phase 1: Specify (Logic Extraction)

- **Actor**: `q-brief` (Logical Architect).
- **Goal**: Convert human intent into a logical invariant map.
- **Output**: `00-spec.yaml`.
- **Logic**: Identify what must be true, what must not change, and success criteria. No code paths yet.
- **Forward auto-transition**: on success, runs `quorum task blueprint <TASK_ID>` (inbox → active).

### Phase 1.5: Decompose (Optional, large features only)

- **Actor**: `q-decompose` (Decomposer).
- **Goal**: When the spec describes a feature too large for one implementation pass, split it into N child tasks (`FEAT-001-a`, `-b`, ...) that each go through their own full lifecycle.
- **Output**: `decomposition: [...]` field appended to the parent's `00-spec.yaml`; child specs materialised in `inbox/`.
- **Logic**: Apply the heuristic in `.agents/policies/decomposition.yaml`. Propose a split, ask the human to confirm, persist, and auto-run `quorum task split <PARENT_ID>`.
- **Forward auto-transition**: on confirmation, runs `quorum task split <PARENT_ID>`.
- **When to skip**: the heuristic raises zero signals or the human declines. The flow continues as a single task.

### Phase 2: Blueprint + Contract (Surgical Cartography)

- **Actor**: `q-blueprint` (Surgical Cartographer).
- **Goal**: Explore the codebase and design the surgical path.
- **Output**: `01-blueprint.yaml` + `02-contract.yaml` + risk events appended to `07-trace.json`.
- **Logic**: Map affected files, symbols, dependencies, existing tests, and required new scenarios.
- **Forward auto-transition**: on success, runs `quorum task start <TASK_ID>` (creates worktree + branch).

### Phase 2.5: Analyze (Optional Consistency Gate)

- **Actor**: `q-analyze` (Artifact Consistency Analyst).
- **Goal**: Verify that `00-spec.yaml`, `01-blueprint.yaml`, and `02-contract.yaml` agree before implementation.
- **Output**: Read-only report in the agent response (no persisted artifact).
- **Logic**: Detect missing tests, contract/blueprint mismatches, slow BDD commands placed in `verify.commands`, and risk/trace drift.
- **Forward auto-transition**: none.

### Phase 3: Implement (Surgical Implementation)

- **Actor**: `q-implement` (Surgical Executor).
- **Goal**: Implement exactly what `02-contract.yaml` authorizes.
- **Output**: Commit(s) on `ai/<TASK_ID>` inside `worktrees/<TASK_ID>/` and `04-implementation-log.yaml`.
- **Logic**: Operate only inside the worktree, touch only contract-authorized paths, and stop without running `verify.commands`.
- **Authorized Retry**: Para una **hija fallida**, `/q-implement` puede ser autorizado por un ADR (ej. ADR 0001) para reintentar una implementación previa. Este retry debe ser iniciado por el despachador/orquestador, preservar `07-trace.json` como append-only, y nunca auto-mergear ni auto-rollbackear. Respeta la **Regla #7**.
- **Forward auto-transition**: none.

### Phase 4: Verify (Functional Verification)

- **Actor**: `q-verify` (Functional Verifier).
- **Goal**: Execute the contract's fast `verify.commands`.
- **Output**: `05-validation.json`.
- **Logic**: Capture commands, exit codes, duration, output excerpts, and `error_category` when failures occur. Do not edit code.
- **Forward auto-transition**: none.

### Phase 5: Review (Contract Compliance)

- **Actor**: `q-review` (Contract Reviewer).
- **Goal**: Review the diff against the spec, blueprint, contract, and validation evidence.
- **Output**: `06-review.json`.
- **Logic**: Approve only when validation passed and the diff stays inside the contract.
- **Forward auto-transition**: none.

### Phase 6: Accept + Human Merge Gate

- **Actor**: `q-accept` (Merge Gatekeeper) + human.
- **Goal**: Decide whether the task is ready for human BDD, inspection, and merge.
- **Output**: `ready|not_ready` verdict in the agent response; human merge decision.
- **Logic**: The system commits agent work. The human runs BDD acceptance (if defined) and performs the merge. The CLI cleanup happens after merge.
- **Forward auto-transition**: none.

### Phase 7: Memory Capture (Optional)

- **Actor**: `q-memory` (Learning Curator).
- **Goal**: Preserve durable decisions, patterns, or lessons after merge or meaningful failure.
- **Output**: Curated entries under `memory/{decisions,patterns,lessons}/`.
- **Logic**: Human-invoked only; no automatic ingestion.
- **Forward auto-transition**: none.

---

## 🔒 Skill Modularity: Single-Phase Skills with Forward Auto-Transition

Cada `/q-*` skill es una **unidad atómica de una sola fase**. Las fases del ciclo (Specify, Decompose, Blueprint, Analyze, Implement, Verify, Review, Accept, Memorize) **no** son una cadena que un mismo agente recorre de punta a punta. Son despachos independientes hechos por el orquestador.

### Regla base

Un skill EJECUTA solo su fase declarada y para. NO activa al siguiente skill, NO llama a otro skill por cuenta del usuario, NO sigue después de su frontera aunque el usuario haya aceptado la salida.

### Excepción acotada: auto-transición de estado hacia adelante

Para reducir fricción operativa, los skills que terminan una fase con éxito **SÍ** ejecutan automáticamente la transición de estado del CLI hacia adelante (no hacia atrás, no hacia otro skill). Las transiciones autorizadas son sólo estas tres:

| Skill | Auto-ejecuta al terminar con éxito | Efecto |
| :--- | :--- | :--- |
| `/q-brief` | `quorum task blueprint <TASK_ID>` | Mueve la tarea de `inbox/` a `active/` |
| `/q-decompose` | `quorum task split <PARENT_ID>` | Materializa hijos en `inbox/` desde el campo `decomposition` |
| `/q-blueprint` | `quorum task start <TASK_ID>` | Crea worktree y rama `ai/<TASK_ID>` |

Los demás skills (`/q-analyze`, `/q-implement`, `/q-verify`, `/q-review`, `/q-accept`, `/q-memory`, `/q-status`) **no** tienen transición de estado para auto-ejecutar.

La excepción está bajo control:

- Solo se autorizan transiciones **forward**. La reversión (`quorum task back`) la decide y la ejecuta exclusivamente el humano.
- Si la fase termina en `BLOCKED`, el skill **no** corre la transición.
- La transición no implica activar otro skill: el siguiente despacho lo hace el orquestador.

### Tabla de fronteras por skill

| Skill | Fase única | Auto-transición forward | Lo que el skill SIGUE sin poder hacer |
| :--- | :--- | :--- | :--- |
| `q-brief` | Specify | `quorum task blueprint` | Activar `q-blueprint`. Pre-llenar 01/02. |
| `q-decompose` | Decomposition | `quorum task split` | Activar `q-brief` por los hijos. Inventar invariantes nuevas. |
| `q-blueprint` | Blueprint + Contract | `quorum task start` | Activar `q-analyze`/`q-implement`. Correr `verify.commands`. |
| `q-analyze` | Consistency Analysis | (ninguna) | Modificar artefactos. Activar `q-blueprint`. |
| `q-implement` | Implementation | (ninguna) | Activar `q-verify`. Correr BDD. Decidir retry. |
| `q-verify` | Verification | (ninguna) | Editar código. Activar `q-review`. Decidir retry. |
| `q-review` | Contract Review | (ninguna) | Editar código. Activar `q-accept`. Mergear. |
| `q-accept` | Merge Gate | (ninguna) | Mergear. Mover a `done/`. Activar `q-memory`. |
| `q-memory` | Memory Capture | (ninguna) | Activar cualquier otro skill. Editar código. |
| `q-status` | Read-only Status | (ninguna) | Modificar artefactos. Activar cualquier otro skill. |

### Handoff es información explícita + indicador de espera

El cierre de cada skill debe (1) declarar la transición ejecutada (si la hubo), (2) enumerar los siguientes pasos para el orquestador con cada uno marcado como `[Obligatorio]` o `[Opcional]`, (3) referenciar `quorum task back <ID>` como vía de rollback humana, y (4) terminar con la última línea exacta `ESPERANDO RESPUESTA DEL USUARIO...` (mayúsculas, tres puntos, sin texto después). Los ejemplos pueden estar en bloques Markdown dentro de la documentación, pero el output real del agente no debe dejar un cierre de bloque después del indicador.

Las salidas que solo dicen "Next phase: X" sin enumeración explícita están deprecadas. La forma canónica está documentada en cada `SKILL.md` bajo `## 🛑 Handoff`.

### Por qué la modularidad sigue siendo no-negociable

1. **Control de costos.** Cada fase puede enrutarse a un nivel de modelo distinto (`config.yaml` tiers 0/1/2). Un modelo barato puede correr `q-status` o `q-brief`; uno potente queda reservado para `q-implement` en tareas de alto riesgo. Auto-encadenar fases adentro de un mismo agente fuerza toda la pipeline a un solo tier — generalmente el más caro — y quema tokens que la política nunca autorizó.
2. **Autoridad de policy (Regla #7).** Routing, retries y escalaciones las decide el dispatcher, no el agente. Un skill que activa al siguiente está tomando una decisión de routing que no tiene autoridad para tomar.
3. **Checkpoints humanos.** Cada transición de artefacto (`00 → 01`, `01 → contrato validado`, `validation → review`, `review → accept`) es un punto de inspección. Auto-encadenar colapsa todos los checkpoints en uno y le quita al humano la posibilidad de intervenir antes del próximo despacho.
4. **Aislamiento de fallos.** Si una fase falla, sólo esa fase falla. Auto-encadenar propaga un mal spec a un mal blueprint a una mala implementación.
5. **Idempotencia.** Un skill modular se puede re-correr sobre una tarea en cualquier estado sin rehacer fases anteriores. Una ejecución encadenada no.

### Anti-patrones (rechazados)

- Un skill que dice *"¿procedo a /q-blueprint?"* y procede sin esperar respuesta.
- Un skill que llama `Skill`/`Activate` sobre otro `/q-*` desde adentro de su ejecución.
- Un skill que ejecuta una transición CLI **fuera** de las tres autorizadas en la tabla de arriba.
- Un skill que ejecuta `quorum task back` por su cuenta. La reversión es exclusivamente humana.
- Un skill "wrapper" que orquesta el ciclo completo. El orquestador ES el ciclo; no se envuelve en un skill.
- Prompts multifase ("actuá como q-brief y después como q-blueprint") — separalos en dos despachos.

Este principio es vinculante en todo skill actual y futuro de Quorum, y está reforzado por la Regla Inmutable #9.

### Decomposition: una feature ≠ una sola tarea

Cuando el spec describe una feature lo suficientemente grande como para superar la capacidad de un LLM modesto en una sola sesión, `/q-decompose <PARENT_ID>` la divide en N hijos independientes (`<PARENT_ID>-a`, `<PARENT_ID>-b`, ...) que recorren cada uno **su propio ciclo completo** (`/q-brief` → `/q-blueprint` → `/q-analyze` opcional → `/q-implement` → `/q-verify` → `/q-review` → `/q-accept` → merge humano → `quorum task clean` → `/q-memory`), cada uno en su propio worktree y rama `ai/<PARENT_ID>-<x>`.

Reglas:

- El padre permanece en `active/` como coordinador y nunca se implementa directamente. Sus hijos referencian `parent_task: <PARENT_ID>`.
- Cada hijo merge a `main` independientemente cuando está `ready`. No hay rama integradora.
- Las dependencias entre hijos se modelan vía el campo `depends_on` en cada hijo. El orquestador respeta el orden topológico al despachar.
- La heurística de splitting está en `.agents/policies/decomposition.yaml` (señales adaptadas de `spec-kitty.tasks`: 3-7 subtareas atómicas por implementación, máx 10, signals por concerns ortogonales / fases mezcladas / cross-runtime / risk alto).
- El skill nunca decompone silenciosamente: aplica la heurística, propone una decomposition concreta y pide confirmación humana antes de persistir.
- `quorum task split <PARENT_ID>` materializa los hijos de forma idempotente y valida que el padre esté en `active/`, que no sea una tarea hija, que los hijos tengan IDs `<PARENT_ID>-a/b/c`, que `depends_on` apunte a hermanos existentes y que no haya ciclos.
- `quorum task clean <PARENT_ID>` no archiva un padre con `decomposition` hasta que todos sus hijos estén en `done/`.

### Spec fields for decomposition

`00-spec.yaml` acepta estos campos opcionales:

```yaml
parent_task: FEAT-001        # sólo en hijos, e.g. FEAT-001-a
depends_on:                  # sólo en hijos cuando necesitan hermanos previos
  - FEAT-001-a
decomposition:               # sólo en padres umbrella, escrito por /q-decompose
  - child_id: FEAT-001-a
    summary: Primera slice implementable de forma independiente.
    depends_on: []
```

Todos los schemas de artefactos por tarea (`spec`, `blueprint`, `contract`, `validation`, `review`, `trace`) aceptan IDs hijos con sufijo de una letra (`FEAT-001-a`). Esto permite que cada hijo tenga su propio worktree, branch, verify, review y accept.

---

## 🧪 Testing Policy

Quorum's `verify.commands` execute fast unit tests and lint for agent feedback loops. BDD acceptance specs run in a separate slower suite, executed by the human before merge approval.

```text
Agent loop:    unit tests + lint     (target: <60s)
Human gate:    BDD acceptance suite  (target: <10min)
```

Agents never wait for BDD. Humans never approve without BDD.

---

## 🧠 Memory Governance

The `memory/` directory is a **curated knowledge library, NOT an activity log**. The activity log lives in `07-trace.json`. This separation is what prevents Model Collapse and noise contamination — do not collapse it.

### Capture is human-invoked, never automatic

Memory ingestion happens only when `q-memory` is explicitly invoked, typically after task acceptance. Quorum does NOT auto-ingest session summaries, retry logs, or per-step traces. Any future proposal that suggests "automatic memory ingestion" or piping session logs into `memory/` is redundant — **human invocation IS the curation gate**. There is no other gate to add.

### Three memory types, no priority states

Memory entries are typed by nature, not by quality grade:

| Type | Directory | Purpose |
| :--- | :--- | :--- |
| `pattern` | `memory/patterns/` | Reusable implementation or testing pattern. |
| `decision` | `memory/decisions/` | Architectural or policy decision affecting future work. |
| `lesson` | `memory/lessons/` | Bug cause, failure mode, review finding, process improvement. |

This typology already encodes priority implicitly: patterns are high-signal canonical forms; lessons are operational learnings. **Do not add orthogonal status fields** like `gold_standard`, `operational_log`, `discarded`, or `confidence_score` — they duplicate what the type system already expresses, or create unverifiable LLM-generated precision.

### What is NOT captured

`q-memory` explicitly excludes:

- Raw source code (Git is the code truth, Rule #1).
- Obvious task summaries (already in `00-spec.yaml.summary`).
- Temporary implementation details, retries, syntax errors (those are trace, not knowledge).
- Secrets or credentials.
- Generic advice not specific to this project.

### Evolution via `supersedes`

The schema field `supersedes` allows a new memory to formally replace a prior one whose conclusion is now wrong or incomplete. Superseded files are NOT deleted; the link plus Git history preserves the causal trace. Use `related` for complementary memories that do not invalidate each other.

### Anti-patterns are first-class

The optional `anti_patterns` field on every memory entry captures approaches that were considered and rejected with technical justification. This prevents rediscovery of known dead-ends and is a peer of positive knowledge, not a footnote.

### External memory systems are out of scope

Quorum is local-first and machine-first on disk. Integrations with external semantic stores (HSME, vector DBs, RRF rerankers, time-decay scoring) are out of scope for the framework itself. Such systems may consume `memory/*.json` as a read-only source, but Quorum does not depend on them and does not write to them. Proposals to embed external retrieval logic into `q-memory` violate Rule #1 (Git is the code truth) and Rule #5 (Machine-First, on-disk artifacts).

---

## 🚦 Routing & Risk Governance

Risk assessment and model routing are policy-driven. The framework already provides the building blocks; new proposals must reuse them, not reinvent them.

### Existing policy files

| File | Role |
| :--- | :--- |
| `.agents/policies/risk.yaml` | Named risk signals (`high/medium/low_risk_signals`) and `sensitive_paths` (executable globs). Touching any glob is a binary signal — it forces a higher tier regardless of file count. |
| `.agents/policies/routing.yaml` | Maps `risk: low|medium|high` → `executor_level`, `reviewer_required`, `max_attempts`, `human_gate_required`. Includes `type_overrides` for `migration` and `security`. |
| `.agents/config.yaml` | Three executor levels (0/1/2). Each level has primary/fallback/secondary models, max cost per call, and a `requires_human_gate` flag. |

### Existing retrievers

| File | What it computes |
| :--- | :--- |
| `.agents/retrievers/ast_neighbors.py` | Files that reference symbols defined in seed files (exported-symbol impact). |
| `.agents/retrievers/import_graph.py` | Multi-hop dependency graph from seed files (dependency depth). |

Any future proposal that asks for "exported-symbol detection" or "dependency depth analysis" must wire these retrievers, not write new ones.

### Risk authority

The human assigns `risk` in `00-spec.yaml`. Automation MAY suggest a level based on signals, but does NOT silently overwrite a value already set by the human. Divergence between calculated and declared is recorded in `07-trace.json`, not silently corrected. This preserves Rule #7 (cost is bounded by policy, not by trust in the agent's self-assessment).

### Levels are decoupled from model names

The risk/routing layer emits `level: 0|1|2` only. The translation level → concrete model lives in `config.yaml`. **Any proposal that hardcodes specific model names in scoring or routing logic is rejected** — it ages with every model release and breaks the decoupling.

### What is NOT needed (rejected proposals)

- **A new `routing_decision.json` artifact.** Routing decisions belong in `07-trace.json` events or in `02-contract.yaml.execution`. Do not introduce new artifact slots for this.
- **Magic-number scoring with arbitrary weights** (e.g. +1 per file, +10 per sensitive match). Risk assignment is **signal-based**: binary glob matches against `sensitive_paths`, plus simple thresholds on file count and exported-symbol count. Weighted accumulation without telemetry is guesswork.
- **Auto-overriding human-set risk.** See "Risk authority" above.
- **Hardcoded model lists in scoring engines.** See "Levels are decoupled" above.
- **Re-implementing dependency or symbol analysis.** The retrievers already exist; reuse them.

---

## 🛟 Failure Handling

When a task does not satisfy its contract, Quorum already has a structured chain of artifacts to record what happened, why, and how to avoid it next time. New proposals must use this chain, not invent parallel ones.

### Existing artifacts in the failure chain

| Artifact | Role on failure |
| :--- | :--- |
| `04-implementation-log.yaml` | What the executor changed and any blockers it hit. Append per attempt. |
| `05-validation.json` | Per-command exit codes and output excerpts (≤2000 chars). `overall_result: passed|failed|blocked`. |
| `06-review.json` | Reviewer verdict (`approve|revise|reject`), `forbidden_files_touched`, `missing_tests`, `functional_risk`, `notes`, and structured `fix_tasks` with `slug` + `scope`. |
| `07-trace.json` | Per-attempt record (`phase`, `result`, `model`, tokens, cost, duration, notes) plus `violations` and `outcome: committed|failed|aborted`. |

### Existing mechanisms for negative knowledge

- **Per-task forbiddance**: `02-contract.yaml.forbid.behaviors` is the binding list of patterns the executor must not introduce. Lessons from past failures of similar tasks belong here.
- **Cross-task lessons**: `memory/lessons/` (with `q-memory`) captures durable failure modes. The `anti_patterns` field on every memory entry records approaches rejected with technical justification (see Memory Governance).
- **Retry policy**: `02-contract.yaml.retry_policy.max_attempts` (range 0-5) caps retries. The dispatcher (when active) is the only authority to retry; the agent never decides. **Authorized Child Retry**: Failed child tasks may be retried by `/q-implement` if authorized by ADR 0001. This preserves the append-only nature of `07-trace.json` and does not automate human-only actions (merge/rollback).

### Failure classification (lightweight)

`05-validation.json` may carry an optional `error_category: logic|dependency|environment|flaky|unknown`. This distinguishes failures that require code change (`logic`, `dependency`) from failures that warrant only a retry (`environment`, `flaky`). `q-verify` assigns the category from heuristics over `output_excerpt`; no extra agent is involved.

### Cross-task failure context (lightweight)

`q-blueprint` may query `.ai/tasks/failed/` for tasks whose `affected_files` overlap significantly with the new blueprint's, and surface their `05-validation.json` excerpts plus `06-review.json.notes` as input to the `risks` array of the new blueprint. No new artifact, no new agent — pure read of existing files.

### Rule #4 boundary

A failure analysis cannot "forgive" a failed test. **Validation is finality** (Rule #4). If a test is wrong, the test gets fixed; the failure is not waived. Any proposal that allows a post-mortem or diagnostic agent to override `verify.commands` is rejected.

### What is NOT needed (rejected proposals)

- **A new `08-post-mortem.json` artifact.** Its fields duplicate `05-validation.json.commands[]` (command, exit_code, output_excerpt), `06-review.json.fix_tasks` and `notes` (suggested fixes), and `07-trace.json.attempts[].phase` (failure step). Use the existing slots; do not introduce a parallel artifact.
- **A separate "Diagnostic-L0" agent.** `q-review` already analyzes the diff against `05-validation.json` and produces `fix_tasks`. `q-memory` already distills failure modes into `lessons` with `anti_patterns`. A third agent is duplication.
- **"Negative constraints" as a new mechanism.** `02-contract.yaml.forbid.behaviors` is exactly this. For knowledge transferable across tasks, `memory/lessons/anti_patterns` is the home.
- **"Promotion to memory" as a new flow.** That IS what `q-memory` does. Invoke it on tasks in `failed/` if the failure carries a durable lesson.
- **Auto-overriding `verify.commands` results.** Rule #4 is non-negotiable.

---

## 🔀 Concurrency & Merge-Gate Governance

Quorum already contains the **foundations** for concurrent work on multiple tasks. New proposals in this area must build on those primitives, not restate them as missing features.

### What already exists

| Existing mechanism | Where | Role |
| :--- | :--- | :--- |
| **Isolated worktrees per task** | `worktrees/<TASK_ID>/` | Parallel tasks never edit the same checkout directly. |
| **Per-task feature branch** | `ai/<TASK_ID>` | Each task works on its own branch, isolated from `main`. |
| **Human-only merge authority** | Rule #6 | Agents may commit; only humans merge to `main`. |
| **Merge-gate phase already reserved** | `07-trace.json.attempts[].phase = merge_gate` | Pre-merge validation belongs in trace, not in a new artifact. |

These are already enough to justify a future **pre-merge compatibility check** without inventing a separate orchestration model.

### Recommended future direction

If Quorum adds concurrency safety, the MVP should be:

1. Create a temporary worktree from the current `main`.
2. Attempt a **shadow merge** of `ai/<TASK_ID>` with `--no-commit`.
3. Run the task's existing `verify.commands`.
4. Record the result in `07-trace.json` as a `merge_gate` attempt.

This preserves:

- Rule #4: validation, not Git alone, decides compatibility.
- Rule #6: the system still does **not** merge.
- Rule #8: tests remain the only proof of integration safety.

### What is NOT needed (rejected or deferred proposals)

- **A dedicated "Integrator agent".** This is primarily Git + deterministic verification, not an LLM task.
- **Continuous monitoring of `main` that mutates all active tasks.** Drift can be computed lazily when a task requests review/pre-merge.
- **A new artifact for integration status.** Use `07-trace.json` `merge_gate` attempts and notes; do not create parallel slots.
- **Auto-rebase as default recovery.** Rewriting task branches automatically is too risky before the runtime/review pipeline exists and is auditable.
- **System-prepared merge commits waiting for human confirmation.** Too close to violating Rule #6; the human performs the merge.

### Schema boundary

`07-trace.json` currently has `additionalProperties: false`. Any future desire to add explicit drift metadata (for example, base commit or validated main commit) must be introduced deliberately in the trace schema, not improvised ad hoc in runtime output.

---

## ⚖️ Immutable Rules (The Constitution)

1. **Git is the Code Truth**  
   Semantic memory is for patterns; Git is for code.

2. **Deterministic Context**  
   Agents never receive "the whole project". They get the `context_bundle` derived from the Blueprint.

3. **No Patching outside the Contract**  
   If an agent touches a file not in the `touch` list, the change is rejected.

4. **Validation is Finality**  
   A task is NOT done until `verify.commands` return 0.

5. **Machine-First Artifacts, Audience-Aware Format**  
   All planning files are YAML or JSON, never Markdown narrative. YAML for planning artifacts: spec, blueprint, contract. JSON for system-captured artifacts: validation, review, trace. Markdown is permitted only in `/docs/adr/` and external documentation.

6. **The System Commits, Never Merges**  
   Agents commit to feature branches in isolated worktrees. Merging to main is a human-only action.

7. **Cost Is Bounded by Policy, Not Trust**  
   Routing, retries, and escalations are decided by the dispatcher, never by the agent.

8. **Tests Are the Only Proof of Work**  
   No spec, blueprint, or contract proves functionality. Only `verify.commands` do.

9. **Skills Are Single-Phase Units**  
   Every `/q-*` skill executes exactly one declared phase and stops. Skills never auto-activate other skills, never chain into the next phase, and never make routing decisions. They MAY auto-execute one authorized forward state-transition CLI (the three listed in "Skill Modularity"); rollback (`quorum task back`) is exclusively human. The orchestrator (human or external runtime) dispatches each phase independently.

---

## 📂 System Structure

```bash
project/
├── .agents/
│   ├── schemas/         # JSON Schemas validating both YAML and JSON artifacts
│   │   ├── spec.schema.json
│   │   ├── blueprint.schema.json
│   │   ├── contract.schema.json
│   │   ├── implementation-log.schema.json
│   │   ├── validation.schema.json
│   │   ├── review.schema.json
│   │   ├── trace.schema.json
│   │   └── memory.schema.json
│   ├── prompts/         # Role-specific system instructions
│   ├── policies/        # Risk, routing, and decomposition logic
│   │   ├── risk.yaml
│   │   ├── routing.yaml
│   │   └── decomposition.yaml
│   ├── config.yaml      # Model assignments, cost ceilings, retry policies
│   ├── templates/
│   │   ├── 00-spec.yaml # includes optional parent_task/decomposition/depends_on examples
│   │   ├── 01-blueprint.yaml
│   │   └── 02-contract.yaml
│   └── skills/          # Quorum skills: q-brief, q-decompose, q-blueprint, ...
├── .ai/tasks/
│   ├── inbox/           # Specs and blueprints in draft
│   ├── active/          # 00-spec.yaml, 01-blueprint.yaml, 02-contract.yaml,
│   │                    # 04-implementation-log.yaml, 05-validation.json,
│   │                    # 06-review.json, 07-trace.json
│   ├── done/
│   └── failed/
├── docs/adr/            # Architectural decisions (Markdown allowed)
├── memory/              # Selective semantic learning
└── worktrees/           # Isolated agent sandboxes (gitignored)
```
