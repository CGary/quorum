# `quorum fleet run` for agents

This document is for an AI agent that wants to invoke `quorum fleet run` from
**another project** (not the Quorum repo itself) to run a delegate LLM CLI
(opencode, aider, agy, ...) against arbitrary local files.

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
- **agy** (`quota_class: subscription`, backed by a Gemini subscription, no
  per-call billing): agy manages its own login/session; there is nothing to
  export here.

### 1.3 Verify the binary is on PATH

`quorum fleet run` execs the transport's `binary` directly (never through a
shell), so it must be resolvable via your process `PATH` (`opencode`,
`aider`, or `agy`).

## 2. Discover the contract

Every transport declares a closed `--model` enum. Discover it, and the full
input/output shape, with `--schema` (no process is started):

```bash
quorum fleet run --agent opencode --schema
```

This prints `{command, description, input:{required, properties}, output,
errors}` — `input.properties.model.enum` is the exact list of canonical model
names valid for `--model` with that `--agent`.

## 3. Flags

| Flag | Required | Meaning |
|------|----------|---------|
| `--agent` | no (default `agy`) | transport name from `agents.yaml` (`opencode`, `aider`, `agy`) |
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
| `opencode` | 5 pinned OpenRouter free models + `openrouter/free` auto-router fallback (section 7) | $0 (free tier) | implement-only; agentic `--dir`-scoped edits |
| `aider` | 6 pinned OpenRouter free models — the same 5 as opencode, plus aider-only `nemotron-nano-9b-v2` (section 7) | $0 (free tier) | implement-only; message-file + explicit file list |
| `agy` | Gemini (your subscription) | included in your subscription, no per-call billing | broader use; higher-effort models available (Gemini 3.5 Flash, Gemini 3.1 Pro, Claude Sonnet/Opus, GPT-OSS 120B — measured pass@10 in §4.1) |

Both `opencode` and `aider` are **implement-only** transports (never used for
review/diagnostic phases in Quorum's own lifecycle) and cost nothing per call
on the OpenRouter free tier — prefer them for routine edits. Use `agy` when
you need a specific higher-capability model and already have quota on your
Gemini subscription.

### 4.1 Measured agy reliability (pass@10, 2026-07-15/16 campaign)

Measured through `quorum fleet run` (agy, subscription quota) with the same
N=10-trial, hidden-9-case-test methodology as the OpenRouter cells in §7.2 —
an upper bound on harness reliability, not real-feature capability (§7.1).

| Model | pass@10 | Notes |
|---|---|---|
| Gemini 3.5 Flash (low / medium / high) | 10/10 each | |
| Gemini 3.1 Pro (low / high) | 10/10 each | |
| Claude Sonnet 4.6 | 10/10 (14–18s) | |
| Claude Opus 4.6 | 10/10 (16–19s) | |
| GPT-OSS 120B | 5/10 | all 5 failures: the model refuses to write outside agy's artifact directory citing sandbox policy, inconsistently run to run — a harness-policy failure, not a codegen failure |

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

## 5. End-to-end example (opencode, dry-run then real)

```bash
export QUORUM_FLEET_AGENTS=/path/to/quorum/.agents/fleet/agents.yaml

echo "Add a doc comment to the exported Foo function in bar.go" > /tmp/prompt.txt

# 1. Sanity-check the resolved argv without spending quota:
quorum fleet run \
  --agent opencode --model openrouter/free \
  --cwd /path/to/my-project \
  --input /tmp/prompt.txt \
  --dry-run --no-input --json

# 2. Real run:
quorum fleet run \
  --agent opencode --model openrouter/free \
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
  "suggested_fix": "quorum fleet run --agent opencode --model openrouter/free --cwd <dir> --input <file> --json"
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

### 7.2 Tier A — validated pinned models (measured pass@10 per cell)

The single-shot/agentic probes in stages 3–4 above picked the original six
Tier A candidates; the stage-5 campaign (§7.1) replaced that one-shot signal
with **N=10 measured pass@10 per cell**, split by transport. Read every
number below as an upper bound on reliability, not a real-feature capability
score — see §7.1. Ordered by measured reliability (the two leaders pass
10/10 on both transports):

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
quorum fleet run \
  --agent opencode --model "nvidia/nemotron-3-ultra-550b-a55b-free" \
  --cwd . --input - --no-input --json
```

This resolves internally to opencode arg
`-m openrouter/nvidia/nemotron-3-ultra-550b-a55b:free`. The other four keys
follow the same transform (e.g. `poolside/laguna-xs-2.1-free`,
`cohere/north-mini-code-free`); run
`quorum fleet run --agent opencode --schema` for the authoritative enum.
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
