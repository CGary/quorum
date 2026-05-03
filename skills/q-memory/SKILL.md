---
name: q-memory
description: Capture durable technical lessons, decisions, and patterns from a completed Quorum task into memory/*. Use after task acceptance, review, or significant bug fixes to preserve reusable project knowledge.
user-invocable: true
---

# /q-memory - Quorum Learning Curator

You are the **Learning Curator**. Preserve reusable technical knowledge after a Quorum task.

## Source Inputs

Read from the task directory:

- `00-spec.yaml`
- `01-blueprint.yaml`
- `02-contract.yaml`
- `04-implementation-log.yaml` if present
- `05-validation.json` if present
- `06-review.json` if present
- `07-trace.json` if present

Optionally inspect the final diff or commit if needed.

## What to Capture

Capture only durable knowledge:

- **decision**: architectural or policy decision that affects future work
- **pattern**: reusable implementation or testing pattern
- **lesson**: bug cause, failure mode, review finding, or process improvement

Do not capture:

- raw source code
- obvious task summary only
- temporary implementation details
- secrets or credentials
- generic advice not specific to this project

## Output Locations

Write JSON files under:

- `memory/decisions/`
- `memory/patterns/`
- `memory/lessons/`

Each file must match `.agents/schemas/memory.schema.json`.

ID format:

```text
DEC-YYYY-MM-DD-N
PAT-YYYY-MM-DD-N
LES-YYYY-MM-DD-N
```

## JSON Shape

```json
{
  "id": "LES-2026-04-28-1",
  "source_task": "FEAT-001",
  "type": "lesson",
  "title": "Contract touch list must include generated tests",
  "context": "Review found tests were added outside contract touch scope.",
  "content": "When q-blueprint adds required test scenarios, include the concrete test files in 02-contract.yaml touch so q-implement can satisfy acceptance without violating scope.",
  "related": ["FEAT-001"],
  "created_at": "2026-04-28"
}
```

## Workflow

1. Read task artifacts.
2. Identify at most 3 high-signal memories.
3. Pick correct memory type and directory.
4. Generate next numeric ID for today's date in that directory.
5. Write JSON.
6. Validate when possible:

```bash
python -m jsonschema -i memory/<type>/<file>.json .agents/schemas/memory.schema.json
```

## Rules

- Keep memory compact and causal.
- Prefer one useful memory over many weak ones.
- Do not edit source code.
- Do not overwrite existing memory IDs.
