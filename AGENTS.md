This file provides guidance when working with code in this repository.

## Project nature

Quorum is a Python framework that orchestrates AI agents through a **Spec-Driven Contracts (SDC)** lifecycle. It is **NOT** a chatbot or general assistant — it converts human intent into validated artifacts (`00`→`07`) and verified Git diffs. The framework is dogfooded: changes to Quorum's own code go through `pytest`, but feature work in *consumer* projects goes through the full `/q-*` skill lifecycle.

The canonical authority is `quorum.md` (the manifesto v1.1). When the manifesto and the code disagree, the manifesto wins and the code is wrong.

## Commands

```bash
# Run the full test suite
uv run pytest -v

# Run a single test file or test
uv run pytest tests/test_task_manager_artifacts.py -v
uv run pytest tests/test_validation_schema.py::test_valid_with_each_error_category -v

# Install Quorum globally as an editable tool (required to use `quorum` CLI elsewhere)
uv tool install --editable .

# Initialize Quorum scaffolding inside another project (creates .ai/tasks/, memory/, .gitignore entries)
quorum init

# Local CLI invocation without global install (sets PYTHONPATH=.agents and runs cli.main)
./agents <command>           # e.g. ./agents task list
```

The `agents` wrapper script and `main.py` both exist for the same reason: they prepend `.agents/` to `PYTHONPATH` because the actual package lives there (`pyproject.toml` declares `package-dir = {"" = ".agents"}`). Direct `python -m cli.main` from the repo root will fail without that path injection.

### Task CLI surface

State-mutating commands (use these in tests and tooling, not the skills):

```bash
quorum task specify <ID>            # creates .ai/tasks/inbox/<ID>-new-spec/00-spec.yaml
quorum task blueprint <ID>          # inbox/ -> active/ (auto-run by /q-brief)
quorum task split <PARENT_ID>       # materialises children from spec.decomposition (auto-run by /q-decompose)
quorum task start <ID>              # creates worktree + ai/<ID> branch (auto-run by /q-blueprint)
quorum task clean <ID>              # archives to done/ and removes worktree
quorum task back <ID>               # human-only rollback of the last forward transition
quorum task artifact-save <ID> <relpath>  # reads stdin, validates against schema, persists
quorum task list
quorum task status <ID>
```

## High-level architecture

### Lifecycle artifacts (`00`→`07`)

A task lives in one directory under `.ai/tasks/{inbox,active,done,failed}/<ID>-<slug>/`. The artifacts inside that directory are the state of the task — there is no database. Each artifact is bound to a JSON Schema in `.agents/schemas/`.

| File | Format | Schema | Producer |
|------|--------|--------|----------|
| `00-spec.yaml` | YAML | `spec.schema.json` | `/q-brief` |
| `01-blueprint.yaml` | YAML | `blueprint.schema.json` | `/q-blueprint` |
| `02-contract.yaml` | YAML | `contract.schema.json` | `/q-blueprint` |
| `04-implementation-log.yaml` | YAML | `implementation-log.schema.json` | `/q-implement` |
| `05-validation.json` | JSON | `validation.schema.json` | `/q-verify` |
| `06-review.json` | JSON | `review.schema.json` | `/q-review` |
| `07-trace.json` | JSON | `trace.schema.json` | system, append-only |
| `memory/{decisions,patterns,lessons}/*.json` | JSON | `memory.schema.json` | `/q-memory` (human-invoked) |

There is **no `03`, `08`, `09`, or `10`**. The manifesto rejects new lifecycle slots: failure data lives in `05/06/07/memory/lessons`, and impact reports go through `q-memory`. Do not propose new numbered artifacts without an ADR.

### Where state actually changes

`.agents/cli/core/task_manager.py` owns nearly all state mutations (~700 lines). The CLI commands (`commands/task.py`, `commands/project.py`) are thin shims. When in doubt, read `task_manager.py` first.

Important invariants enforced there:

- **`save_artifact()` validates before writing.** Any `task artifact-save` (or skill that persists via this path) is schema-checked against `ARTIFACT_SCHEMA_MAP`. Failure raises `ArtifactValidationError` with a `field=$.path; reason=...` format.
- **`07-trace.json` is append-only.** `_ensure_trace_append_only()` rejects any save that shortens or rewrites existing `attempts[]`. New attempts are appended via `append_trace_attempt()`.
- **`find_task_dir()` resolves IDs in three priority tiers**: (1) `task_id` field inside `00-spec.yaml`, (2) exact directory name, (3) `<ID>-` prefix match. The third tier explicitly skips child-suffix-shaped names (e.g. `FEAT-001` will NOT match `FEAT-001-a-foo`) so parent and child IDs do not collide. Multiple matches abort with `AMBIGUITY ERROR`.
- **`PROJECT_ROOT` is dynamic.** It calls `git rev-parse --show-toplevel` so the same code works from a worktree subdirectory or a cwd that's not the repo root. `SCHEMAS_DIR`, `POLICIES_DIR`, and `TEMPLATES_DIR` are anchored to the *tool installation* (relative to the file), not the project, because consumers run Quorum against their own repos.

### Skills are single-phase units (Rule #9)

Each `/q-*` skill executes exactly one phase and stops. Skills NEVER chain into the next skill. The only exception is **forward CLI auto-transitions**, and only these three are authorized:

| Skill | Auto-runs on success | Effect |
|-------|----------------------|--------|
| `/q-brief` | `quorum task blueprint <ID>` | inbox → active |
| `/q-decompose` | `quorum task split <PARENT_ID>` | materialise children |
| `/q-blueprint` | `quorum task start <ID>` | create worktree + branch |

`q-analyze`, `q-implement`, `q-verify`, `q-review`, `q-accept`, `q-memory`, `q-status` have NO auto-transition. Rollback (`quorum task back`) is **always human** — no skill is ever permitted to call it.

If you find a skill that auto-chains into another skill or runs `back`, that is a bug against the constitution, not a feature.

### Decomposition: parents and children

A large feature can be split via `/q-decompose`, which writes a `decomposition: [...]` block into the parent's `00-spec.yaml`. `quorum task split <PARENT>` then materialises children with IDs `<PARENT>-a`, `<PARENT>-b`, ... (single lowercase letter — pattern enforced by `CHILD_ID_RE`/`PARENT_ID_RE` in `task_manager.py`).

- The parent stays in `active/` as a coordinator and is never implemented directly.
- Each child runs its own complete lifecycle, in its own worktree (`ai/<PARENT>-<x>` branch), and merges to `main` independently.
- `quorum task clean <PARENT>` refuses to archive the parent until all children listed in `decomposition` are in `done/`.
- Schemas accept child IDs (regex matches both `^[A-Z]+-[0-9]+$` and `^[A-Z]+-[0-9]+-[a-z]$`).

### Worktrees and branches

Every task gets `worktrees/<ID>/` on branch `ai/<ID>`. `worktrees/` is gitignored. `quorum task back` deletes the worktree (and the branch if it has no unique commits) — be cautious if you have unpushed work on a feature branch.

The base branch is detected dynamically by `get_base_branch()`: tries `origin/HEAD`, falls back through `main`/`master`/`develop`/`trunk`, finally falls back to current branch.

### Risk and routing (signal-based, never magic numbers)

- `.agents/policies/risk.yaml` defines `sensitive_paths` (binary glob signals) and named risk signals (advisory).
- `.agents/policies/routing.yaml` maps `risk → executor_level`. Executor levels (0/1/2) live in `.agents/config.yaml` with primary/fallback/secondary models. **Never hardcode model names in scoring or routing logic** — that's what the level abstraction is for.
- `.agents/cli/core/risk_scorer.py` is a pure function: glob-matches `sensitive_paths` (any hit → high), then thresholds on file count (>5) and symbol count (>2) for medium. It NEVER overwrites human-declared risk in `00-spec.yaml`. Divergence emits a `risk_level_divergence` event into `07-trace.json` instead of silently correcting.
- `.agents/cli/core/failure_lookup.py` queries `.ai/tasks/failed/` for tasks whose `affected_files` overlap ≥50% with the new blueprint, surfacing past validation excerpts and review notes as risks for the new blueprint.
- `.agents/cli/core/blueprint_context.py` wires the retrievers (`ast_neighbors.py`, `import_graph.py` under `.agents/retrievers/`) to enrich a draft blueprint with neighboring files and import graph.

### Memory is curated, never automatic

`memory/` is a knowledge library, NOT an activity log (the activity log is `07-trace.json`). Entries are typed (`pattern` / `decision` / `lesson`), not graded. The schema field `supersedes` records causal corrections — superseded files are kept, never deleted. `q-memory` is the only ingestion path; there is no auto-capture, and proposals to add one are rejected by the manifesto.

External semantic stores (HSME, vector DBs) may *consume* `memory/*.json` read-only, but Quorum itself is local-first and does not depend on or write to them.

## The Constitution (immutable rules)

These are enforced by both the manifesto and the code paths above. Violations are not refactor opportunities — they're bugs.

1. **Git is the code truth.** Memory holds patterns; Git holds code.
2. **Deterministic context.** Agents receive the contract's `context_bundle`, never the whole repo.
3. **No patches outside the contract.** Touching files outside `02-contract.yaml.touch` rejects the task.
4. **Validation is finality.** Done means `verify.commands` returned 0. No diagnostic agent can waive this.
5. **Machine-first artifacts.** YAML for planning, JSON for capture. Markdown only in `docs/adr/` and external docs.
6. **The system commits, never merges.** Merging to `main` is human-only.
7. **Cost is bounded by policy.** Routing/retries/escalations are dispatcher decisions, never agent self-judgments.
8. **Tests are the only proof.** Specs and blueprints don't prove functionality.
9. **Skills are single-phase.** See the auto-transition table above. The orchestrator dispatches; skills don't.

## Skill output protocol

When editing or writing `.agents/skills/q-*/SKILL.md`, every skill must:

- **Output in Spanish** to the user, regardless of input or doc language.
- **End every waiting turn** with the exact line `ESPERANDO RESPUESTA DEL USUARIO...` (uppercase, three dots, nothing after — no trailing fence).
- **Mark next-step actions** as `[Obligatorio]` or `[Opcional]` and reference `quorum task back <ID>` as the human rollback path.
- **Never auto-activate** another `/q-*` skill. The only authorized auto-action is the forward CLI transition for the three skills listed above.

## Python and tooling

- Python `>=3.13` (pinned in `.python-version`).
- Dependencies: `jsonschema`, `pyyaml`. Dev: `pytest>=9.0.3`. Managed by `uv`.
- The package layout is unusual: `pyproject.toml` declares `package-dir = {"" = ".agents"}`, so imports like `from cli.core import task_manager` and `import retrievers.ast_neighbors` work from `.agents/` as the source root. Tests are at the repo root (`tests/`) and rely on cwd being the repo root for relative paths like `Path(".agents/schemas/validation.schema.json")`.
