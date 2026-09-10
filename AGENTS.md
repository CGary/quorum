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

# Report artifacts: create from template / validate+persist / list (cmd/report.go)
quorum report new <id>
quorum report save <id>
quorum report list

# Read-only viewer server for projects, reports, memories, and task state (cmd/serve.go)
quorum serve            # foreground
quorum serve start      # background
quorum serve stop
quorum serve status

# Centralized SQLite memory: persist / inspect config+DB status / search (cmd/memory.go, cmd/memory_search.go)
quorum memory save
quorum memory status
quorum memory search <query>

# Read-only diagnostic: 6 checks, exit 0 clean / 1 with findings (cmd/doctor.go, internal/core/doctor.go with CollectDoctorFacts as IO and EvaluateDoctor as pure logic)
quorum doctor
quorum doctor --json

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
quorum fleet route        # stdin JSON {task_id?, phase, risk, complexity_band, incumbent_family?, exclusions?, dispatch_id?} -> resolves an executor Candidate strictly from .agents/config.yaml + .agents/policies/routing.yaml + .agents/fleet/agents.yaml + .ai/fleet-control.json (internal/core/fleet_route.go, cmd/fleet_route.go)
quorum fleet bundle <ID>  # writes a deterministic dispatch context bundle + manifest under .ai/tasks/active/<ID>/dispatch/<dispatch_id>/ (internal/core/fleet_bundle.go)
quorum fleet dispatch     # stdin JSON {task_id, agent, model, bundle_path, timeout_s?, dispatch_id} -> runs a delegated CLI in the task worktree with lock, process-group-kill timeout, forensic ref, ADR 0011 outcome class, and a normalized result.json (internal/core/fleet_dispatch.go)
quorum fleet run          # NON-LIFECYCLE: runs a transport in an explicit --cwd via core.RunDelegate; no task, worktree, git, forensic ref, 07-trace, or result.json (cmd/fleet_run.go)
quorum fleet status       # manual kill-switch: reads .ai/fleet-control.json and reports currently disabled targets, optionally as --json (internal/core/fleet_control.go, cmd/fleet_status.go)
quorum fleet enable <target>   # manual kill-switch: re-enables a disabled agent or agent/model target in .ai/fleet-control.json (internal/core/fleet_control.go, cmd/fleet_enable.go)
quorum fleet disable <target> --reason <reason>  # manual kill-switch: disables an agent or agent/model target in .ai/fleet-control.json; --reason is required (internal/core/fleet_control.go, cmd/fleet_disable.go)
quorum fleet smoke <agent> <task_id>  # LEVEL 2 manual-only real dispatch against an existing task worktree via core.Dispatch; consumes real quota and is never wired into CI, cron, or a q-* skill (cmd/fleet_smoke.go)
quorum fleet stats        # read-only: aggregates terminal (done/failed) dispatch telemetry via core.CollectDispatchRecords + core.ComputeFleetStats, grouped by cell/level/band, optionally as --json (internal/core/fleet_stats.go, cmd/fleet_stats.go); also surfaced read-only via GET /api/fleet/stats in the quorum serve dashboard
quorum fleet catalog <transport> [--json] [--timeout <s>]  # read-only, manual-only: probes the named transport binary with a sentinel model value to capture its "Available models:" rejection block, then diffs the live list against the models declared for that transport in .agents/fleet/agents.yaml (cmd/fleet_catalog.go, internal/core/fleet_catalog.go); reports status "ok" + delta, "unknown" (exit 0) when output does not parse as a catalog, or "unsupported" (exit 0) when the transport has no {model_arg} argv placeholder; the live-catalog complement of `quorum analyze fleet-preflight` (which is schema/config-only and never execs); never wired into CI, cron, or any q-* skill
```

`quorum fleet route` is the pure decision step (`internal/core/fleet_route.go`, `core.Route`):
zero hardcoded model/agent names, first-match-wins against `routing.yaml`'s `{phase, risk, band}`
matrix, deterministic candidate enumeration (`primary` -> `fallback` -> `secondary`, in
`config.yaml.policies.fleet_transport_order`), and disabled/excluded filtering before a soft
review-family diversity preference picks the final `Candidate`. When the request carries a
`task_id`, `quorum fleet route` also appends one `routing_decision` event to that task's
`07-trace.json` (a reserved ADR 0011 event type) carrying a full `inputs_snapshot` — policy file
hashes, control-state snapshot, risk, complexity band, exclusions, and router version — sufficient
to reconstruct the decision later without re-reading policy state.

`/q-dispatch` (`.agents/skills/q-dispatch/SKILL.md`) is the single-phase human face that drives
one implement-phase delegation cycle end to end: it checks preconditions (task in `active/` with a
worktree, `01-blueprint.yaml` and `02-contract.yaml` present), calls `quorum fleet route`, always
shows the router's decision (agent, model, level, signals) to the human and then — since v2
(2026-08-09, human decision) — proceeds automatically to `quorum fleet bundle` then
`quorum fleet dispatch` without waiting for confirmation (sole exception: a codex candidate still
requires an explicit human "si" — finite credits; an explicit human veto is honored as
"elegir otro"/"cancelar"), and reports the outcome per the ADR 0011 taxonomy (`attempt_done`
marks `/q-verify` as `[Obligatorio]`; `reroute` re-invokes `quorum fleet route` with the failed
candidate added to `exclusions` and auto-dispatches the next candidate after display;
`blocked` is a rich Spanish question; `attempt_failed`/`noop` present
evidence and reference `quorum task back` as the human-only rollback path). It never computes or
decides routing itself, never auto-chains into another `/q-*` skill, and never calls
`quorum task back`.

The ratified G1 cell set (see `.agents/config.yaml.levels[0]` and
`.agents/policies/routing.yaml`) pins `agy_edit` on `google/gemini-3.6-flash-low` as the primary
cheap cell for level 0 (`low` risk / `S` band). `agy_edit` (2026-07-30) is the agentic-editing
variant of the `agy` transport: same binary, argv adds `--mode accept-edits
--dangerously-skip-permissions --sandbox --add-dir {cwd}` so the delegate edits the worktree in
place (`--sandbox` WITHOUT `--add-dir` silently redirects writes to
`~/.gemini/antigravity-cli/scratch/` — never ship it alone). Since 2026-07-31 the model catalog is
SYMMETRIC across both agy transports: the gemini-3.6-flash trio exists on `agy_edit` (agentic
pass@3, FLEET-19) AND on base `agy` (one-shot smoke 3/3, task 21). Capability is declared as
policy data via a per-transport `mode: agentic | oneshot` field in `agents.yaml` (`agy` is the
only `oneshot`; absent = agentic), and `core.Route` excludes `oneshot` transports from
implement-phase candidate enumeration — this preserves the G1 cross-provider reroute guarantee
despite the symmetric catalog. Side effect: level 1's old fallback `google/gemini-3.1-pro-low`
(only on `agy`) is no longer routable for implement — it was a latent bug (a one-shot transport
can never execute an implement). That left level 1 — which owns `{low,M}`, `{medium,S}` and
`{medium,M}`, the band most SDC tasks land in — resolving to nothing routable, so every level-1
implement silently fell back to an internal Claude subagent. Level 1 was therefore REBUILT on
2026-07-31: `primary: google/gemini-3.6-flash-medium` (agentic on `agy_edit`) with
`fallback: poolside/laguna-m.1-free` keeping the G1 cross-provider invariant, and the codex cells
demoted to the tail of `secondary`. First real exercise: FLEET-032 (`risk=low`, `band=M`) routed to
the primary and completed its implement externally. Since 2026-07-31 `agy_edit` also exposes `gpt-oss-120b`, `claude-sonnet-4-6`, and
`claude-opus-4-6` (verified agentic under accept-edits, smoke 3/3), which makes level 2's
`primary: anthropic/claude-opus-4-6` resolvable as an agentic cell (the old `claude-opus-4-7`
reference was dead — no active transport exposed it). Base `agy` keeps its 3.1-pro cells for
one-shot `--print` review/analysis and rollback (gemini-3.5 cells were retired by the provider
2026-09-03, incident HEX-060; they were migrated to base `agy` from level 0 by FLEET-030 but
are no longer available).
Both agy transports run with `timeouts.default_s: 600` (raised from 300 after a real hexcell
dispatch was killed mid-work at 300s, 2026-07-31). The $0 OpenRouter cross-provider cells are
`opencode`-backed `nvidia/nemotron-3-ultra-550b-a55b-free` and `poolside/laguna-s-2.1-free`
(2026-08-09: `laguna-m.1` was removed from OpenRouter — probes returned "Unexpected server
error" — and was swapped for `laguna-s-2.1`, the larger of the two live free lagunas, in
`agents.yaml` and both route slots). Since the 2026-08-09 free-first reordering (human decision),
these free cells LED levels 0 and 1 (chains as of 2026-08-09, superseded by the rebalance below; level 0: nemotron → laguna → gpt-oss-120b →
gemini-3.5-flash-low; level 1: laguna → nemotron → gpt-oss-120b → gemini-3.6-flash-low; primary
and fallback share the OpenRouter rate limit, the cross-provider jump happens at secondary) and
the Gemini-subscription cells close each chain as backstops; `aider` stays restricted to
mechanical single-file changes and sits mid-order (`[agy_edit, agy, opencode, aider, codex,
claude]`) in `fleet_transport_order`. This cell set is expressed only as policy data
(`config.yaml`, `routing.yaml`, `agents.yaml`); `core.Route` never hardcodes any of it.

**2026-08-26 ladder rebalance (human decision): the matrix now has FOUR levels.** The
gemini-3.7-flash trio and the gemini-3.1-pro duo were exposed on `agy_edit` (agentic smoke
campaign 2026-08-26; the 3.7 trio also on one-shot `agy`), and levels were rebuilt on three
human rules: (1) free cells do the bulk and the Anthropic pair are RARE RESCUERS at the tail of
level 1 only — a tail cell runs only after every cheaper cell failed the same task, so its
expected cost is ~0; (2) one model, one home (no duplicate cells across levels; free cells
laguna-s/gpt-oss may repeat across 0/1); (3) proven-before-new — 3.6-flash-high leads level 2
ahead of the unproven 3.7 cells, and 3.1-pro (zero agentic history) is the level-3 fallback,
never primary. Chains: level 0 = nemotron-3-ultra → nemotron-3-super-120b → [cohere/north-mini-code,
nemotron-3-nano-omni]; level 1 = claude-sonnet-4-6 → claude-opus-4-6 → [nvidia/nemotron-3-ultra]
(sonnet leads per 2026-08-27 human decision; gpt-oss-120b removed as redundancy); level 2
({high,S/M} and {low/medium,L}) = 3.6-flash-high → 3.7-flash-medium; level 3
({high,L}, catch-all, migration/security overrides — all with `human_gate_required: true`) =
3.7-flash-high → 3.1-pro-high (superseded 2026-09-08, see below). Levels 2-3 carry no Claude cell and no free tail:
an exhausted chain BLOCKS and returns to the human instead of degrading. The codex cells were
removed from every route slot (subscription retired; catalog entries and the kill-switch state
remain).

**2026-09-03 (provider retirement, incident HEX-060):** Gemini 3.5 Flash was retired by the
provider. All `google/gemini-3.5-flash-{low,medium,high}` entries have been removed from both
`agy` and `agy_edit` model catalogs, from `config.yaml` level 1 secondary and level 2 primary.
Level 2 now enters on `3.6-flash-high` (proven agentic) → `3.7-flash-medium`. The
interim kill-switch entries for `agy_edit/google/gemini-3.5-flash-{high,medium}` were cleared with
`quorum fleet enable` after the merge (2026-09-04). Gemini 3.8 Flash is live on the provider
(`quorum fleet catalog agy` reports the trio as `live_undeclared`) but is **not** in the catalog,
pending a smoke campaign (proven-before-new rule applies; no routing change implied). The
hexcell copy of `config.yaml` / `agents.yaml` / `agents.schema.json` is NOT updated by this
repo's merge — propagate by hand.

**2026-09-08 (human decision): DeepSeek enters level 3; Gemini 3.1 Pro retired.**
`opencode-go/deepseek-v4-pro` (transport `opencode_go`, OpenCode Go subscription) passed an
agentic smoke pass@10 = 10/10 on the M-task hidden test (2026-09-07, 15-27 s/trial;
`docs/fleet-run-for-agents.md` section 7.8) and REPLACES `google/gemini-3.1-pro-high` as the
level-3 fallback (chain: 3.7-flash-high → deepseek-v4-pro). The four `gemini-3.1-pro-*` catalog
entries (1/4 dispatch success, one timeout) were removed from both agy transports. Level 3 is
now cross-provider by construction: primary on the Antigravity subscription, fallback on the
OpenCode Go subscription, so an exhausted Antigravity quota no longer leaves it without a
routable cell. The same day (human decision, "option 2": route by decision, smoke pending) the
OpenCode Go placement ratified on 2026-09-06 was applied and then PROMOTED TO PRIMARY of every
level (Antigravity quota exhausted; OpenCode Go is the live subscription). Chains as of
2026-09-08: level 0 = deepseek-v4-flash → nemotron-3-super-120b → [north-mini-code]; level 1 =
qwen3.7-plus → claude-sonnet-4-6 → [claude-opus-4-6, nemotron-3-super-120b]; level 2 =
minimax-m3 → 3.6-flash-high → [3.7-flash-medium]; level 3 = deepseek-v4-pro → gpt-5.6-luna →
[3.7-flash-high]. Only deepseek-v4-pro has smoke evidence (section 7.8); the other four Go
cells carry NONE and their first real dispatches are the evidence — watch `quorum fleet stats`. Also on 2026-09-07:
`nemotron-3-ultra-550b`, `laguna-xs-2.1` and `nemotron-3-nano-omni` were removed from the
catalog for latency; level 0 is now `super-120b → north-mini-code` with no secondary.

**2026-09-09 (human decision): Gemini/Antigravity retired, free cells dropped, ladder rebuilt on
OpenCode Go.** The Antigravity subscription no longer exists — the same situation that retired
codex on 2026-07-27 — and the human additionally dropped the $0 OpenRouter cells. Applied in one
pass:
(1) **Kill-switch + deactivation.** `agy` and `agy_edit` are disabled in `.ai/fleet-control.json`
(quorum AND hexcell) and set `active: false` in `agents.yaml`; `codex`, `opencode` and `aider` are
`active: false` too. Blocks are kept intact (verified argv, model_args, effort whitelists,
`wrapper_signatures`) so a returning subscription is a one-line change — the `claude` precedent.
`opencode_go` is the ONLY active transport. Retiring `agy_edit` also retires
`anthropic/claude-sonnet-4-6` and `claude-opus-4-6`, which existed only there.
Both mechanisms are needed, and closing the second one required a code change. The kill-switch is
per-project and honored only by `route`/`dispatch`; `quorum fleet run` honors neither it nor —
until this change — `active:`. So a skill with a hardcoded `--agent agy` could still exec a dead
provider after every policy file had retired it (verified: `fleet run --agent agy_edit --dry-run`
returned `ok:true` and would have exec'd `agy`). `cmd/fleet_run.go` now refuses an inactive
transport with `INVALID_ARGUMENT`, mirroring the check `fleet dispatch` has had since FLEET-018.
`active: false` is now genuinely the global "this transport does not exist any more" switch.
(2) **A kill-switch bug fixed on the way.** `core.ValidateFleetTarget` resolved `agents.yaml` as
`<projectRoot>/.agents/fleet/agents.yaml` only, while `cmd.fleetAgentsPath` honors
`QUORUM_FLEET_AGENTS` first. In a consumer project that shares Quorum's catalog through the env
var and has no `.agents/fleet/` of its own (hexcell), `quorum fleet disable` therefore failed
while `route`/`dispatch` worked. `core.FleetAgentsPath` now mirrors the dispatch resolver
(`TestFleetAgentsPathEnvOverride`).
(3) **The ladder is now eight OpenCode Go cells, two per level, no secondary anywhere** (human's
own ordering): level 0 `deepseek-v4-flash` → `qwen3.8-flash`; level 1 `minimax-m3` → `hy3`;
level 2 `deepseek-v4-pro` → `kimi-k2.7-code`; level 3 `kimi-k3` → `grok-4.6`. Five cells were new
to the catalog and required three new `provider` enum values (`opencode-go-tencent`,
`opencode-go-moonshot`, `opencode-go-xai`). `fleet_transport_order` is `[opencode_go, claude]`.
`opencode_go`'s `timeouts.default_s` went 300 → 600: HEX-063 killed `minimax-m3` at exactly
300.06 s mid-work and rerouted to Gemini — the same evidence that raised agy's timeout in July.
`quorum fleet run`'s default `--agent` flipped `agy` → `opencode_go`.
(4) **The level-3 human gate is GONE** (`human_gate_required: false` on `{high,L}`, the catch-all
and both `type_overrides`). Two reasons, one a defect: the human wants uniform behaviour across
levels, AND the flag was never enforced — `cmd/fleet_route.go` documents
`reviewer_required`/`human_gate_required`/`type_overrides`/`routes` as silently ignored, and
`q-dispatch` gates only on a codex candidate. Declaring a control the system does not implement is
worse than not having it; rebuilding it means code first, data second.
(5) **ACCEPTED RISK, recorded.** Every routed cell now hangs off ONE subscription. Each level is
still cross-FAMILY (deepseek→alibaba, minimax→tencent, deepseek→moonshot, moonshot→xai), which is
what the G1 test asserts, but there is no independent quota class anywhere: an exhausted OpenCode
Go quota blocks the whole fleet. AC-4 (`TestFleetRouteLevel1DegradesWhenCodexDisabled`) was
rewritten to assert what the fleet can actually guarantee — disabling a primary CELL degrades to
the fallback — and its final step now PINS the transport-level block as observed behaviour, so
reintroducing a second live transport fails the test and forces the stronger assertion back.
(6) **Smoke, two stages, informative not gating.** Stage 1 (name verification) is the answer to
"is the model_arg right?" — `quorum fleet catalog opencode_go` cannot answer it (`status:
"unknown"`; opencode prints no parseable "Available models:" block), so each cell got one trivial
probe: **8/8**, 4-7 s, no rejection signature, nothing written. Stage 2 is the section 7.1 M-layer
pass@5 campaign; per human decision ("rutear las 8 igual, smoke solo informativo") the ladder was
routed BEFORE it finished, so it documented rather than gated — a second deliberate suspension of
the proven-before-new rule in two days. It came back **39/40**: seven cells 5/5, and
`deepseek-v4-flash` 4/5 (one 300 s TIMEOUT with zero bytes on trial 1, then 15-25 s passes on
trials 2-5 — a transient provider hang, but it is the level-0 primary, so watch it). Every passing
trial scored 15/15 hidden subtests and wrote exactly the two requested files. Two operational
facts worth carrying: `kimi-k3` (level-3 primary) is the slowest and most variable cell
(45/99/266 s min/median/max) and only fits because `timeouts.default_s` was raised to 600 s in
this same change — do NOT lower it while k3 leads level 3; and all 40 trials are now in the shared
ledger, so `rung0-cells.py` reports real evidence for every routed cell instead of `0/0`. Full
tables: `docs/fleet-run-for-agents.md` section 7.8.
(7) **Skills updated with the fleet** (the ladder is written down in several of them):
`.agents/skills/{fleet-cli-usage,q-blueprint,q-dispatch}`, and globally
`~/.claude/skills/{think-cheap,fleet-delegate,q-orchestrate}` plus think-cheap's
`references/capability.yaml`. fleet-delegate's ladder collapsed from five rungs to two (the USD-0
rungs and the one-shot `agy` rung are gone); **no one-shot external cell exists any more**, since
`agy` was the only `mode: oneshot` transport — one-shot work now runs agentically in a scratch
`--cwd` with a `git status --porcelain` check, or goes internal. The hexcell
copies were handled as follows, because the note has been wrong in both directions before: its
`.ai/fleet-control.json` (kill-switch), `.agents/config.yaml` and `.agents/policies/routing.yaml`
WERE updated in place on 2026-09-09 and now carry the 8-cell ladder and the gate removal — do not
re-apply them. hexcell has no `.agents/fleet/agents.yaml` of its own (it reads Quorum's through
`QUORUM_FLEET_AGENTS`), so the catalog needs nothing. What DOES need propagating on every future
change is hexcell's own `.agents/skills/` copies: they are a separate tree from this repo's, they
are what `/q-*` actually loads there (`.claude/skills` symlinks to them), and on 2026-09-09
`q-blueprint`, `fleet-cli-usage` and `q-dispatch` had to be copied over by hand after the quorum
originals were fixed.

**Tooling shipped with the retirement (FLEET-036 / FLEET-037, merged 2026-09-04).**
(1) `wrapper_signatures` — a per-transport list in `agents.yaml` (validated by
`agents.schema.json`, sibling of `failure_signatures`) of case-sensitive substrings matched
against the delegate's output. A hit with an empty diff classifies the dispatch as
`reroute` / `wrapper_broken` (ADR 0011) instead of `attempt_failed`: a transport rejecting an
unknown model ("invalid model selection", "is not recognized as a known model" on both agy
transports) now consumes `reroute_budget`, not the contract's `max_attempts`, and is not counted
as a model failure in `quorum fleet stats`. Precedence in `classifyOutcome`
(`internal/core/fleet_dispatch.go`): non-empty diff → attempt; then quota
(`failure_signatures`) → timeout → wrapper. An absent list is legacy-compatible (never matches).
(2) `quorum fleet catalog <transport>` (`cmd/fleet_catalog.go`; pure `ParseAvailableModels` /
`DiffCatalog` in `internal/core/fleet_catalog.go`) probes the transport binary with a sentinel
model, parses the "available models" list it prints on rejection, and reports `declared_dead`
(in `agents.yaml`, not live) and `live_undeclared` (live, not in `agents.yaml`). Read-only,
manual-only, one rejected call of quota. Operational rule: run it before any ladder rebalance,
and whenever a dispatch fails within seconds with `exit_code` 1 read `notes.txt` before drawing
any conclusion about the model — a three-second failure is configuration, not reasoning, and
must never poison the ledger that drives routing. Known gap: `quorum analyze fleet-preflight`
check 2 still flags every cell exposed on both agy transports as `ambiguous` (idea 14,
unimplemented); those errors are noise until the check mirrors the router's `oneshot` exclusion.

#### Agent usage (`quorum fleet run`, mk-cli contract)

`quorum fleet run` is the agent-friendly, **non-lifecycle** standalone runner. It executes an
agent transport (default `opencode_go`; `agy` until 2026-09-09) in an explicit `--cwd` and returns
the delegate result. Since 2026-09-09 it REFUSES a transport with `active: false`. It is
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
# --model is a closed enum and the catalog churns: read the name from --schema,
# never from a literal written in this file.
quorum fleet run --agent opencode_go --schema
quorum fleet run --agent opencode_go --model <key from --schema> --cwd . --input - --no-input --json
quorum fleet run --agent opencode_go --model <key from --schema> --cwd /repo --input prompt.txt --dry-run --json
```

HISTORICAL (both transports are `active: false` since 2026-09-09 and `fleet run` refuses them;
kept for the measured evidence below and for a possible future $0 tier). The `opencode` transport
pinned five OpenRouter free models plus the `openrouter/free` auto-router as the availability
fallback; `aider` pinned six — the same five plus `nvidia/nemotron-nano-9b-v2-free` (aider-only;
no auto-router). Canonical keys substitute `-free` for OpenRouter's `:free` suffix (the
agents.schema.json key pattern forbids `:`). A 2026-07-15/16 pass@10 campaign (N=10/cell, hidden test, 21
cells) found `nano-9b-v2` reliable under aider's edit harness (9/10) but unreliable agentically
(3/10, why it was dropped from opencode) — full evidence in `docs/fleet-run-for-agents.md` §7.
A second, harder M-difficulty layer of that campaign (2026-07-16, same N=10/cell methodology)
found aider unreliable at M difficulty — two cells scored **0/10** on a two-file task — while
opencode and agy (Gemini) stayed reliable; prefer opencode/agy over aider for anything beyond
trivial single-file edits (§7.2/§4.1). That M layer also surfaced a `quorum fleet run`
bug (since fixed): its placeholder guard used to false-positive when the prompt itself
contained literal braces (e.g. Go code); the guard now scans only the raw argv template
before substitution, so prompt content with `{`/`}` passes through untouched (§4.1).
OpenRouter free-tier limits bound every `:free` call while those transports were live: 20 req/min
shared account-wide, 1000 req/day on this account (≥ $10 lifetime purchased credits; 50/day
otherwise), and 429s COUNT against the daily quota. Moot since 2026-09-09; the live constraint is
now a single OpenCode Go subscription shared by every routed cell, so a quota 429 there takes the
whole external fleet down at once and the correct reaction is to fall internal, not to walk the
catalog.

`opencode_go` (FLEET-038) is a distinct transport sharing the `opencode` binary, env, argv_template, input_channel, and output_format but with `quota_class: subscription` and its own vendor-branded models, never folded into the api-quota `opencode` block. Since 2026-09-09 it is the ONLY active transport and it backs all four routing levels; it declares TEN models (`deepseek-v4-flash`, `deepseek-v4-pro`, `qwen3.7-plus`, `qwen3.8-flash`, `minimax-m3`, `hy3`, `kimi-k2.7-code`, `kimi-k3`, `grok-4.6`, `gpt-5.6-luna`), of which eight are routed and two (`qwen3.7-plus`, `gpt-5.6-luna`) are catalog-only. `timeouts.default_s` is 600. Evidence: stage-1 name verification 8/8 and stage-2 pass@5 39/40, `docs/fleet-run-for-agents.md` section 7.8. (Superseded: this paragraph used to say five models, "declared as policy data only", and "stays unrouted" — all three were true on 2026-09-04 and false after 2026-09-08.)

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

Task state mutation logic lives in the `internal/core/task_manager.go` family — since the store refactor it is split across `task_manager.go` (resolution, IDs, project root), `task_store.go`/`task_transition.go`/`task_query.go` (persistence, state moves, queries), and `artifact.go` (artifact writes). `task_manager.go` is still the first place to inspect when task state changes unexpectedly. The CLI commands (`cmd/task*.go`, `cmd/init.go`) are thin shims.

Important invariants enforced there (Go identifiers, grep-able as written):

- **`SaveArtifact()` validates before writing.** Any `task artifact-save` (or skill that persists via this path) is schema-checked before the file is written. The validation engine itself lives in `internal/core/schema.go` (`ValidateArtifact`, keyed by `artifactSchemaMap`); `SaveArtifact` in `internal/core/artifact.go` (plus the `TaskStore.SaveArtifact` wrapper in `task_store.go`) only orchestrates the write. Failure raises `ArtifactValidationError` with a `field=$.path; reason=...` format (Python-compatible messages built by `pythonReason`/`jsonPointer` in `schema.go`).
- **`07-trace.json` is append-only.** `EnsureTraceAppendOnly()` (`artifact.go`) rejects any save that shortens or rewrites existing `attempts[]` or `events[]`. New attempts/events are appended by persisting the grown payload through `SaveArtifact` — there is no separate append helper. A delegated `q-implement` dispatch is recorded in `attempts[]` with `phase: "execute"` (see `docs/adr/0011-attempt-reroute-blocked-trace.md`).
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

A large feature can be split via `/q-decompose`, which writes a `decomposition: [...]` block into the parent's `00-spec.yaml`. `quorum task split <PARENT>` then materialises children with IDs `<PARENT>-a`, `<PARENT>-b`, ... (single lowercase letter — pattern enforced by `parentIDRE` and `isChildSuffixRest` in `task_manager.go`).

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

### HSME subordination in this repo (ADR 0008)

Any global prompt or client configuration describing HSME as a "primary system" is subordinate to ADR 0008's authority rule inside this repo: **HSME informs; Git, lifecycle artifacts, and curated `q-memory` decide.** Operational rules for every HSME call made from this repo (MCP tools or `hsme-cli`): always pass `project` (HSME does not isolate projects by itself); treat results as advisory suggestions requiring human confirmation before they influence any persisted artifact; apply explicit timeouts and degrade gracefully when HSME/Ollama are unavailable — no Quorum functionality may depend on HSME being up. The `quorum` binary never invokes HSME; only skills and semantic-side processes do.

## hsme-cli for agents

Use `hsme-cli` for HSME memory operations instead of querying its SQLite
database directly.

Default agent flags:
- Always pass `--json`.
- Always pass `--no-input`.
- Use `--dry-run` before `store`, `admin restore`, `import-quorum`.
- Prefer `--output <file>` for large `search-fuzzy`/`search-exact`/`explore` results.
- Do not parse human-text output when `--json` is available.

Common commands:
- `hsme-cli search-fuzzy "<query>" --project <proj> --limit 10 --json --no-input`
- `hsme-cli search-exact "<keyword>" --project <proj> --limit 10 --json --no-input`
- `hsme-cli status --json --no-input`
- `hsme-cli store --source-type note --project <proj> --json --no-input < notes.md`

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
