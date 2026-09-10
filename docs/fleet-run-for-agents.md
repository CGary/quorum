# `quorum fleet run` for agents

This document is for an AI agent that wants to invoke `quorum fleet run` from
**another project** (not the Quorum repo itself) to run a delegate LLM CLI
against arbitrary local files.

> **2026-09-09 — read this before anything below.** The fleet was reduced to a
> SINGLE live transport: `opencode_go` (OpenCode Go subscription). `agy` and
> `agy_edit` were retired with the Antigravity/Gemini subscription, and the $0
> OpenRouter transports (`opencode`, `aider`) plus `codex` were deactivated by
> human decision. All of them are `active: false` in `agents.yaml` and
> `quorum fleet run` now REFUSES an inactive transport with `INVALID_ARGUMENT`.
> Sections 1-7.2 below still describe the multi-transport world and are kept for
> their measured evidence and for the day a transport comes back; every
> *instruction* in them is superseded by this banner. The current state is
> section 7.8. Practical rule: `--agent opencode_go`, and read `--model` from
> `quorum fleet run --agent opencode_go --schema`, never from a name written
> in this file.

`quorum fleet run` is **NON-LIFECYCLE**: it creates no task, no worktree, runs
no git command, and writes no `07-trace.json`/`result.json`. It just execs a
transport in an explicit `--cwd` and returns one JSON envelope. For the
task-bound, forensic Quorum lifecycle dispatch, see `quorum fleet dispatch`
instead (not covered here).

## 1. One-time setup

### 1.1 Point at the live transport config: `QUORUM_FLEET_AGENTS`

`quorum fleet run` needs `.agents/fleet/agents.yaml` to know how to invoke
each transport (binary, argv, models). From another project, do **not** copy
that file — point at the Quorum repo's live copy instead, so you always get
the current, maintained transport definitions:

```bash
export QUORUM_FLEET_AGENTS=/path/to/quorum/.agents/fleet/agents.yaml
```

If `QUORUM_FLEET_AGENTS` is unset, `quorum fleet run` falls back to
`<git-root-of-your-current-directory>/.agents/fleet/agents.yaml` (resolved via
`git rev-parse` from wherever you run the command), which only works when your
current directory is inside the Quorum repo itself. Setting the env var is what
makes the command usable from any other project.

### 1.2 Credentials

Credentials are never stored in `agents.yaml` (constitutional rule: no
secrets in versioned artifacts). Depending on the transport you pick:

- **opencode / aider** (both backed by OpenRouter's free tier, $0 cost,
  `quota_class: api`; opencode pins five validated models plus the
  `openrouter/free` auto-router fallback, while aider pins six models — the
  same five plus `nvidia/nemotron-nano-9b-v2:free`, aider-only since the
  2026-07-15/16 pass@10 campaign (§7.2) found it unreliable agentically —
  and no auto-router (removed from aider on 2026-07-15):
  set `OPENROUTER_API_KEY` in your environment, **or** rely on
  the CLI's own stored auth (opencode: `opencode auth login` writes
  `auth.json`; aider reads its own configured provider credentials). Either
  path works — `quorum fleet run` does not perform its own preflight check
  for this, so a missing credential fails loudly at the opencode/aider CLI
  layer itself, not silently.
- **agy** (RETIRED 2026-09-09 — subscription gone, `active: false`): agy
  managed its own login/session. Kept here as a record of the contract.
- **opencode_go** (`quota_class: subscription`, backed by an OpenCode Go
  subscription, no per-call billing): reuses the `opencode` binary and
  invocation shape but requires OpenCode Go's own separate subscription
  credential (distinct from the `OPENROUTER_API_KEY` used by opencode/aider). It
  is the ONLY active transport since 2026-09-09 and backs all four routing
  levels; it is also the default for `--agent`.

### 1.3 Verify the binary is on PATH

`quorum fleet run` execs the transport's `binary` directly (never through a
shell), so it must be resolvable via your process `PATH`. For the only live
transport, `opencode_go`, that binary is `opencode`.

## 2. Discover the contract

Every transport declares a closed `--model` enum. Discover it, and the full
input/output shape, with `--schema` (no process is started):

```bash
quorum fleet run --agent opencode_go --schema
```

This prints `{command, description, input:{required, properties}, output,
errors}` — `input.properties.model.enum` is the exact list of canonical model
names valid for `--model` with that `--agent`.

## 3. Flags

| Flag | Required | Meaning |
|------|----------|---------|
| `--agent` | no (default `opencode_go`) | transport name from `agents.yaml`. Since 2026-09-09 `opencode_go` is the only `active: true` one; any other value is refused with `INVALID_ARGUMENT`. |
| `--model` | yes | canonical model name; closed enum, see `--schema` |
| `--cwd` | yes | existing directory the delegate runs in (its working directory / agentic scope) |
| `--input` | yes | prompt source: a file path, or `-` for stdin (there is no inline prompt flag) |
| `--json` | recommended | emit exactly one JSON envelope on stdout; all logs go to stderr |
| `--output <file>` | no | redirect a large result to a file; the envelope returns `data.result_file` instead of inlining `data.output` |
| `--timeout <s>` | no | seconds before the delegate's process group is hard-killed (default: the transport's own `timeouts.default_s`) |
| `--dry-run` | no | resolve/validate the argv (including the `--cwd` substitution) and print it, without starting a process — use this to sanity-check a call before spending quota |
| `--no-input` | no | never prompt interactively (agent-friendly default; always pass this) |

Always pass `--json` and `--no-input`. There is no way to pass an inline
prompt string — write it to a file (or pipe it via `--input -`).

## 4. Choosing an agent

| Agent | Backend | Cost | Use for |
|-------|---------|------|---------|
| `opencode_go` | 10 OpenCode Go subscription models (DeepSeek, Qwen, MiniMax, Hunyuan, Moonshot, xAI, GPT-5.6) | included in subscription | **the only live transport**; agentic `--dir`-scoped edits; backs all four routing levels (§7.8) |
| `opencode` | 5 pinned OpenRouter free models + `openrouter/free` auto-router fallback (section 7) | — | RETIRED 2026-09-09 (`active: false`) |
| `aider` | 6 pinned OpenRouter free models (section 7) | — | RETIRED 2026-09-09 (`active: false`) |
| `agy` / `agy_edit` | Gemini (Antigravity subscription) | — | RETIRED 2026-09-09, subscription gone (`active: false`) |
| `codex` | ChatGPT free-tier credits | — | RETIRED 2026-07-27 (kill-switch) + `active: false` 2026-09-09 |

There is no transport choice to make any more: `opencode_go` or nothing. The
choice that remains is which CELL — pick it by capability class and measured
evidence from `~/.claude/skills/think-cheap/scripts/rung0-cells.py`, and
validate the name against `--agent opencode_go --schema`.

**Task-difficulty evidence, retained (§7.2):** the 2026-07-16 M-difficulty
layer measured two `aider` cells at **0/10** on a two-file task, because its
single-shot whole-file edit format has no compiler feedback loop inside Quorum.
That is why aider was never trusted beyond trivial single-file edits — a
conclusion worth keeping if a `-free` tier is ever reintroduced. The 2026-09-09
campaign re-ran the same two-file task against all eight live `opencode_go`
cells: **39/40** (§7.8).

### 4.1 Measured agy reliability (pass@10, 2026-07-15/16 campaign)

Measured through `quorum fleet run` (agy, subscription quota) — trivial
layer via the CLI itself; see the M-layer note below for why the M cells
were measured differently — with the same N=10-trial methodology as the
OpenRouter cells in §7.2: trivial layer graded by a hidden 9-case test
(2026-07-15), M layer graded by a hidden, independent 14-subtest suite on a
harder two-file task (2026-07-16, stage 6 of §7.1). Both are an upper bound
on harness reliability, not real-feature capability (§7.1).

| Model | Trivial pass@10 | M pass@10 | Notes |
|---|---|---|---|
| Gemini 3.5 Flash (low / medium / high) | 10/10 each | 10/10 each | retired by the provider 2026-09-03; row kept as historical evidence only |
| Gemini 3.1 Pro (low / high) | 10/10 each | 10/10 each | |
| Claude Sonnet 4.6 | 10/10 (14–18s) | 9/10 | M: all failures were "Individual quota reached" errors from Antigravity — availability, not capability; 9/9 when it actually ran (see the quota-bucket finding below) |
| Claude Opus 4.6 | 10/10 (16–19s) | 8/10 | M: same quota-exhaustion cause as Sonnet; 8/8 when it actually ran |
| GPT-OSS 120B | 5/10 | 3/10 | Trivial: all 5 failures were the model refusing to write outside agy's artifact directory citing sandbox policy — a harness-policy failure, not a codegen failure. M: same systematic sandbox-policy-refusal pattern (5 of the 7 failures) plus 2 more quota-exhaustion failures |

**agy absolute-path trap**: with a small, fresh `--cwd` repo, agy may ignore
the process cwd, resolve its own workspace, and write to
`~/.gemini/antigravity-cli/scratch/` instead — a stale file there can make it
claim a file is "already created" without writing anything. Always name the
absolute destination path for any file the delegate must create; real
task-bound `quorum fleet dispatch` runs into git worktrees are unaffected.
(Methodology note: the agy cells above were probed with absolute-path
prompts; the opencode/aider cells in §7.2 used a plain "current directory"
prompt instead — keep that asymmetry in mind when comparing numbers across
transports.)

**Antigravity quota is bucketed per model family**: during the M-layer
campaign, roughly 150 agy calls in one day exhausted the PREMIUM (Claude/GPT)
individual quota ("Individual quota reached... Resets in ~2h") while the
Gemini cells kept running unaffected — Gemini and Claude/GPT draw from
separate quota pools. Plan measurement or reroute budgets accordingly:
exhausting one family's quota says nothing about the other's remaining
budget.

**Fixed bug — `quorum fleet run`'s residual-placeholder guard false-positived
on brace-containing prompts**: on `prompt_arg` transports (agy), the guard
meant to reject un-substituted dispatch-only placeholders used to also match
literal braces that appeared inside the prompt text itself (e.g. a Go code
block), failing instantly with `INVALID_ARGUMENT` ("argv references
dispatch-only placeholder", quoting the entire prompt back in the error)
before any process started. This is why the M-layer agy cells in the table
above were measured by **invoking the transport binary directly** with the
same argv `quorum fleet run` would have used, rather than through
`quorum fleet run` itself — that campaign predates the fix. The guard now
scans only the raw argv template tokens read from `agents.yaml` **before**
variable substitution, so it only ever rejects a genuine unresolved
`{name}` placeholder in the template; prompt/`--input` content containing
literal `{`/`}` (Go code, JSON, etc.) passes through untouched. No
workaround is needed on current builds.

### 4.2 Live catalog check (`quorum fleet catalog`, 2026-09-03)

Model rows in this document decay (§7.7): providers retire models without notice, and a
dispatch to a retired model fails in seconds with the transport's "invalid model selection"
message, not a reasoning error. Before trusting any model name from these tables run:

```bash
quorum fleet catalog agy --json        # or agy_edit / opencode / aider
```

It probes the transport with a sentinel model, parses the live "available models" list the
transport prints on rejection, and reports `declared_dead` (in `agents.yaml` but gone) and
`live_undeclared` (live but not yet in `agents.yaml`; needs a smoke campaign before routing).
Read-only, manual-only, costs one rejected call. Since FLEET-036 both agy transports also declare
`wrapper_signatures`, so a retired-model rejection during `quorum fleet dispatch` is classified
as `reroute`/`wrapper_broken` (ADR 0011) instead of burning a contract attempt.

## 5. End-to-end example (opencode, dry-run then real)

```bash
export QUORUM_FLEET_AGENTS=/path/to/quorum/.agents/fleet/agents.yaml

echo "Add a doc comment to the exported Foo function in bar.go" > /tmp/prompt.txt

# 1. Sanity-check the resolved argv without spending quota:
# --model is a closed enum; NEVER copy a model name from this doc, read it from
# `quorum fleet run --agent opencode_go --schema`.
quorum fleet run \
  --agent opencode_go --model <key from --schema> \
  --cwd /path/to/my-project \
  --input /tmp/prompt.txt \
  --dry-run --no-input --json

# 2. Real run:
quorum fleet run \
  --agent opencode_go --model <key from --schema> \
  --cwd /path/to/my-project \
  --input /tmp/prompt.txt \
  --no-input --json
```

## 6. Output envelope

Every invocation prints exactly one JSON object on stdout (all logs go to
stderr):

**Success:**

```json
{
  "ok": true,
  "command": "fleet.run",
  "summary": "delegate opencode exited with code 0",
  "data": {
    "agent": "opencode",
    "model": "openrouter/free",
    "cwd": "/path/to/my-project",
    "exit_code": 0,
    "killed": false,
    "quota_matched": false,
    "output_parse_ok": true,
    "output": "..."
  },
  "next_actions": []
}
```

**Failure** (stable error codes: `MISSING_REQUIRED_FLAG`, `INVALID_ENUM`,
`FILE_NOT_FOUND`, `TIMEOUT`, `INVALID_ARGUMENT`, `INTERNAL_ERROR`):

```json
{
  "ok": false,
  "command": "fleet.run",
  "error": {
    "code": "INVALID_ENUM",
    "message": "--model must be one of: openrouter/free",
    "field": "model",
    "received": "gpt-4"
  },
  "retryable": false,
  "suggested_fix": "quorum fleet run --agent opencode_go --model <key from --schema> --cwd <dir> --input <file> --json"
}
```

A `TIMEOUT` result means the delegate exceeded `--timeout` (or the
transport's `timeouts.default_s`) and its whole process group was killed —
retry with a larger `--timeout` or a smaller prompt/scope.

## 7. OpenRouter free models: validated list (2026-07)

This section is a curated result of an empirical evaluation run against
OpenRouter's `:free` model catalog on 2026-07-15. It exists so you don't have
to pay the same probing cost twice: `openrouter/free` (the auto-router
already wired above, section 4) picks **randomly** among capability-matching
free models with no uptime/latency filtering, so its pass rate is
inconsistent call to call. The models below are pinned to specific IDs with
actual test evidence behind them.

### 7.1 Methodology

Five narrowing stages, all measured live — no number below is estimated:

1. **Expiration filter** — `GET https://openrouter.ai/api/v1/models`, keep
   only entries where `expiration_date == null`. 20 candidate `:free` models
   → 13 survive.
2. **Latency probe** — one real request, trivial prompt, 2 attempts. 13 → 12
   survive.
3. **Single-shot codegen probe** — one prompt, graded by a hidden,
   independent Go test suite of 9 cases the model never saw. 12 → 8 pass 9/9.
4. **Agentic probe** — the same hidden grading, but driven through
   `opencode`'s own tool-use loop (file read/write via the CLI's tools, not a
   single completion). All 8 codegen survivors probed: 6 pass 9/9, 2 fail.
5. **Per-cell pass@10 campaign** (2026-07-15/16) — a "cell" is a
   (transport, model) pair. Every cell wired into `agents.yaml`, plus every
   `agy` model/effort combination, was run **N=10** trials against a trivial
   synthetic Go task (implement `ParsePairs`), graded by the same kind of
   hidden 9-case test the model never sees. All 21 active fleet cells were
   measured this way (7 opencode + 6 aider + 8 agy, ~220 runs total). This is
   an **upper bound** on reliability — it measures harness/tool-use
   consistency on a trivial task, not real-feature capability (the F5
   experiment measures that separately). OpenRouter probes ran serially, 8s
   apart, to respect the shared 20 req/min cap (§7.7); aider cells ran
   through the real `quorum fleet dispatch` forensic pipeline (live task +
   contract, worktree reset between trials), not a bare CLI call. Methodology
   note: `agy` cells (§4.1) were prompted with the absolute destination path
   for the file to create; the opencode/aider cells below used a plain
   "current directory" prompt instead — see the agy absolute-path trap in
   §4.1.
6. **M-difficulty layer** (2026-07-16) — a second, harder difficulty tier,
   same **N=10**-per-cell methodology as stage 5, graded by a hidden,
   independent Go test suite of **14 subtests** the model never saw. Task:
   implement a small in-memory inventory store split across **two files**
   (`store.go` + `report.go`, package `store`) — `Add`/`Remove`/`Get`/`Len`
   with trim/error semantics, delete-on-zero-quantity, and a sorted
   `name=qty` `Report()`. The two-file structure plus the int→string
   formatting it requires is deliberately harder than the trivial
   single-function `ParsePairs` task in stage 5 — it is designed to separate
   "can follow one clean edit" from "can reason about a small multi-file API
   surface." OpenRouter cells (opencode/aider) again ran serially, 8s apart
   (§7.7), and aider cells again ran through the real `quorum fleet dispatch`
   forensic pipeline against a live task + contract (worktree reset between
   trials) — same protocol as stage 5, just a harder task. `agy` cells were
   measured via **direct invocation of the transport binary**, bypassing
   `quorum fleet run`, because this campaign surfaced an open
   `quorum fleet run` bug: its residual-placeholder guard false-positives
   when the prompt itself contains literal braces (this M task's prompt
   necessarily includes Go code with `{`/`}`), rejecting the call instantly
   with `INVALID_ARGUMENT` before a process is even started — see the
   workaround in §4.1. Results: §7.2 (opencode/aider) and §4.1 (agy).

### 7.2 Tier A — validated pinned models (measured pass@10 per cell)

The single-shot/agentic probes in stages 3–4 above picked the original six
Tier A candidates; the stage-5 campaign (§7.1) replaced that one-shot signal
with **N=10 measured pass@10 per cell**, split by transport. Read every
number below as an upper bound on reliability, not a real-feature capability
score — see §7.1. Ordered by measured reliability (the two leaders pass
10/10 on both transports):

**Trivial layer (`ParsePairs`, 2026-07-15):**

| Model ID | opencode pass@10 | aider pass@10 | Failure causes |
|---|---|---|---|
| `nvidia/nemotron-3-ultra-550b-a55b:free` | 10/10 | 10/10 | — |
| `poolside/laguna-m.1:free` | 10/10 | 10/10 | — |
| `poolside/laguna-xs-2.1:free` | 9/10 | 10/10 | opencode: 1 malformed tool_use call |
| `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free` | 7/10 | 10/10 | opencode: all 3 failures were availability — consecutive Nvidia 502 "ResourceExhausted: Worker local total request limit reached (16/16)"; capability was 7/7 when the provider actually responded |
| `cohere/north-mini-code:free` | 9/10 | 7/10 | opencode: 1 malformed tool_use call. aider: 3 timeouts at the 240s cap |
| `openrouter/free` (auto-router) | 9/10 | not wired to aider | opencode only; 1 TEST_FAIL; per-call random routing means this varies with the day's pool health |
| `nvidia/nemotron-nano-9b-v2:free` | 3/10 — **removed from opencode** | 9/10 (32–154s/call) — **aider-only** | opencode: wrong code (3 TEST_FAIL), reasons-without-acting, 66–200s runs. aider: 1 TEST_FAIL |

Reliability is a property of the **cell** (transport, model), not the model
alone — see the divergence in the last three rows, and §7.3.

**M layer (two-file inventory store, 2026-07-16; see stage 6, §7.1):**

> **The trivial→M ordering is NOT preserved.** Two aider cells that were
> reliable on the trivial task collapsed to **0/10** on the M task
> (`nemotron-3-ultra-550b-a55b` 10/10 → 0/10, `nemotron-nano-9b-v2` 9/10 →
> 0/10). A pass@10 result on the trivial layer alone is **insufficient
> evidence for a routing decision** — the M layer is where a gate like G1
> should actually bind, not the trivial layer.

| Model ID | opencode M pass@10 | aider M pass@10 | M failure causes |
|---|---|---|---|
| `nvidia/nemotron-3-ultra-550b-a55b:free` | 10/10 | **0/10** | aider: the model narrates its own edit ("Let me correct report.go") and aider's whole-edit-format parser treats that narration text as a **filename**, creating a junk file `Let me correct report.go.report.go` with duplicate method declarations → build fails, on all 10 runs |
| `poolside/laguna-m.1:free` | 10/10 | 9/10 | aider: 1 failure (cause not itemized in this pass) |
| `poolside/laguna-xs-2.1:free` | 10/10 | 4/10 | aider: classic Go `string(rune(n))` conversion bug — `Report()` emitted `"apple=\x02"` instead of `"apple=2"` |
| `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free` | 4/10 | 6/10 | opencode: failure **cause flipped** vs. the trivial layer — there, all 3 failures were provider 502s (availability); here, failures are capability (`TEST_FAIL`s and runs that never created the files). aider: 4 failures not itemized |
| `cohere/north-mini-code:free` | 6/10 | 5/10 | opencode: failures not itemized. aider: 3 of the 5 failures timed out at the 300s cap; the remaining 2 not itemized |
| `openrouter/free` (auto-router) | 8/10 | not wired to aider | opencode only; failures not itemized — per-call random routing means this varies with the day's pool health |
| `nvidia/nemotron-nano-9b-v2:free` | not wired to opencode | **0/10** | aider: hallucinated API `strings.Itoa` (the real API is `strconv.Itoa`) plus unused imports → build fails, on all 10 runs |

Two findings the M layer adds on top of the reliability point above:

- **The agentic loop earns its value at M difficulty.** For the same
  models, opencode cells (agentic — can inspect/build/self-correct via
  tools) beat aider cells (single-shot whole-file edit, no compiler
  feedback) at M: `nemotron-3-ultra-550b-a55b` is 10/10 opencode-M vs. 0/10
  aider-M. This is a direct, now-quantified consequence of a Quorum design
  choice: the aider transport deliberately disables aider's own
  auto-lint/auto-test loop for forensic purity (`q-verify` is the only
  validation truth in the SDC lifecycle), which also trades away aider's
  built-in self-correction.
- **Practical guidance (HISTORICAL — every transport named here is retired,
  see the 2026-09-09 banner at the top)**: aider was trusted only for trivial,
  mechanical, single-file edits; `opencode` or `agy` for M difficulty or harder.
  Across both layers `agy` Gemini was the only family that scored 10/10 on
  both. The lasting lesson is about EDIT HARNESS, not vendor: a single-shot
  whole-file format without a feedback loop collapses at M difficulty, while an
  agentic tool-loop does not. `opencode_go` is agentic and scored 39/40 on the
  same M task (§7.8).
- **Composing a pass@≤1-reroute policy**: from the measured marginals above,
  any cell backed by a single reroute to a 10/10 target (an `agy` Gemini
  tier, or opencode `ultra-550b`/`laguna-m.1`) clears a 70% reliability
  threshold trivially — the real design constraints are reroute cost and
  provider correlation, not raw pass rate. A reroute should cross providers
  (e.g. a saturated OpenRouter free-tier cell rerouting to `agy`, not to
  another OpenRouter cell) so that an OpenRouter-wide saturation event
  doesn't take out both the primary and the reroute at once. This assumes
  the two attempts' failures are independent, which only holds when they
  don't share a provider/quota pool.

### 7.3 Tier B — single-shot codegen only (agentic FAIL)

These pass the hidden 9-case suite single-shot (direct completion) but are
**confirmed to fail** the agentic tool-use probe. Do not point
`opencode`/`aider` at these; use them only via direct, non-agentic API calls
(section 7.6). Neither model was part of the stage-5 campaign (§7.1) — both
were discarded at stage 4, before `agents.yaml` ever pinned them, so there is
no pass@10 number to report for them.

They remain the clearest binary examples of "good generator, unreliable
agent." The campaign has since produced a third, **quantified** case of the
same gap: `nvidia/nemotron-nano-9b-v2:free` (§7.2) is not a hard pass/fail
like these two — 9/10 under aider's edit harness vs. 3/10 under opencode's
agentic loop — but the underlying failure mode is the same (tool-use/agentic
unreliability, not codegen capability), which is why it was demoted to
aider-only instead of fully discarded like the two below.

| Model ID | Single-shot codegen | Agentic evidence |
|---|---|---|
| `nvidia/nemotron-3-nano-30b-a3b:free` | PASS 9/9 in 4.8s | **FAIL** — truncated file write via tool at line 26 |
| `openai/gpt-oss-20b:free` | PASS 9/9 in 38.7s | **FAIL** — ran 26s, exit 0, but never created the file (no tool call issued; reasons-without-acting pattern) |

### 7.4 Discarded (with evidence)

| Model ID | Reason |
|---|---|
| `qwen/qwen3-coder:free` | expires 2026-07-19; 429 on 2/2 attempts |
| `qwen/qwen3-next-80b-a3b-instruct:free` | expires 2026-07-19; 429 on 2/2 attempts |
| `meta-llama/llama-3.3-70b-instruct:free` | expires 2026-07-19; 429 on 2/2 attempts |
| `meta-llama/llama-3.2-3b-instruct:free` | expires 2026-07-19; 429 on 2/2 attempts |
| `nousresearch/hermes-3-llama-3.1-405b:free` | expires 2026-07-19; 429 on 2/2 attempts |
| `cognitivecomputations/dolphin-mistral-24b-venice-edition:free` | expires 2026-07-19; 429 on 2/2 attempts |
| `tencent/hy3:free` | expires 2026-07-21 (launch promo, 9 days old at eval time) |
| `google/gemma-4-31b-it:free` | no `expiration_date`, but 429 on 2/2 attempts |
| `nvidia/nemotron-3-super-120b-a12b:free` | no `expiration_date`; 200 OK on the trivial probe (1.0s) but 429 on 2/3 probes; re-probed 2026-07-15 (2 attempts, 60s apart): 429 on both — persistent saturation confirmed, stays discarded |
| `google/gemma-4-26b-a4b-it:free` | emitted invalid Go (stray `?` character, syntax garbage) |
| `nvidia/nemotron-nano-12b-v2-vl:free` | hallucinated `strings.Split` with 3 arguments |
| `nvidia/nemotron-3.5-content-safety:free` | safety classifier, not a general-purpose model |

Observed pattern: popular veteran free models saturate (429) as they
approach their `expiration_date`; nvidia-hosted free models are currently
the most consistently available tier.

### 7.5 How to use Tier A from Quorum fleet (internal and external projects)

`.agents/fleet/agents.yaml`'s `opencode` transport carries a pinned entry for
each of the **five** opencode-side Tier A models (`nvidia/nemotron-nano-9b-v2`
was dropped after §7.2's campaign — 3/10 agentically), alongside the existing
`openrouter/free` auto-router fallback. The canonical map keys cannot reuse
the raw OpenRouter model IDs: `agents.schema.json`'s `models` `propertyNames`
pattern (`^[a-z0-9_.-]+/[a-z0-9_.-]+$`) allows exactly one `/` but excludes
`:` from its character class entirely, so an ID like
`nvidia/nemotron-3-ultra-550b-a55b:free` cannot be a map key as-is. Every
checked-in key substitutes `-free` for `:free`; `model_arg` carries the
exact, colon-preserving string opencode's `-m` flag actually needs:

```bash
# HISTORICAL example — `opencode` is active:false since 2026-09-09 and this call
# now returns INVALID_ARGUMENT. Shown for the KEY TRANSFORM, which still applies
# to any future `-free` cell.
quorum fleet run \
  --agent opencode --model "nvidia/nemotron-3-ultra-550b-a55b-free" \
  --cwd . --input - --no-input --json
```

This resolves internally to opencode arg
`-m openrouter/nvidia/nemotron-3-ultra-550b-a55b:free`. The other four keys
follow the same transform (e.g. `poolside/laguna-xs-2.1-free`,
`cohere/north-mini-code-free`). The authoritative enum for any transport is
`quorum fleet run --agent <transport> --schema`; the only one that answers with
live cells today is `opencode_go`.
`openrouter/free` (the auto-router) remains available as the
availability-resilience fallback when a pinned model is saturated or its
`expiration_date` changes (section 7.7) — it is opencode-only, not wired to
aider.

External projects: point `QUORUM_FLEET_AGENTS` at the Quorum repo's
`.agents/fleet/agents.yaml`, exactly as described in section 1.1 — no extra
setup is needed to reach these models.

`--agent aider` carries **six** pinned keys: the same five opencode keys,
plus `nvidia/nemotron-nano-9b-v2-free` (aider-only; see
`quorum fleet run --agent aider --schema` for its own authoritative enum).
aider is a different harness, though: whole-file edit format, no agentic
tool-calls, and `quorum fleet run` rejects it task-less because its argv
needs `{files}` from a contract — it runs via `quorum fleet dispatch`
instead. Measured pass@10 (§7.2): all six pass at least 7/10, led by
`nvidia/nemotron-3-ultra-550b-a55b:free`,
`nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free`,
`poolside/laguna-xs-2.1:free`, and `poolside/laguna-m.1:free` at 10/10; the
edit harness is specifically why `nano-9b-v2` (9/10) stays pinned here
despite its 3/10 opencode agentic result.

### 7.6 Using Tier B models outside Quorum (direct API, non-agentic only)

Tier B models are **not** wired into `agents.yaml` — `opencode` is agentic,
and both Tier B models are confirmed to fail there. Their single-shot
capability is still real; to use one, call the OpenRouter chat-completions
API directly with a single-shot prompt:

```bash
curl -s https://openrouter.ai/api/v1/chat/completions \
  -H "Authorization: Bearer $OPENROUTER_API_KEY" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "nvidia/nemotron-3-nano-30b-a3b:free",
    "messages": [{"role": "user", "content": "Write a Go function that reverses a string."}]
  }'
```

Do not drive these through `opencode`/`aider`'s agentic tool-use loop — both
fail there despite passing the same hidden suite single-shot:
`nvidia/nemotron-3-nano-30b-a3b:free` truncates tool-driven file writes
(observed at line 26), and `openai/gpt-oss-20b:free` exits 0 without ever
issuing a tool call (reasons-without-acting).

### 7.7 Re-validation: this list decays

Free-tier availability shifts fast — do not treat this section as permanent:

- **Re-check `expiration_date` before relying on any pinned model:**
  ```bash
  curl -s https://openrouter.ai/api/v1/models | jq -r '.data[] | select(.id=="<MODEL_ID>") | .expiration_date'
  ```
  A non-null result means the model is scheduled for removal — treat it like
  the discarded expiring models in section 7.4.
- **Re-probe latency** with a single trivial-prompt request before a real
  dispatch, especially for Tier B or previously saturated models (e.g.
  `nvidia/nemotron-3-super-120b-a12b:free`, section 7.4) — 429s become
  common as veteran free models approach expiration.

Operational precautions for the free tier (policy source:
https://openrouter.ai/docs/api-reference/limits):

- **Know your daily tier**: 50 req/day if your lifetime purchased credits
  are under $10; 1000 req/day once $10 total has been purchased (a
  permanent, one-time unlock). Check which tier your account is on:
  ```bash
  curl -s https://openrouter.ai/api/v1/credits \
    -H "Authorization: Bearer $OPENROUTER_API_KEY"
  ```
  `total_credits >= 10` (lifetime purchases) means the 1000/day tier. Do
  NOT use `/api/v1/auth/key`'s `limit` field for this — that is a per-key
  spending cap, not your lifetime purchase total, and the two are easy to
  confuse.
- **The 20 req/min cap is shared across ALL `:free` calls from the
  account.** One agentic opencode run consumes several requests (each
  tool-loop step is one request), so 2–3 concurrent agentic runs can
  saturate the cap by themselves. Space bulk probes ~3s apart and avoid
  concurrent agentic runs on free models.
- **Never retry-loop a 429**: failed/429 requests still count against the
  daily quota, so a retry loop against a saturated model burns the day's
  budget while returning nothing.

### 7.8 OpenCode Go: smoke campaign (FLEET-038)

`opencode_go` (FLEET-038) was declared as policy data on 2026-09-04 with no empirical evidence. On 2026-09-07 `opencode-go/deepseek-v4-pro` was smoked with the section 7.1 M-layer methodology (`quorum fleet run --agent opencode_go`, agentic, isolated scratch git repo reset between trials, two-file task `store.go` + `report.go` graded by a hidden 15-subtest Go test the model never saw). Raw envelopes, prompt, hidden test, runner and generated code: `~/.claude/skills/fleet-delegate/raw-runs/2026-09-07-opencode_go-deepseek-v4-pro-smoke/`; ledger lines in `~/.claude/skills/fleet-delegate/ledger.jsonl`.

Routing consequence (2026-09-08, human decision): `deepseek-v4-pro` is the **level-3 fallback** in `config.yaml`, replacing `google/gemini-3.1-pro-high` (1/4 dispatch success), and the four `gemini-3.1-pro-*` catalog entries were retired from both agy transports.

The other four cells were routed the same day **by human decision without smoke evidence** ("option 2"), following the placement ratified on 2026-09-06 during the FLEET-038 plan, and then — a second human decision the same day — **promoted to `primary` of every level**. That ladder was superseded 24 hours later; see below.

#### 2026-09-09 — full ladder rebuild, and a two-stage smoke

Human decision: the Antigravity/Gemini subscription is retired (like codex on 2026-07-27) and the $0 OpenRouter cells are dropped, leaving `opencode_go` as the only active transport. The ladder was rebuilt on eight OpenCode Go cells named by the human, two per level:

| Level | Primary | Fallback |
|-------|---------|----------|
| 0 | `opencode-go/deepseek-v4-flash` | `opencode-go/qwen3.8-flash` |
| 1 | `opencode-go/minimax-m3` | `opencode-go/hy3` |
| 2 | `opencode-go/deepseek-v4-pro` | `opencode-go/kimi-k2.7-code` |
| 3 | `opencode-go/kimi-k3` | `opencode-go/grok-4.6` |

Five of those cells (`qwen3.8-flash`, `hy3`, `kimi-k2.7-code`, `kimi-k3`, `grok-4.6`) were new to `agents.yaml` and needed three new `provider` values in the closed enum: `opencode-go-tencent`, `opencode-go-moonshot`, `opencode-go-xai`.

The smoke was run in **two stages**, because they answer different questions and fail at different costs:

**Stage 1 — is the model NAME right?** `quorum fleet catalog opencode_go` cannot answer this: it returns `status: "unknown"` because opencode prints no parseable "Available models:" block on rejection. So each cell got one trivial probe (`Reply with exactly the word OK`, `--timeout 120`, scratch repo, prompt forbidding writes) and was checked for `ok:true`, `exit_code 0`, no rejection signature, and a clean `git status`. A wrong vendor-side `model_arg` fails here in seconds without polluting the capability ledger.

Result 2026-09-09: **8/8**. Every cell answered exactly `OK` in 4-7 s with no rejection signature and wrote nothing. All eight `model_arg` values are confirmed against the live provider.

| Cell | Stage 1 | Latency |
|------|---------|---------|
| `deepseek-v4-flash` | OK | 4 s |
| `qwen3.8-flash` | OK | 7 s |
| `minimax-m3` | OK | 4 s |
| `hy3` | OK | 6 s |
| `deepseek-v4-pro` | OK | 5 s |
| `kimi-k2.7-code` | OK | 7 s |
| `kimi-k3` | OK | 5 s |
| `grok-4.6` | OK | 5 s |

**Stage 2 — does it actually DO the work?** The section 7.1 M-layer methodology at pass@5 per cell (same two-file task, same hidden 15-subtest grader, git reset between trials). Runner: `$SCRATCH/smoke/stage2.sh`, a parameterised copy of the 2026-09-07 `run.sh`.

Result 2026-09-09: **39/40 trials**. Every passing trial scored 15/15 hidden subtests and wrote exactly the two requested files — no partial credit anywhere, no extra files, no cell that "almost" worked.

| Cell | Level / slot | pass@5 | Latency min / median / max | Notes |
|------|--------------|--------|----------------------------|-------|
| `opencode-go/deepseek-v4-flash` | L0 primary | **4/5** | 15 / 18 / 300 s | The one failure in the whole campaign: trial 1 returned `TIMEOUT` at exactly 300 s having written **zero bytes**. Trials 2-5 solved the task in 15-25 s. A transient provider hang, not a capability limit — but it is the level-0 primary, so watch `quorum fleet stats`. |
| `opencode-go/qwen3.8-flash` | L0 fallback | **5/5** | 34 / 39 / 76 s | Reliable but the slowest "flash" cell — ~2x deepseek-v4-flash on the same task. |
| `opencode-go/minimax-m3` | L1 primary | **5/5** | 8 / 12 / 17 s | Fastest cell measured, on any campaign. |
| `opencode-go/hy3` | L1 fallback | **5/5** | 17 / 20 / 21 s | Tightest variance of the eight. |
| `opencode-go/deepseek-v4-pro` | L2 primary | **5/5** | 21 / 23 / 28 s | Consistent with its 2026-09-07 pass@10 = 10/10 (15-27 s). Two independent campaigns agree. |
| `opencode-go/kimi-k2.7-code` | L2 fallback | **5/5** | 26 / 42 / 59 s | |
| `opencode-go/kimi-k3` | L3 primary | **5/5** | 45 / 99 / 266 s | **Slowest and most variable cell by far.** A 266 s trial would have been killed under the old 300 s transport timeout with almost no margin; it passes only because `timeouts.default_s` was raised to 600 s in the same change. Do not lower that timeout while k3 leads level 3. |
| `opencode-go/grok-4.6` | L3 fallback | **5/5** | 19 / 24 / 29 s | |

Two things this campaign is evidence FOR, and one it is not. It IS evidence that all eight `model_arg` values are correct (stage 1) and that all eight cells can carry a two-file M task agentically and reliably. It is NOT evidence about capability on a real `{high, L}` feature — the same limit as every earlier campaign — nor about behaviour under a saturated subscription, which no synthetic run can exercise.

Evidence policy for this rebuild (human decision, "rutear las 8 igual, smoke solo informativo"): the ladder was routed **before** stage 2 finished, so stage 2 DOCUMENTS rather than gates. This is a deliberate, recorded exception to the proven-before-new rule — the second one in two days.

#### Annex — the superseded 2026-09-08 ladder (historical)

Kept as the causal record of the 24-hour-old ladder the rebuild above replaced. Nothing here is current: `qwen3.7-plus` and `gpt-5.6-luna` are in the catalog but UNROUTED, and Antigravity/OpenRouter are no longer reroute targets because those transports are `active: false`.

That day's decision promoted the Go cells to `primary` of every level — `deepseek-v4-flash` L0, `qwen3.7-plus` L1, `minimax-m3` L2, `deepseek-v4-pro` L3 (fallback `gpt-5.6-luna`) — shifting the former primary/fallback down one slot each (L0 nemotron-super → north-mini; L1 sonnet → opus → nemotron-super; L2 3.6-flash-high → 3.7-flash-medium; L3 3.7-flash-high), on the rationale that the Antigravity subscription was exhausted while OpenCode Go was live. Only one row of its evidence table survives as current data:

| Model ID | model_arg | pass@10 | Notes |
|----------|-----------|---------|-------|
| `opencode-go/deepseek-v4-pro` | `opencode-go/deepseek-v4-pro` | **10/10** | 2026-09-07 campaign: 15/15 hidden subtests every trial; 15-27 s/trial; 0 extra files; 0 timeouts. Independently re-confirmed at 5/5 on 2026-09-09. |
| `opencode-go/qwen3.7-plus` | `opencode-go/qwen3.7-plus` | never smoked | Was L1 primary 2026-09-08 → 2026-09-09. Catalog only, UNROUTED. |
| `opencode-go/gpt-5.6-luna` | `opencode-go/gpt-5.6-luna` | never smoked | Was L3 fallback 2026-09-08 → 2026-09-09. Catalog only, UNROUTED. |

Per the proven-before-new rule (AGENTS.md, 2026-08-26 ladder rebalance), no cell should be routed before its row is filled in with empirical smoke evidence. That rule was consciously suspended on 2026-09-08 and again on 2026-09-09 (see above); both suspensions are human decisions on record, not drift.
