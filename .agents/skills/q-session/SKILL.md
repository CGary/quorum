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

## Pre-Save Duplicate Advisor (HSME)

Before persisting a curated memory entry, this skill runs an **advisory-only** duplicate
check against HSME, Quorum's subordinate semantic layer (ADR 0008). The advisor **never
blocks, gates, or writes** to any Quorum artifact — it only suggests candidates and
requires human approval before the persist step runs.

### Invocation

Shell out to `hsme-cli` with the project-scoped flag, wrapped in an explicit timeout, and
with `SQLITE_DB_PATH` pointing at HSME's database (typically `data/engram.db` relative to
the semantic module root, or as configured by the user's HSME environment):

```bash
SQLITE_DB_PATH="<hsme-db-path>" timeout 20 hsme-cli search-fuzzy "<proposed_title_and_content>" --project quorum --limit 10
```

If `search-fuzzy` is unavailable (missing binary, missing Ollama, or any runtime error),
fall back to the keyword search with the same flags:

```bash
SQLITE_DB_PATH="<hsme-db-path>" timeout 20 hsme-cli search-exact "<proposed_title>" --project quorum --limit 10
```

The query text is derived from the proposed memory's `title` and `content`. Every result
carries provenance: the HSME `memory_id` and the memory's title (retrieved from the
result's highlighted text or a supplementary record lookup).

### Human Decision Point

If candidates are returned, present them (id and title) to the human and ask to choose:

- **save** — proceed to the normal `quorum memory save` persist step.
- **skip** — do not persist; close the phase.
- **supersede** — persist the new entry with `supersedes` set to the matching Quorum memory
  ID (translate the HSME `memory_id` via the importer's mapping table per ADR 0008 §3
  data frontier).

The human's decision is final; the advisor's suggestions are informational only. Wait for
the human's explicit choice — do not persist without approval when candidates are shown.

### Graceful Degradation (ADR 0008 + ADR 0013)

If `hsme-cli` is missing, times out (>20s), errors, or returns no results, proceed exactly
as today — emit a one-line note in Spanish: `[ADVISOR] No disponible — se procede sin revisión semántica.` and continue to the persist step without blocking. An empty or stale capsule corpus (ADR 0013 §4) is a normal outcome, not an error condition.

**ADR 0008 authority rule**: HSME informs; Git, lifecycle artifacts, and curated `q-memory`
decide. The advisor is never code truth, never a validation gate, never an ingestion path
into Quorum's curated memory. This skill's `quorum memory save` call remains the only
ingestion path.

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
4. **Pre-save duplicate advisor** (HSME): run the advisory-only duplicate check (see section above) — query `hsme-cli` with `--project quorum` under a `timeout 20` with `SQLITE_DB_PATH` set; if candidates are returned (id + title), present them to the human and wait for the save/skip/supersede decision before proceeding. If the advisor is unavailable or returns no results, emit a one-line note and proceed to step 5.
5. Persist the confirmed payloads.
6. Report the returned SQLite IDs.

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
