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
4. **The catalog is the in-skill contract**: The Component catalog in Phase 1 mirrors `report.schema.json` and is authoritative here — author from it. Do NOT re-read the raw schema on every run; consult it (or the seed `.agents/templates/report.yaml`) only to resolve a specific doubt. A drift test keeps the catalog and the schema in sync.

## 🛠 Workflow

### Phase 0: Know the contract
The **Component catalog** in Phase 1 below is authoritative for this skill: it lists every valid component and its YAML shape. Author from it directly — do NOT re-read the raw `report.schema.json` (~200 lines) on every run; that only burns context. Read the raw schema (or the seed `.agents/templates/report.yaml`) ONLY to resolve a genuine doubt about a field. The closed catalog still wins: anything outside it is rejected at save.

### Phase 1: Decide the ID and select components (proceed by default)
- **Derive the `<id>`** from the user's request as a concise kebab-case slug that matches the canonical ID regex (`^[A-Za-z0-9][A-Za-z0-9_-]*$`). The `<id>` is both the filename and `meta.id` — they MUST be identical. **Proceed without asking.** Ask the user ONLY if the topic is irrecoverably vague or the derived `<id>` collides with an existing `.ai/reports/<id>.yaml`.
- **Select components from the catalog** below that fit the material. A "report on how X is used" is a guide, not an audit: it might use `verdict` + `summary` + `keyFindings` + `actionPlan` and omit `findings`/`evidence`/`risks`. Do NOT force every component; the body is a suggestion set, not a checklist.

#### Component catalog (each maps 1:1 to a viewer renderer)

##### Top-Level Semantic Properties (New v1.1 Model)
- `kind`: Report classification. Enumerates: `generic`, `project_usage`, `refactor_plan`, `refactor_result`, `audit`, `decision_brief`, `technical_analysis`.
- `presentation`: Visual settings with mandatory fields `profile` (e.g. `presentation.profile` matches one of `cognitive`, `executive`, `audit`, `teaching`, `raw`), `density`, `audience`, `language`.
- `content`: Root container for semantic content, containing `content.title`, `kicker`, `summary`, `verdict`, and `content.sections`.

##### Semantic Section Roles
- `decision_surface`: Key triage fields (recommendation, main risk, etc.).
- `verification`: Explicit uncertainty or correctness checks.
- `findings`: Structured technical or audit findings.
- `analysis`: Narrative/causal prose with optional progressive disclosure details.
- `diagram`: Mermaid diagram code.
- `tradeoffs`: Option comparison table.
- `risks`: Risks table with impact levels.
- `action_plan`: Executable next steps.
- `evidence`: Supports findings with paths and details.
- `appendix`: Non-blocking exhaustive detail.
- `metrics`: Numeric scalar tables or bars.
- `callout`: Decision, warning, or note boxes.

##### Legacy Model Properties (Compatibility Only)
- `verdict`: Front-loaded bottom line.
- `summary`: Short context paragraph.
- `decisionSurface`: Key-value triage fields.
- `callouts`: List of alert boxes.
- `verify`: Places to inspect and check.
- `keyFindings`: Scannable findings table.
- `diagrams`: Mermaid diagram list.
- `findings`: Audit findings table.
- `evidence`: Supporting location table.
- `tradeoffs`: Option comparison table.
- `risks`: Risks table.
- `actionPlan`: Action steps list.
- `appendix`: Exhaustive detail string.

#### Full example (cheat sheet — copy, then DELETE the components you don't need)

```yaml
meta:
  id: "my-report"                 # MUST equal the filename / save <id>
  schemaVersion: "1.1"            # optional: save auto-fills if omitted
  date: "2026-06-01T12:00:00Z"    # optional: save auto-fills (UTC RFC3339)

kind: generic

presentation:
  profile: cognitive              # cognitive | executive | audit | teaching | raw
  density: medium                 # low | medium | high
  audience: engineer              # engineer | maintainer | reviewer | manager | user
  language: es                    # es | en

content:
  title: "My Semantic Report"
  kicker: "Subtitle/kicker"
  summary: "Context paragraph"
  verdict:
    text: "Bottom line text"
    confidence: high              # high | medium | low
  sections:
    - id: section-1
      role: analysis
      title: "Section Title"
      body: "Body text"
```

#### Cognitive-load heuristics (apply while selecting and phrasing)
- **Front-load**: start `verdict`, headings, and table cells with the information-carrying term ("Risk: cache invalidation is manual", not "Analysis").
- **Tables over prose for comparison** (`tradeoffs`, `findings`, `risks`); reserve prose (`summary`) for causal reasoning; use scalar lists only for short parallel items.
- **Preserve 100%**: never delete detail to shorten — move it into `appendix`.
- **Verify first**: when the report is based on AI analysis or unexecuted assumptions, include `verify` with 1–4 concrete checks. Each row must say what to inspect, why it is risky, and the exact command or action to verify it. If everything looks suspicious, choose the few checks that would prevent the most rework.
- **Surface human decisions**: use `callouts` for pending human choices, warnings, or notes that should be visible before detail. Keep it to 3 or fewer so alert fatigue does not replace cognitive fatigue.
- **Use diagrams only for relationships**: include `diagrams` when prose would force the reader to mentally simulate flow, dependency, sequence, state, ownership, or phases. Keep Mermaid diagrams small (about 5–9 nodes), prefer `flowchart LR` for left-to-right process flow, `flowchart TD` for hierarchies, `sequenceDiagram` for temporal interactions, `stateDiagram-v2` for lifecycle states, and `timeline` for phased plans. Do not use diagrams as decoration.
- **Concise field values**: every field value is plain text rendered by the viewer (no Markdown emphasis); keep values short and scannable.

### Phase 2: Produce the draft in `.tmp/` (two equally valid paths)
The goal is a `.tmp/<id>.yaml` draft. Pick whichever path is cheaper:

- **Direct write (fewest tool calls)** — if you already know the catalog above, write the final YAML straight into `.tmp/<id>.yaml` with your file tool (copy the cheat sheet below and trim). Set `meta.id` = `<id>` (required); `meta.schemaVersion` and `meta.date` are OPTIONAL — `save` auto-fills them if omitted.
- **Scaffold (when you want a guided skeleton)** — let the CLI stamp `meta` and emit the commented component menu, then fill only what fits:

```bash
quorum report new <id> --output .tmp/<id>.yaml   # valid skeleton into .tmp/ (NOT .ai/reports/)
```

Either way: select ONLY the components that fit the material (palette — omit the rest) and never hand-build `.ai/reports/`. The dry-run in Phase 3 is your safety net, so direct authoring is safe.

### Phase 3: Persist (one step — `save` is safe by default)
`save` validates BEFORE writing (id regex + `meta.id`↔filename identity + full schema). On any failure it aborts with a non-zero exit and writes nothing, so a single `save` is the whole happy path — there is no separate "preflight" step:

```bash
quorum report save <id> --file .tmp/<id>.yaml
```

No temp file at all? Pipe the draft via stdin in one call:

```bash
cat .tmp/<id>.yaml | quorum report save <id>
```

`save` auto-fills `meta.schemaVersion` and `meta.date` if you omitted them. A non-zero exit means nothing was written — fix the payload and re-run. Do NOT hand-write files into `.ai/reports/`.

Optional: add `--dry-run` ONLY when you want to check validity WITHOUT creating the file (e.g. CI). It is not a required step.

## 🚫 Rules
- **Language**: The generated `.ai/reports/<id>.yaml` field values MUST match the language of the user's prompt, UNLESS the user explicitly requests a specific language. Reports are human-facing deliverables rendered in the viewer; unlike lifecycle artifacts (`00`–`07`) and SQLite memory — which stay in concise English for machine interoperability — report content follows the reader's language. Keep values concise and front-loaded in whatever language applies.
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
