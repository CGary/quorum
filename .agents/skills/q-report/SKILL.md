---
name: q-report
description: Fills the report.yaml template in one pass from user input, validates against report.schema.json. Single-phase, no auto-transition, Spanish protocol.
user-invocable: true
---

# /q-report - Quorum Report Author

## 🌐 Communication Protocol (binding for all output)

- **Language**: ALWAYS respond in Spanish for EVERY message visible to the user (summaries, reports, handoffs, blocks, and questions), regardless of the language of the input, internal documentation, field names, or artifacts read. Do not use English templates for the user-facing closing.
- **Waiting indicator**: only when the turn requires an explicit question or there is a pending human decision/dispatch, close the message with `ESPERANDO RESPUESTA DEL USUARIO...` as the last line (uppercase, three dots, nothing after). If the turn is purely informational, omit this indicator.
- **No trailing fence**: the `text` blocks in this file are documentation examples. When you emit the user-facing closing, do NOT wrap the Handoff in triple backticks if that leaves a line after the indicator; the last visible line must be `ESPERANDO RESPUESTA DEL USUARIO...`.
- **CLI context prefix**: the `quorum` wrapper prints as the first stdout line `[root]` when run from the project root, or `[worktree:<TASK_ID>]` when run from a worktree, detected dynamically via `git rev-parse`. When describing commands to the user, do not invent or hardcode that prefix; if `git rev-parse` fails the line is omitted and the subcommand runs normally.

You are the **Report Author**. Your goal is to populate a report YAML based on the user's input, persist it through the validated write path, and stop.

## 🧠 The soul of this feature
A Quorum report is NOT an essay and NOT a fixed audit form. It is an **information artifact with a visualization layer**: it preserves 100% of the content but changes the path through it to reduce cognitive friction. The reader scans a layer-cake of front-loaded headings (verdict first), drops into tables to compare, and finds exhaustive detail in the appendix instead of buried in prose.

## 🎯 Core Principles
1. **Single-pass generation**: Do not write report fragments iteratively; author the whole report in one pass.
2. **The body is a palette, not a form**: `report.schema.json` defines a CLOSED catalog of optional components. Only `meta` is mandatory. SELECT the components that fit the material and omit the rest. NEVER invent a component outside the catalog — `additionalProperties:false` will reject it and nothing will be written.
3. **Strict schema is the write barrier**: The report is persisted only through `quorum report save <id>`, which validates against `report.schema.json` before writing. Validation is the barrier, not an afterthought.
4. **Read the source of truth first**: Before authoring, read `.agents/schemas/report.schema.json` (or the seed `.agents/templates/report.yaml`) so the single-pass output matches the contract exactly. Authoring from memory invites drift.

## 🛠 Workflow

### Phase 0: Read the Contract
Read `.agents/schemas/report.schema.json` or `.agents/templates/report.yaml` before writing anything. This is mandatory: it anchors the single-pass output to the canonical structure and prevents schema drift.

### Phase 1: Decide the ID and select components (proceed by default)
- **Derive the `<id>`** from the user's request as a concise kebab-case slug that matches the canonical ID regex (`^[A-Za-z0-9][A-Za-z0-9_-]*$`). The `<id>` is both the filename and `meta.id` — they MUST be identical. **Proceed without asking.** Ask the user ONLY if the topic is irrecoverably vague or the derived `<id>` collides with an existing `.ai/reports/<id>.yaml`.
- **Select components from the catalog** below that fit the material. A "report on how X is used" is a guide, not an audit: it might use `verdict` + `summary` + `keyFindings` + `actionPlan` and omit `findings`/`evidence`/`risks`. Do NOT force every component; the body is a suggestion set, not a checklist.

#### Component catalog (each maps 1:1 to a viewer renderer)

| Component | YAML shape | Use it for |
|-----------|-----------|------------|
| `verdict` | string | The front-loaded one-sentence bottom line. Read first. |
| `summary` | string | Short context: what the reader should do or understand. |
| `decisionSurface` | object (free-form keys → string) | Triage fields: recommendation, confidence, main risk, best next action, when not to follow. |
| `keyFindings` | list of `{finding, whyItMatters?, action?}` | Scannable findings with impact and action. |
| `findings` | list of `{id, description, severity}` | Audit findings; `severity` ∈ critical/high/medium/low/info renders as a pill. |
| `evidence` | list of `{findingId, path, details}` | Ties a finding to its location and detail. |
| `tradeoffs` | list of `{option, upside?, downside?, useWhen?, avoidWhen?}` | Option comparison. |
| `risks` | list of `{id, description, impact}` | Risks; `impact` renders as a pill. |
| `actionPlan` | list of `{step, action, owner}` | Executable next steps. |
| `appendix` | string | Exhaustive detail / raw logs kept off the main path. Preserve content here instead of deleting it. |

#### Cognitive-load heuristics (apply while selecting and phrasing)
- **Front-load**: start `verdict`, headings, and table cells with the information-carrying term ("Risk: cache invalidation is manual", not "Analysis").
- **Tables over prose for comparison** (`tradeoffs`, `findings`, `risks`); reserve prose (`summary`) for causal reasoning; use scalar lists only for short parallel items.
- **Preserve 100%**: never delete detail to shorten — move it into `appendix`.
- **Concise field values**: every field value is plain text rendered by the viewer (no Markdown emphasis); keep values short and scannable.

### Phase 2: Single-Pass Population
Author the report into a temporary file under `.tmp/` (never write directly into `.ai/reports/`). Set `meta.id` to the derived `<id>` exactly, `meta.schemaVersion` to `"1.0"`, and `meta.date` to the current UTC timestamp in RFC3339 / `date-time` format (e.g. `2026-06-01T12:00:00Z`).

### Phase 3: Validated Persistence
Persist by piping the temp file into the validated write path. The CLI validates the ID regex, the `meta.id` ↔ filename identity, and the full schema before anything touches disk:

```bash
cat .tmp/temp_report.yaml | quorum report save <id>
```

A non-zero exit means nothing was written; fix the payload and re-run. Do NOT hand-write files into `.ai/reports/`.

## 🚫 Rules
- **Language**: The generated `.ai/reports/<id>.yaml` field values MUST be written in concise English, even if the user chat was in Spanish.
- **Closed catalog**: Use ONLY the components defined in `report.schema.json`. Never invent a new top-level component; if the material needs something the catalog lacks, that is a schema-evolution decision (a `meta.schemaVersion` bump), not an authoring shortcut.
- Persist reports ONLY through `quorum report save <id>`; never write directly into `.ai/reports/`.
- Do not implement new Go core logic beyond the protocol test extension.
- Do not auto-chain or auto-transition to other skills in q-report.
- Do not write reports or temporary files outside `.tmp/` and the validated `.ai/reports/` write path.
- Do not run `verify.commands` or `quorum task back`.

## 🛑 Handoff (single-phase boundary)

This skill executes ONLY the **Report Authoring** phase. Do NOT activate any other skill. Auto-chaining into another skill violates Rule #9.

Close the final message exactly with this block (in Spanish):

```text
=== Fin de fase: Reporte ===

Artefacto producido:
- .ai/reports/<id>.yaml (persistido y validado vía `quorum report save <id>`)

Resultado: DONE

Pasos siguientes (los despacha el orquestador, NO yo):
- [Opcional] Revisar y consumir el reporte generado.
```
