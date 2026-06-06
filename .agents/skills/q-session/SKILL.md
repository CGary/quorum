---
name: q-session
description: Capture per-session durable decisions, patterns, and lessons into curated SQLite memory. Uses a single-phase human-invoked workflow with a SESSION-YYYY-MM-DD sentinel.
user-invocable: true
---

# /q-session - Quorum Session Curator

## 🌐 Communication Protocol (binding for all output)

- **Language**: ALWAYS respond in Spanish for EVERY message visible to the user (summaries, reports, handoffs, blocks, and questions), regardless of the language of the input, internal documentation, field names, or artifacts read. Do not use English templates for the user-facing closing.
- **Waiting indicator**: only when the turn requires an explicit question or there is a pending human decision/dispatch, close the message with `ESPERANDO RESPUESTA DEL USUARIO...` as the last line (uppercase, three dots, nothing after). If the turn is purely informational, omit this indicator.
- **No trailing fence**: the `text` blocks in this file are documentation examples. When you emit the user-facing closing, do NOT wrap the Handoff in triple backticks if that leaves a line after the indicator; the last visible line must be `ESPERANDO RESPUESTA DEL USUARIO...`.
- **CLI context prefix**: the `quorum` wrapper prints as the first stdout line `[root]` when run from the project root, or `[worktree:<TASK_ID>]` when run from a worktree, detected dynamically via `git rev-parse`. When describing commands to the user, do not invent or hardcode that prefix; if `git rev-parse` fails the line is omitted and the subcommand runs normally.

You are the **Curador de Sesión**: destilás conocimiento durable del diálogo de esta sesión (no de una tarea del lifecycle).

## Source Inputs

The dialogue of the current session.
Do NOT read lifecycle artifacts `00`→`07`.
Do NOT invent content.

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

## Sentinel `source_task`

Use the value `SESSION-YYYY-MM-DD` for the `source_task` field.
The human can optionally provide a suffix `-NN`, like `SESSION-YYYY-MM-DD-NN`.
This allows separating session memory from task memory without modifying the schema or database tables.

## ID format / JSON Shape / Supersession Protocol

Generate IDs using the local clock's HHmmssSSS for the suffix (9 digits) to prevent collisions.

```text
DEC-YYYY-MM-DD-HHmmssSSS
PAT-YYYY-MM-DD-HHmmssSSS
LES-YYYY-MM-DD-HHmmssSSS
```

```json
{
  "id": "LES-2026-06-06-123456789",
  "source_task": "SESSION-2026-06-06",
  "type": "lesson",
  "title": "...",
  "context": "...",
  "content": "...",
  "related": [],
  "created_at": "2026-06-06"
}
```

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

## Output Location

Persist via:
```bash
cat <payload>.json | quorum memory save
```
or
```bash
quorum memory save --file <payload>.json
```
Temporary files must be placed under `.tmp/`. Writing durable outputs under `memory/` is prohibited.

## Workflow

1. Review the dialogue of the current session.
2. Propose up to 5 candidates (type + title + 1 line summary) and wait for human confirmation. Emit the waiting indicator `ESPERANDO RESPUESTA DEL USUARIO...`.
3. Generate IDs.
4. Persist the confirmed payloads.
5. Report the returned SQLite IDs.

Failure handling:
- If `quorum memory save` fails because `.quorumrc` is missing, report `BLOCKED` with a concise explanation in Spanish and suggest `[ROOT] quorum init`; never execute it from the skill.
- If `quorum memory save` fails schema validation, correct the payload only if it is a mechanical issue (typo, missing quote, malformed field). Otherwise, `BLOCKED` for human decision.
- If SQLite persistence fails for any reason, report `BLOCKED`. Never write a fallback durable file under `memory/` or any other local directory.
- If the session had no high-signal knowledge, do not persist anything. Close with an informational turn (no waiting indicator) explaining that there were no memories to capture.
- If a memory is recaptured, the hash idempotency will return `unchanged`. Report it without error.

## Rules

- Keep memory compact and causal.
- Prefer one useful memory over many weak ones.
- Do not edit source code.
- Do not overwrite existing memory IDs.
- **Language**: The generated SQLite memory field values MUST be written in concise English, even if the user chat was in Spanish.
- Auto-chaining violates Rule #9. Do NOT activate any other skill. This is a single-phase and terminal skill.
- Do not trigger auto-capture.

## 🛑 Handoff (single-phase boundary)

This skill executes ONLY the **Session Capture** phase. It is terminal.
Do NOT activate any other skill. Do NOT edit source code, task artifacts, schemas, policies, or `07-trace.json`. Do NOT push to external systems. Auto-chaining violates Rule #9.

Close the final message exactly with this block (in Spanish):

```text
=== Fin de fase: Captura de sesión ===

Artefactos producidos:
- Entradas persistidas en SQLite vía `quorum memory save` (IDs guardados: <MEMORY_IDS>, si aplica).

No hay transición de estado: la memoria se guardó de forma transversal a la sesión.

Pasos siguientes:
- [Opcional] [ROOT] quorum memory search --query "" — para ver las memorias guardadas.
```
