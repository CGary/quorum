---
name: q-memory
description: Capture durable technical lessons, decisions, and patterns from a completed Quorum task into centralized SQLite memory. Use after task acceptance, review, or significant bug fixes to preserve reusable project knowledge.
user-invocable: true
---

# /q-memory - Quorum Learning Curator

## 🌐 Communication Protocol (binding for all output)

- **Language**: ALWAYS respond in Spanish for EVERY message visible to the user (summaries, reports, handoffs, blocks, and questions), regardless of the language of the input, internal documentation, field names, or artifacts read. Do not use English templates for the user-facing closing.
- **Waiting indicator**: only when the turn requires an explicit question or there is a pending human decision/dispatch, close the message with `ESPERANDO RESPUESTA DEL USUARIO...` as the last line (uppercase, three dots, nothing after). If the turn is purely informational, omit this indicator.
- **No trailing fence**: the `text` blocks in this file are documentation examples. When you emit the user-facing closing, do NOT wrap the Handoff in triple backticks if that leaves a line after the indicator; the last visible line must be `ESPERANDO RESPUESTA DEL USUARIO...`.
- **CLI context prefix**: the `quorum` wrapper prints as the first stdout line `[root]` when run from the project root, or `[worktree:<TASK_ID>]` when run from a worktree, detected dynamically via `git rev-parse`. When describing commands to the user, do not invent or hardcode that prefix; if `git rev-parse` fails the line is omitted and the subcommand runs normally.

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

## Output Location

Construct a JSON payload matching `.agents/schemas/memory.schema.json` and persist it via the centralized CLI command. `quorum memory save` performs the final schema validation and durable SQLite write.

Preferred stdin form:

```bash
cat <payload>.json | quorum memory save
```

Allowed temporary-file form:

```bash
quorum memory save --file <payload>.json
```

If a temporary payload file is needed, place it under `.tmp/` or the system temporary directory. Do not create, recreate, or write durable outputs under `memory/`, `memory/decisions/`, `memory/patterns/`, or `memory/lessons/`.

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
1. Query the target memory to confirm it should be replaced.
2. In the new memory payload, set `supersedes` to the target's `id`.
3. The old memory remains in the database; the `supersedes` link preserves the causal trace.

## Workflow

1. Read task artifacts.
2. Identify at most 3 high-signal memories.
3. Pick correct memory type.
4. Generate next numeric ID for today's date.
5. Persist the JSON payload with `cat <payload>.json | quorum memory save` or `quorum memory save --file <payload>.json`.
6. If `quorum memory save` fails because `.quorumrc` is missing or memory setup is unavailable, report `BLOCKED` and suggest `quorum init`; do not run `quorum init` automatically.
7. If `quorum memory save` fails validation, correct the payload only when the issue is mechanical; otherwise report `BLOCKED` for human decision.
8. If SQLite persistence fails for any reason, do not write a fallback file under `memory/` or any other durable local directory.

## Rules

- Keep memory compact and causal.
- Prefer one useful memory over many weak ones.
- Do not edit source code.
- Do not overwrite existing memory IDs.
- **Language**: The generated SQLite memory field values MUST be written in concise English, even if the user chat was in Spanish.


## Failure Handling

- Missing `.quorumrc` or unavailable memory setup: report `BLOCKED` with a concise Spanish explanation and suggest the human run `quorum init`; never execute `quorum init` from this skill.
- `quorum memory save` validation errors: fix mechanical JSON or schema-shape mistakes when the intended meaning is unchanged; otherwise stop with `BLOCKED` for human decision.
- SQLite, locking, permission, or database errors: stop with `BLOCKED`; never persist a fallback copy to a durable local file or legacy memory directory.
- Successful captures report the SQLite memory IDs returned by `quorum memory save`, not local file paths.

## 🛑 Handoff (single-phase boundary)

This skill executes ONLY the **Memory Capture** phase. It is terminal — there is no state transition to auto-run.

Do NOT activate any other skill. Do NOT edit source code, task artifacts, schemas, policies, or `07-trace.json`. Do NOT push to external systems (HSME, vector DBs); external consumers read the memory on their own via export tools. Do NOT auto-trigger ingestion by time/volume — capture is exclusively human-invoked.

Close the final message exactly with this block (in Spanish):

```text
=== Fin de fase: Captura de memoria ===

Artefactos producidos:
- Entradas persistidas en SQLite vía `quorum memory save` (IDs guardados: <MEMORY_IDS>, si aplica).

No hay transición de estado: la tarea ya fue archivada antes por quorum task clean.

Pasos siguientes:
- [Opcional] /q-status — vista global para confirmar que la tarea quedó en done/ y la memoria está registrada.
- [Terminal] El ciclo Quorum cerró para esta tarea. Si esta era una tarea hija (parent_task definido en el spec), considerá despachar /q-status <PARENT_ID> para ver si todas sus hermanas también cerraron.

```

Auto-chaining or auto-ingesting violates Rule #9, Memory Governance ("human-invoked, never automatic"), and Rule #7.
