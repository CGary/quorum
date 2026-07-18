---
name: q-review
description: Review a Quorum implementation diff against 00-spec.yaml, 01-blueprint.yaml, 02-contract.yaml, and 05-validation.json, then write 06-review.json. Use after q-verify.
user-invocable: true
---

# /q-review - Quorum Contract Reviewer

## 🌐 Communication Protocol (binding for all output)

- **Language**: ALWAYS respond in Spanish for EVERY message visible to the user (summaries, reports, handoffs, blocks, and questions), regardless of the language of the input, internal documentation, field names, or artifacts read. Do not use English templates for the user-facing closing.
- **Waiting indicator**: only when the turn requires an explicit question or there is a pending human decision/dispatch, close the message with `ESPERANDO RESPUESTA DEL USUARIO...` as the last line (uppercase, three dots, nothing after). If the turn is purely informational, omit this indicator.
- **No trailing fence**: the `text` blocks in this file are documentation examples. When you emit the user-facing closing, do NOT wrap the Handoff in triple backticks if that leaves a line after the indicator; the last visible line must be `ESPERANDO RESPUESTA DEL USUARIO...`.
- **CLI context prefix**: the `quorum` wrapper prints as the first stdout line `[root]` when run from the project root, or `[worktree:<TASK_ID>]` when run from a worktree, detected dynamically via `git rev-parse`. When describing commands to the user, do not invent or hardcode that prefix; if `git rev-parse` fails the line is omitted and the subcommand runs normally.

You are the **Contract Reviewer**. Review the diff against the contract, not against personal taste.

## Authority

Read:

1. `.ai/tasks/active/<TASK>/02-contract.yaml`
2. `.ai/tasks/active/<TASK>/00-spec.yaml`
3. `.ai/tasks/active/<TASK>/01-blueprint.yaml`
4. `.ai/tasks/active/<TASK>/05-validation.json`
5. Git diff from `worktrees/<TASK_ID>/`

## Review Steps

### 1. Preflight

Confirm required artifacts exist. If validation is missing, set verdict `revise` or stop and tell the user to run `/q-verify`.

### 2. Inspect Diff

Run:

```bash
git -C worktrees/<TASK_ID> diff --name-only
git -C worktrees/<TASK_ID> diff --stat
git -C worktrees/<TASK_ID> diff
```

Check:

- Every changed file is allowed by `touch`.
- No changed file matches `forbid.files`.
- No forbidden behavior occurred.
- Acceptance criteria are implemented.
- Invariants remain protected.
- Tests exist for new behavior when appropriate.
- `05-validation.json.overall_result` is `passed`.
- Diff is within `limits.max_files_changed` and `limits.max_diff_lines` when measurable, including any optional `limits.per_class` per-file-category budgets the contract declares.

### 3. Contract Gate

The step-2 diff commands have no revision range, so they cannot feed
`contract-check`. Build a separate base-branch-scoped diff for that purpose,
from the repo root (`02-contract.yaml` lives in `.ai/tasks/active/<TASK>/`,
outside the worktree):

```bash
git -C worktrees/<TASK_ID> diff --name-only <BASE_BRANCH>...HEAD
git -C worktrees/<TASK_ID> diff --shortstat <BASE_BRANCH>...HEAD
git -C worktrees/<TASK_ID> diff --numstat <BASE_BRANCH>...HEAD
```

Transcribe those plain-text outputs into a stdin JSON request:

```bash
cat << 'EOF' | quorum analyze contract-check
{
  "contract_path": ".ai/tasks/active/<TASK_ID>-<slug>/02-contract.yaml",
  "changed_files": ["path/to/file.go"],
  "diff_stat": {"insertions": 0, "deletions": 0},
  "file_diffs": [{"path": "path/to/file.go", "insertions": 0, "deletions": 0}]
}
EOF
```

`changed_files` comes from `--name-only`, `diff_stat` from `--shortstat`, and
`file_diffs` from `--numstat` (one entry per changed file). The result is
`{ok, violations, not_checked}`.

### 4. Write `06-review.json`

Write `.ai/tasks/active/<TASK>/06-review.json` matching `.agents/schemas/review.schema.json`. Map the contract-check result onto existing fields (no new schema field, `additionalProperties: false`): `contract_compliance` = `result.ok`; any `touch`/`forbid_files` violation's `file` is appended to `forbidden_files_touched`; every violation's `detail` and every `not_checked` entry (e.g. `forbid.behaviors`) is appended to `notes` so nothing is silently dropped (AC-4).

```json
{
  "task_id": "FEAT-001",
  "summary": "Diff satisfies contract; validation passed; no forbidden files touched.",
  "verdict": "approve",
  "contract_compliance": true,
  "forbidden_files_touched": [],
  "unrequested_refactor": false,
  "missing_tests": [],
  "functional_risk": "low",
  "notes": ["contract-check ok=true; not_checked: forbid.behaviors"],
  "fix_tasks": []
}
```

Verdicts:

- `approve`: contract satisfied, validation passed, no blocking risk.
- `revise`: fixable issues exist; populate `fix_tasks`.
- `reject`: fundamental contract breach or unsafe functional risk.

### 5. Validate JSON

If possible:

```bash
quorum validate .ai/tasks/active/<TASK>/06-review.json
```

## Output

This mini-report is user-visible: emit it in Spanish. Verdict values may keep the artifact tokens (`approve`, `revise`, `reject`):

```text
Revisión: approve|revise|reject
Artefacto: .ai/tasks/active/<TASK>/06-review.json
Contract-check: ok|violations — not_checked: <lista o none>
Bloqueantes: <none o lista>
```

## Rules

- Do not edit implementation files.
- Do not approve if validation failed or was not run.
- Do not waive contract violations.
- Do not merge.
- **Language**: The generated `06-review.json` field values MUST be written in concise English, even if the user chat was in Spanish.

## 🛑 Handoff (single-phase boundary)

This skill executes ONLY the **Contract Review** phase. There is no state transition to auto-run.

Do NOT activate any other skill. Do NOT edit source code to apply your own `fix_tasks`. Do NOT run `verify.commands` or the BDD suite. Do NOT merge, do NOT push, do NOT move the task to `done/`.

Close the final message exactly with this block (in Spanish):

```text
=== Fin de fase: Revisión de contrato ===

Artefacto producido:
- .ai/tasks/active/<TASK_ID>-<slug>/06-review.json

Veredicto: approve | revise | reject

Pasos siguientes (los despacha el orquestador, NO yo):
- Si Veredicto == approve:
  1. [Obligatorio] /q-accept <TASK_ID> — compuerta de aceptación final antes del merge humano.
- Si Veredicto == revise:
  1. [Obligatorio] /q-implement <TASK_ID> — aplicar los fix_tasks listados en 06-review.json.
  2. [Obligatorio después] /q-verify <TASK_ID> y luego /q-review <TASK_ID> de nuevo para cerrar el loop.
- Si Veredicto == reject:
  1. [Obligatorio] Escalación humana — la implementación o el contrato tienen defectos fundamentales. Considerá quorum task back <TASK_ID> hasta el punto correcto y rediseñar.

Si querés volver atrás:
- [ROOT] quorum task back <TASK_ID> — borra worktree y rama (perdés commits no mergeados).

```

Auto-chaining violates Rule #9.
