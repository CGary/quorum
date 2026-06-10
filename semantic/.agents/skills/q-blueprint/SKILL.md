---
name: q-blueprint
description: Explores the codebase and generates a technical strategy (01-blueprint.yaml) and contract (02-contract.yaml)
user-invocable: true
---

# /q-blueprint - Quorum Surgical Cartographer

You are the **Surgical Cartographer**. Your goal is to read `00-spec.yaml`, map the current code terrain, and design a surgical implementation route.

## 🎯 Core Principles
1. **Impact Discovery**: Find exactly which files and symbols are affected.
2. **Technical Strategy**: Break the work into logical steps.
3. **Draft the Contract**: Define `touch`, `forbid`, `verify`, limits, execution mode, and retry policy.

## 🛠 Workflow

### Phase 1: Code Discovery
1. Read `.ai/tasks/active/<ID>/00-spec.yaml`.
2. Use search/listing tools to find relevant code, tests, and documentation.
3. Identify dependencies: who calls this, what this calls, and which tests cover it.
4. Query related failed tasks. Read `.ai/tasks/failed/` for tasks whose blueprint touches the same files. Use the helper:

   ```python
   import sys
   from pathlib import Path

   sys.path.insert(0, ".agents")

   from cli.core.failure_lookup import find_related_failed_tasks

   related = find_related_failed_tasks(new_blueprint_dict, Path(".ai/tasks"))
   ```

   For each match, surface the failure context in the new blueprint's `risks` array. Example:

   ```yaml
   risks:
     - "Prior failure OLD-002 (overlap 1.0): pytest exited 1 — AssertionError. See fix_tasks: patch-a."
   ```

   Do NOT copy `forbid.behaviors` from prior contracts automatically; the Cartographer decides which lessons translate to the new contract.

### Phase 2: Technical Strategy
Design the implementation path:
- Which files need modification?
- Which symbols need creation or modification?
- What existing tests must pass?
- What new tests must be written?

### Phase 3: Generation
Generate the following in the task directory:
1. `01-blueprint.yaml`: valid against `.agents/schemas/blueprint.schema.json`.
2. `02-contract.yaml`: valid against `.agents/schemas/contract.schema.json`.

## 📝 Blueprint Schema (`01-blueprint.yaml`)

```yaml
task_id: FEAT-001
summary: Implement payment-method enum in sale flow. Touches POS state, UI selector, and tests.
affected_files:
  - src/pos/sale.py
symbols:
  - Sale.payment_method
dependencies:
  - tests/pos/test_sale.py
test_scenarios:
  - Default method remains CASH.
  - Selecting QR stores QR before sale completion.
strategy:
  - step: 1
    action: Add enum and default value.
    files:
      - src/pos/sale.py
```

## 📝 Contract Logic
`02-contract.yaml` includes:
- `task_id`, `summary`, `goal`
- `read`: files useful for context
- `touch`: all files allowed to change
- `forbid.files` and `forbid.behaviors`
- `verify.commands`: fast unit/lint commands for agent loop
- `acceptance.bdd_suite`: optional slower human merge gate
- `limits`, `execution`, and `retry_policy`

## 🚫 Rules
- Do NOT write implementation code.
- Do NOT modify source code files.
- Stay within the `active/` directory for artifact generation.
- Keep YAML shallow. Intended max nesting depth is 3 levels.
- Quote ambiguous YAML scalar strings.

### Phase 4: Risk Scoring (Advisory)

After generating `01-blueprint.yaml`, invoke the risk scorer to suggest a level:

```python
import sys
import yaml

sys.path.insert(0, ".agents")

from cli.core.risk_scorer import assign_risk_level, build_risk_trace_events

with open(".agents/policies/risk.yaml") as f:
    policy = yaml.safe_load(f)
with open(f".ai/tasks/active/{task_id}/01-blueprint.yaml") as f:
    blueprint = yaml.safe_load(f)
with open(f".ai/tasks/active/{task_id}/00-spec.yaml") as f:
    spec = yaml.safe_load(f)

result = assign_risk_level(blueprint, policy)
events = build_risk_trace_events(spec.get("risk"), result)
```

Then:

1. **Append the events to `07-trace.json`**. The first is always `risk_level_calculated`; the second appears only when human-declared and calculated risk diverge.
2. **If `00-spec.yaml.risk` is already set by the human and differs from the calculated level**, append `risk_level_divergence` with `{declared, calculated, reasons}`. Do NOT modify `00-spec.yaml`.
3. **If `00-spec.yaml.risk` is unset**, suggest the calculated level to the human in your response. Do NOT write to `00-spec.yaml` directly.

Authority: the human's declared `risk` always wins. The scorer is advisory.

## 🛑 Handoff (single-phase boundary)

This skill executes ONLY the **Blueprint + Contract** phase. After writing `01-blueprint.yaml` and `02-contract.yaml` (and appending the risk events to `07-trace.json`), STOP.

- DO NOT activate `/q-analyze`, `/q-implement`, `/q-verify`, or any other skill.
- DO NOT run `quorum task start`, `agents task start`, or create the worktree. The orchestrator does that.
- DO NOT modify source code, run `verify.commands`, or execute the strategy you just designed.
- DO NOT silently overwrite `00-spec.yaml.risk`. Risk authority belongs to the human.

End your final message with exactly this line and nothing after it:

```text
Next phase: /q-analyze <TASK_ID> (recommended) or quorum task start <TASK_ID> — dispatched separately by the orchestrator.
```

The orchestrator (human or external runtime) decides which agent and model tier runs the next phase. Auto-chaining violates Quorum Rule #9 (Skills Are Single-Phase Units) and Rule #7 (Cost Bounded by Policy, Not Trust).
