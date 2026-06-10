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

This skill may also be invoked on tasks in `.ai/tasks/failed/<TASK>/` when the failure carries a durable lesson (e.g., a recurring root cause across multiple tasks). In that case capture a `lesson` memory with `anti_patterns` filled in. The failed task's directory remains in `failed/` regardless.

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

## Anti-patterns

Optionally capture approaches that were rejected during the task in the `anti_patterns` field.

Capture an anti-pattern when:
- An obvious-looking approach was tried and failed during implementation.
- A reviewer rejected an approach with technical justification.
- The blueprint considered an alternative and discarded it for traceable reasons.

Do NOT capture:
- Generic best-practice violations (those belong in linters, not memory).
- Personal style preferences without technical rationale.
- Approaches no one actually proposed.

Format: one sentence per anti-pattern, technical and concrete.

Example:
"Avoided global singleton in TaskManager because it broke worktree isolation."

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

## Supersession Protocol

The schema field `supersedes` references the ID of a prior memory this one corrects or replaces.

Use `supersedes` when:
- A new pattern/decision invalidates a prior one (e.g., refactor changed the canonical approach).
- A lesson was incomplete and a more accurate version is now available.
- The prior memory contains an error discovered later.

Do NOT use `supersedes` when:
- The new memory simply extends or complements the prior one (use `related` instead).
- The two memories address different aspects of the same task.

When superseding:
1. Read the target memory file to confirm it should be replaced.
2. In the new memory, set `supersedes` to the target's `id`.
3. Do NOT delete the superseded file. Git history + the `supersedes` link preserve the causal trace.

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

## 🛑 Handoff (single-phase boundary)

This skill executes ONLY the **Memory Capture** phase. After writing the curated `memory/*.json` entries, STOP.

- DO NOT activate any other skill. Memory capture is the terminal phase of the Quorum lifecycle.
- DO NOT edit source code, task artifacts, schemas, policies, or trace files.
- DO NOT push to external memory systems (HSME, vector DBs, etc.). Quorum is local-first; external consumers read `memory/*.json` themselves.
- DO NOT auto-trigger ingestion based on time, file count, or task volume. Capture is human-invoked exclusively.

End your final message with exactly this line and nothing after it:

```text
Next phase: lifecycle complete — orchestrator may dispatch /q-status to confirm state.
```

Self-chaining or auto-ingestion violates Quorum Rule #9 (Skills Are Single-Phase Units), the Memory Governance "human-invoked, never automatic" gate, and Rule #7 (Cost Bounded by Policy, Not Trust).
