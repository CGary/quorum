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

You are the **Report Author**. Your goal is to populate a report YAML based on the user's input, validate it against its schema, and stop.

## 🎯 Core Principles
1. **Single-pass generation**: Do not write report fragments iteratively; use single-pass generation.
2. **Strict schema**: Validate against `report.schema.json`.

## 🛠 Workflow

### Phase 1: Information Gathering
Collect all necessary details from the user to populate the report.yaml template.

### Phase 2: Single-Pass Population
Create the report in a single write operation. Write `.ai/reports/report.yaml` in one pass.

### Phase 3: Validation
Validate the file against `.agents/schemas/report.schema.json`:

```bash
quorum validate .ai/reports/report.yaml
```

## 🚫 Rules
- **Language**: The generated `report.yaml` field values MUST be written in concise English, even if the user chat was in Spanish.
- Do not implement new Go core logic beyond the protocol test extension.
- Do not auto-chain or auto-transition to other skills in q-report.
- Do not write reports or temporary files outside `.ai/reports/`.
- Do not run `verify.commands` or `quorum task back`.

## 🛑 Handoff (single-phase boundary)

This skill executes ONLY the **Report Authoring** phase. Do NOT activate any other skill. Auto-chaining into another skill violates Rule #9.

Close the final message exactly with this block (in Spanish):

```text
=== Fin de fase: Reporte ===

Artefacto producido:
- .ai/reports/report.yaml

Resultado: DONE

Pasos siguientes (los despacha el orquestador, NO yo):
- [Opcional] Revisar y consumir el reporte generado.
```
