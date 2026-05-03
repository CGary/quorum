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

---

## 🚀 The AI-First Lifecycle (SDC: Spec-Driven Contracts)

### Phase 1: Specify (Logic Extraction)

- **Actor**: `q-brief` (Logical Architect).
- **Goal**: Convert human intent into a logical invariant map.
- **Output**: `00-spec.yaml`.
- **Logic**: Identify what must be true, what must not change, and success criteria. No code paths yet.

### Phase 2: Blueprint (Surgical Cartography)

- **Actor**: `q-blueprint` (Surgical Cartographer).
- **Goal**: Explore the codebase and design the surgical path.
- **Output**: `01-blueprint.yaml`.
- **Logic**: Map affected files, symbols, dependencies, existing tests, and required new scenarios.

### Phase 3: Contract (Operational Authority)

- **Actor**: Automation derived from Blueprint.
- **Goal**: Generate the strict execution contract.
- **Output**: `02-contract.yaml`.
- **Logic**: Define `touch`, `forbid`, fast `verify.commands`, limits, execution mode, and retry policy.

### Phase 4: Execute (Surgical Implementation)

- **Actor**: Executor L0.
- **Goal**: Implementation and fast verification.
- **Output**: Verified diff, `04-implementation-log.yaml`, `05-validation.json`, and `07-trace.json`.
- **Logic**: Operate in a Git worktree. Retries are controlled by dispatcher policy and `verify.commands` failures.

### Phase 5: Review and Merge Gate

- **Actor**: Reviewer agent + human.
- **Goal**: Verify contract compliance and acceptance.
- **Output**: `06-review.json` and human merge decision.
- **Logic**: The system commits agent work. The human runs BDD acceptance and performs the merge.

---

## 🧪 Testing Policy

Quorum's `verify.commands` execute fast unit tests and lint for agent feedback loops. BDD acceptance specs run in a separate slower suite, executed by the human before merge approval.

```text
Agent loop:    unit tests + lint     (target: <60s)
Human gate:    BDD acceptance suite  (target: <10min)
```

Agents never wait for BDD. Humans never approve without BDD.

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

---

## 📂 System Structure

```bash
project/
├── .agents/
│   ├── schemas/         # JSON Schemas validating both YAML and JSON artifacts
│   │   ├── spec.schema.json
│   │   ├── blueprint.schema.json
│   │   ├── contract.schema.json
│   │   ├── validation.schema.json
│   │   ├── review.schema.json
│   │   ├── trace.schema.json
│   │   └── memory.schema.json
│   ├── prompts/         # Role-specific system instructions
│   ├── policies/        # Risk and routing logic
│   └── config.yaml      # Model assignments, cost ceilings, retry policies
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
