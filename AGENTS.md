This file provides guidance when working with code in this repository. It is intentionally written in English for broad agent interoperability; user-facing `/q-*` skill output remains Spanish as specified below.

## Project nature

Quorum is a Go framework that orchestrates AI agents through a **Spec-Driven Contracts (SDC)** lifecycle. It is **NOT** a chatbot or general assistant — it converts human intent into validated artifacts (`00`→`07`) and verified Git diffs. The framework is dogfooded: changes to Quorum's own code go through `go test ./...`, but feature work in *consumer* projects goes through the full `/q-*` skill lifecycle.

The canonical authority is `quorum.md` (the manifesto v1.1). When the manifesto and the code disagree, the manifesto wins and the code is wrong.

## Monorepo layout (two Go modules)

This repository is a `go.work` workspace with **two independent Go modules** (see `docs/adr/0008-fusion-monorepo-capa-semantica-hsme.md`):

- **`quorum` (root module)** — the SDC orchestrator. Pure Go, `CGO_ENABLED=0`, `modernc.org/sqlite`. Everything in this document refers to this module unless stated otherwise. Build: `go build -o quorum .`; test: `go test ./...`.
- **`github.com/hsme/core` under `semantic/`** — HSME, the opt-in semantic memory engine (formerly the standalone `mcp-semantic-memory` repo, merged with full history). Requires **CGO + build tags `sqlite_fts5 sqlite_vec` + a running Ollama** (`nomic-embed-text`, `phi3.5`). It is built/tested only from `semantic/` via its own `just` recipes (`cd semantic && just install` / `just test`), never from the root.

**Hard rules for working across the two modules:**

1. **No cross-module imports.** The core module must never import `semantic/` packages, and vice versa. The integration contract is *data and protocol* (`memory.schema.json`, the `~/.quorum/memory.db` SQLite schema, the HSME MCP tool surface) — not Go code. The CI acid test: the core builds and passes `go test ./...` with `CGO_ENABLED=0`, no C compiler, and `semantic/` absent.
2. **HSME is subordinate.** HSME informs; Git, lifecycle artifacts, and curated `q-memory` decide. HSME is never code truth, never a validation gate, never an ingestion path into curated memory, and data flow is unidirectional (HSME may read Quorum's memory DB read-only; never the reverse).
3. **Do not drag CGO/Ollama into the core.** Any change that makes `quorum` require a C compiler or a runtime daemon violates ADR 0008.
4. **When editing inside `semantic/`**, follow HSME's own conventions and `semantic/CLAUDE.md`; the `/q-*` lifecycle and the rest of this document govern the core module, not HSME's internals.

## Commands

```bash
# Run the full test suite
go test ./...

# Run a single test file or test
go test ./internal/core -run TestPartitionFeedbackFindings

# Build Quorum globally
go build -o quorum .
# (Then move the binary to your PATH)

# Initialize Quorum scaffolding inside another project (creates .ai/tasks/, SQLite setup, .gitignore entries)
quorum init

# Validate one artifact file against its schema without saving it
quorum validate path/to/00-spec.yaml

# Local CLI invocation
./quorum <command>           # e.g. ./quorum task list
```

The Quorum binary is built using Go. It replaces the legacy Python entry points.

`quorum init` creates the task directories, initializes SQLite memory setup via `.quorumrc`, scaffolds `.agents/{skills,schemas,policies}` and `.agents/config.yaml` from Quorum resources when available, creates/updates `.claude/skills` as a symlink to the local `.agents/skills`, and adds `.gitignore` rules for worktrees and runtime task directories.

### Task CLI surface

State-mutating commands (use these in tests and tooling, not the skills):

```bash
quorum task specify <ID>            # creates .ai/tasks/inbox/<ID>-new-spec/00-spec.yaml
quorum task blueprint <ID>          # inbox/ -> active/ (auto-run by /q-brief)
quorum task split <PARENT_ID>       # materialises children from spec.decomposition (auto-run by /q-decompose)
quorum task start <ID>              # creates worktree + ai/<ID> branch (auto-run by /q-blueprint)
quorum task clean <ID>              # archives to done/ and removes worktree
quorum task back <ID>               # human-only rollback of the last forward transition
quorum task retry-prepare <CHILD_ID>  # failed/ -> active/ for a failed CHILD (requires parent_task); keeps 07-trace append-only. Human/orchestrator-initiated only (ADR 0001)
quorum task feedback-consume <ID>   # removes feedback.json once its findings have been consumed
quorum task artifact-save <ID> <relpath>  # reads stdin, validates against schema, persists
quorum task list
quorum task status <ID>
```

### Analyze CLI surface

Read-only analytical helpers under `quorum analyze`. They never mutate task state and each reads a **JSON request from stdin** (not positional args/flags) — they exist to be called programmatically by skills and the orchestrator. Implementations live in `cmd/analyze_*.go` (thin shims over `internal/core/*`):

```bash
quorum analyze risk-score             # stdin: blueprint + policy -> risk signal (internal/core/risk.go)
quorum analyze failure-lookup         # stdin: blueprint -> related failed tasks (internal/core/failure_lookup.go)
quorum analyze blueprint-context      # stdin: draft blueprint -> retriever neighbors + import graph (internal/core/blueprint_context.go)
quorum analyze feedback-partition     # stdin: findings JSON -> {mechanical, semantic} split (internal/core/feedback.go)
quorum analyze decomposition-coverage # stdin: parent_spec_path -> parent<->child coverage report (internal/core/decomposition_analysis.go)
quorum analyze decomposition-render   # stdin: decomposition -> deterministic ASCII DAG (internal/core/decomposition_render.go)
quorum analyze acceptance-coverage    # stdin: spec_path + blueprint_path -> acceptance-id<->test_scenario coverage report (internal/core/acceptance_coverage.go)
quorum analyze complexity-score       # stdin: blueprint + policy -> advisory S/M/L complexity band + signals (internal/core/complexity_score.go)
quorum analyze contract-check         # stdin: contract_path + changed_files + diff_stat + optional file_diffs -> {ok, violations, not_checked} touch/forbid/limits (incl. optional per-class) gate (internal/core/contract_check.go)
```

If examples inside older skill documents disagree with this section, the Go CLI contract wins: inspect `cmd/analyze_*.go` or run `quorum analyze <command> --help`, then send the documented JSON request through stdin.

### Fleet CLI surface

`quorum fleet` is a new command group for headless-delegate dispatch helpers, distinct from `quorum analyze` (`quorum analyze fleet-preflight` is untouched and stays under `analyze`):

```bash
quorum fleet bundle <ID>  # writes a deterministic dispatch context bundle + manifest under .ai/tasks/active/<ID>/dispatch/<dispatch_id>/ (internal/core/fleet_bundle.go)
quorum fleet dispatch     # stdin JSON {task_id, agent, model, bundle_path, timeout_s?, dispatch_id} -> runs a delegated CLI in the task worktree with lock, process-group-kill timeout, forensic ref, ADR 0011 outcome class, and a normalized result.json (internal/core/fleet_dispatch.go)
quorum fleet run          # NON-LIFECYCLE: runs a transport in an explicit --cwd via core.RunDelegate; no task, worktree, git, forensic ref, 07-trace, or result.json (cmd/fleet_run.go)
```

#### Agent usage (`quorum fleet run`, mk-cli contract)

`quorum fleet run` is the agent-friendly, **non-lifecycle** standalone runner. It executes an
agent transport (default `agy`) in an explicit `--cwd` and returns the delegate result. It is
NOT `quorum fleet dispatch`: `run` is task-less and produces no SDC artifact, forensic ref, or
git side effect; `dispatch` is task-bound and runs the full forensic pipeline against a worktree.

Default agent flags:

- Always pass `--json` (stable `{ok, command, summary, data, next_actions}` envelope; errors are
  `{ok:false, command, error:{code, message, field, received}, retryable, suggested_fix}`). Under
  `--json` stdout is exactly one JSON object; all logs go to stderr.
- Always pass `--no-input`; supply the prompt via `--input <file>` or `--input -` (stdin). There is
  no inline prompt flag.
- `--model` is a **closed enum** derived from the transport's models map; an unknown value is
  rejected with `INVALID_ENUM` listing the valid names. Run `quorum fleet run --schema` to see it.
- Use `--dry-run` to resolve/validate the argv without starting a process; `--output <file>` to
  redirect large results (returned as `data.result_file`); `--timeout <s>` to bound the run
  (a timed-out delegate returns `TIMEOUT`).
- Stable error codes: `MISSING_REQUIRED_FLAG`, `INVALID_ENUM`, `FILE_NOT_FOUND`, `TIMEOUT`,
  `INVALID_ARGUMENT`, `INTERNAL_ERROR`.

```bash
quorum fleet run --schema
quorum fleet run --agent agy --model anthropic/claude-sonnet-4-6 --cwd . --input - --no-input --json
quorum fleet run --agent agy --model anthropic/claude-opus-4-6 --cwd /repo --input prompt.txt --dry-run --json
```

For $0 delegate runs, the `opencode` transport pins five OpenRouter free models plus the
`openrouter/free` auto-router as the availability fallback; the `aider` transport pins six
models — the same five plus `nvidia/nemotron-nano-9b-v2-free` (aider-only; no auto-router).
Canonical keys substitute `-free` for OpenRouter's `:free` suffix (the agents.schema.json key
pattern forbids `:`), the authoritative list is `quorum fleet run --agent opencode --schema`
(or `--agent aider --schema`). A 2026-07-15/16 pass@10 campaign (N=10/cell, hidden test, 21
cells) found `nano-9b-v2` reliable under aider's edit harness (9/10) but unreliable agentically
(3/10, why it was dropped from opencode) — full evidence in `docs/fleet-run-for-agents.md` §7.
A second, harder M-difficulty layer of that campaign (2026-07-16, same N=10/cell methodology)
found aider unreliable at M difficulty — two cells scored **0/10** on a two-file task — while
opencode and agy (Gemini) stayed reliable; prefer opencode/agy over aider for anything beyond
trivial single-file edits (§7.2/§4.1). That M layer also surfaced a `quorum fleet run`
bug (since fixed): its placeholder guard used to false-positive when the prompt itself
contained literal braces (e.g. Go code); the guard now scans only the raw argv template
before substitution, so prompt content with `{`/`}` passes through untouched (§4.1).
OpenRouter free-tier limits bind every `:free` call: 20 req/min shared account-wide, 1000
req/day on this account (≥ $10 lifetime purchased credits; 50/day otherwise), and 429s COUNT
against the daily quota — space probes, never retry-loop a 429, and avoid concurrent agentic
runs on free models.

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
| SQLite (Memory) | DB | `memory.schema.json` | `/q-memory` via `quorum memory save` |

There is **no `03`, `08`, `09`, or `10`**. The manifesto rejects new lifecycle slots: failure data lives in `05/06/07` and SQLite `lessons`, and impact reports go through `q-memory`. Do not propose new numbered artifacts without an ADR.

### Where state actually changes

`internal/core/task_manager.go` owns nearly all state mutations and is the first place to inspect when task state changes unexpectedly. The CLI commands (`cmd/task*.go`, `cmd/init.go`) are thin shims. When in doubt, read `task_manager.go` first.

Important invariants enforced there (Go identifiers, grep-able as written):

- **`SaveArtifact()` validates before writing.** Any `task artifact-save` (or skill that persists via this path) is schema-checked before the file is written. The validation engine itself lives in `internal/core/schema.go` (`ValidateArtifact`, keyed by `artifactSchemaMap`); `SaveArtifact` in `task_manager.go` only orchestrates the write. Failure raises `ArtifactValidationError` with a `field=$.path; reason=...` format (Python-compatible messages built by `pythonReason`/`jsonPointer` in `schema.go`).
- **`07-trace.json` is append-only.** `EnsureTraceAppendOnly()` rejects any save that shortens or rewrites existing `attempts[]` or `events[]`. New attempts/events are appended by persisting the grown payload through `SaveArtifact` — there is no separate append helper. A delegated `q-implement` dispatch is recorded in `attempts[]` with `phase: "execute"` (see `docs/adr/0011-attempt-reroute-blocked-trace.md`).
- **`FindTaskDir()` resolves IDs in three priority tiers**: (1) `task_id` field inside `00-spec.yaml`, (2) exact directory name, (3) `<ID>-` prefix match. The third tier explicitly skips child-suffix-shaped names (e.g. `FEAT-001` will NOT match `FEAT-001-a-foo`) so parent and child IDs do not collide. Multiple matches abort with `AMBIGUITY ERROR`.
- **`ProjectRoot()` is dynamic.** It calls `git rev-parse --show-toplevel` and then falls back to walking upward for `.git`, so the same code works from a worktree subdirectory or a cwd that's not the repo root.
- **Schema lookup and init resources are separate concerns.** `SchemasDir()` first honors `QUORUM_SCHEMAS_DIR`, then searches the project root/current working directory and their ancestors for `.agents/schemas`. `quorum init` resources are resolved by `getResourceSrc()` from a usable `.agents` bundle near the project root, binary, source tree, or fallback project root.

### Artifact and task-state editing rules

- Prefer `quorum task artifact-save <ID> <relpath>` when persisting lifecycle artifacts, because it validates before writing and preserves special invariants such as append-only trace attempts.
- Use `quorum validate <artifact-path>` for local preflight validation when you need to inspect schema errors without mutating task state.
- Do not manually edit `07-trace.json` attempts, move task directories between `.ai/tasks/*`, or remove worktrees to force state transitions. Use the task CLI, or leave rollback/retry decisions to the human/orchestrator paths documented here.
- Runtime task directories under `.ai/tasks/{inbox,active,done,failed}` are gitignored except `.gitkeep`; durable knowledge belongs in curated centralized SQLite memory, not in ad-hoc task-state edits.

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

A large feature can be split via `/q-decompose`, which writes a `decomposition: [...]` block into the parent's `00-spec.yaml`. `quorum task split <PARENT>` then materialises children with IDs `<PARENT>-a`, `<PARENT>-b`, ... (single lowercase letter — pattern enforced by `CHILD_ID_RE`/`PARENT_ID_RE` in `task_manager.go`).

- The parent stays in `active/` as a coordinator and is never implemented directly.
- Each child runs its own complete lifecycle, in its own worktree (`ai/<PARENT>-<x>` branch), and merges to `main` independently.
- `quorum task clean <PARENT>` refuses to archive the parent until all children listed in `decomposition` are in `done/`.
- Schemas accept child IDs (regex matches both `^[A-Z]+-[0-9]+$` and `^[A-Z]+-[0-9]+-[a-z]$`).

### Worktrees and branches

Every task gets `worktrees/<ID>/` on branch `ai/<ID>`. `worktrees/` is gitignored. `quorum task back` deletes the worktree (and the branch if it has no unique commits) — be cautious if you have unpushed work on a feature branch.

The base branch is detected dynamically by `GetBaseBranch()` (`task_manager.go`): tries `origin/HEAD`, falls back through `main`/`master`/`develop`/`trunk`, finally falls back to current branch.

### Risk and routing (signal-based, never magic numbers)

- `.agents/policies/risk.yaml` defines `sensitive_paths` (binary glob signals) and named risk signals (advisory).
- `.agents/policies/routing.yaml` maps `risk → executor_level`. Executor levels (0/1/2) live in `.agents/config.yaml` with primary/fallback/secondary models. **Never hardcode model names in scoring or routing logic** — that's what the level abstraction is for.
- `internal/core/risk.go` is a pure function: glob-matches `sensitive_paths` (any hit → high), then thresholds on file count (>5) and symbol count (>2) for medium. It NEVER overwrites human-declared risk in `00-spec.yaml`. Divergence emits a `risk_level_divergence` event into `07-trace.json` instead of silently correcting.
- `internal/core/failure_lookup.go` queries `.ai/tasks/failed/` for tasks whose `affected_files` overlap ≥50% with the new blueprint, surfacing past validation excerpts and review notes as risks for the new blueprint.
- `internal/core/blueprint_context.go` wires the retrievers (`ast_neighbors.py`, `import_graph.py` under `.agents/retrievers/`) to enrich a draft blueprint with neighboring files and import graph.

### Supporting core modules

The rest of `internal/core` is small, pure, single-purpose logic exposed through the `analyze` CLI and consumed by skills:

- `internal/core/schema.go` is the JSON Schema validation engine behind `SaveArtifact`. It compiles schemas from `.agents/schemas/`, chooses the most specific validation leaf, and renders `ArtifactValidationError` messages in a Python-compatible `field=...; reason=...` shape (`pythonReason`, `jsonPointer`, `valueAt`). This is where "validation before write" actually happens.
- `internal/core/feedback.go` (`PartitionFeedbackFindings`) splits review/validation feedback into **mechanical** (machine-applicable) vs **semantic** (meaning-changing) findings. Only an explicit `category: "mechanical"` is machine-applicable; unknown or malformed categories default to **semantic** so the human stays the authority over meaning. Backs `quorum analyze feedback-partition` and `quorum task feedback-consume`.
- `internal/core/blocked_signal.go` (`ParseBlockedSignal`) parses the standardized `BLOCKED` contract signal a skill emits when it cannot proceed, so a blocked dispatch is structured data, not free text.
- `internal/core/decomposition_analysis.go` (`AnalyzeParentChildCoverage`) reports whether the materialised children cover every item declared in the parent's `decomposition`; `decomposition_render.go` (`RenderAsciiDag`) draws a deterministic ASCII DAG and is presentation-only — it never validates, mutates, or persists.

### Failed-child retry (human/orchestrator-initiated)

`quorum task retry-prepare <CHILD_ID>` (`PrepareFailedChildRetry` in `task_manager.go`) moves a **failed child** back from `failed/` to `active/` so its lifecycle can be re-run. It is deliberately narrow: it refuses non-child tasks (no `parent_task`), refuses to clobber an existing `active/` copy, and requires `07-trace.json` to exist so the append-only attempt history is preserved. Per **`docs/adr/0001-q-implement-child-retry.md`**, retry is always initiated by a human or the orchestrator — never decided autonomously by a skill — and never implies auto-merge or auto-`back`. Automatic contract renegotiation remains deliberately deferred (**`docs/adr/0002-defer-contract-renegotiation-protocol.md`**).

### Memory is curated, never automatic

The centralized SQLite memory is a knowledge library, NOT an activity log (the activity log is `07-trace.json`). Entries are typed (`pattern` / `decision` / `lesson`), not graded. The schema field `supersedes` records causal corrections — superseded entries are kept, never deleted. `q-memory` is the only ingestion path via `quorum memory save`; there is no auto-capture, and proposals to add one are rejected by the manifesto.
`q-session` acts as a second human-invoked route on top of `quorum memory save`, using `source_task=SESSION-*`. It is not auto-capture.

Quorum is user-sovereign, not local-first: local operational data belongs to the user and may be explicitly exported, deleted, reset, or rebuilt by user-approved tooling. External semantic stores (HSME, vector DBs) may integrate with exported/restored data when they remain subordinate to Git, lifecycle artifacts, validation, and curated `q-memory`; they must never become code truth or erase append-only evidence.

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
10. **User data sovereignty.** Quorum is not local-first as a constitutional constraint. User-approved tooling may export, delete, reset, or rebuild local operational data without treating external memory as code truth or bypassing trace/validation/contract invariants.

## Skill output protocol

When editing or writing `.agents/skills/q-*/SKILL.md`, every skill must:

- **Output in Spanish** to the user, regardless of input or doc language.
- **End only waiting turns** with the exact line `ESPERANDO RESPUESTA DEL USUARIO...` (uppercase, three dots, nothing after — no trailing fence). A waiting turn is one that asks an explicit user question, reports a blocked dispatch, or leaves a pending human decision; successful informational completions must omit it.
- **Persisted artifact field values MUST be written in concise English** (`00-spec.yaml`, `01-blueprint.yaml`, `02-contract.yaml`, `04-implementation-log.yaml`, `05-validation.json`, `06-review.json`, `07-trace.json`, and SQLite memory entries), even when user-facing chat stays Spanish.
- **Mark next-step actions** as `[Obligatorio]` or `[Opcional]` and reference `quorum task back <ID>` as the human rollback path.
- **Never auto-activate** another `/q-*` skill. The only authorized auto-action is the forward CLI transition for the three skills listed above.

## Python and tooling

- Quorum is built in Go and has no Python runtime dependency. The golden-master black-box harness that exercises the compiled binary's CLI contract lives in `internal/core/golden_master_test.go`, and the skill-protocol invariants live in `internal/core/skill_protocol_test.go` — both run under `go test ./...`. The two `.agents/retrievers/*.py` scripts are retained as reference; the live blueprint-context retriever logic is reimplemented natively in `internal/core/blueprint_context.go`.
