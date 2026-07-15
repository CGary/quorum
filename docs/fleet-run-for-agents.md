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

- **opencode / aider** (both backed by OpenRouter's free auto-router,
  `openrouter/free`, model_arg `openrouter/openrouter/free`, `quota_class:
  api`, $0 cost): set `OPENROUTER_API_KEY` in your environment, **or** rely on
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
| `opencode` | OpenRouter `openrouter/free` auto-router | $0 (free tier) | implement-only; agentic `--dir`-scoped edits |
| `aider` | OpenRouter `openrouter/free` auto-router | $0 (free tier) | implement-only; message-file + explicit file list |
| `agy` | Gemini (your subscription) | included in your subscription, no per-call billing | broader use; higher-effort models available (Gemini 3.1 Pro, Claude Sonnet/Opus, GPT-OSS 120B) |

Both `opencode` and `aider` are **implement-only** transports (never used for
review/diagnostic phases in Quorum's own lifecycle) and cost nothing per call
on the OpenRouter free tier — prefer them for routine edits. Use `agy` when
you need a specific higher-capability model and already have quota on your
Gemini subscription.

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

Four narrowing stages, all measured live — no number below is estimated:

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

### 7.2 Tier A — validated for agentic use (opencode)

Six models validated end-to-end through an agentic tool-use loop, ordered by
agentic speed:

| Model ID | Single-shot codegen | Agentic (opencode) | Notes |
|---|---|---|---|
| `nvidia/nemotron-3-nano-omni-30b-a3b-reasoning:free` | PASS 9/9 in 16.2s | PASS 9/9 in 11s | |
| `poolside/laguna-xs-2.1:free` | PASS 9/9 in 19.5s | PASS 9/9 in 12s | |
| `cohere/north-mini-code:free` | PASS 9/9 in 27.1s | PASS 9/9 in 14s | code-specialized |
| `nvidia/nemotron-3-ultra-550b-a55b:free` | PASS 9/9 in 3.8s | PASS 9/9 in 18s | 1M context, tool-use, no `expiration_date`, 1.1s trivial-prompt latency; fastest single-shot |
| `poolside/laguna-m.1:free` | PASS 9/9 in 99.9s (very slow) | PASS 9/9 in 19s | agentic is much faster than its single-shot |
| `nvidia/nemotron-nano-9b-v2:free` | PASS 9/9 in 57.6s (slow) | PASS 9/9 in 72s | **last-resort fallback** — slow in both modes even on a trivial single-function task (small 9B reasoning model that burns long reasoning chains before acting); expect multi-minute latencies on real multi-file tasks. Prefer the five faster entries; use this one only when they are saturated |

### 7.3 Tier B — single-shot codegen only (agentic FAIL)

These pass the hidden 9-case suite single-shot (direct completion) but are
**confirmed to fail** the agentic tool-use probe. Do not point
`opencode`/`aider` at these; use them only via direct, non-agentic API calls
(section 7.6).

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
| `nvidia/nemotron-3-super-120b-a12b:free` | no `expiration_date`; 200 OK on the trivial probe (1.0s) but 429 on 2/3 probes (persistent saturation) |
| `google/gemma-4-26b-a4b-it:free` | emitted invalid Go (stray `?` character, syntax garbage) |
| `nvidia/nemotron-nano-12b-v2-vl:free` | hallucinated `strings.Split` with 3 arguments |
| `nvidia/nemotron-3.5-content-safety:free` | safety classifier, not a general-purpose model |

Observed pattern: popular veteran free models saturate (429) as they
approach their `expiration_date`; nvidia-hosted free models are currently
the most consistently available tier.

### 7.5 How to use Tier A from Quorum fleet (internal and external projects)

`.agents/fleet/agents.yaml`'s `opencode` transport carries a pinned entry for
each of the six Tier A models, alongside the existing `openrouter/free`
auto-router fallback. The canonical map keys cannot reuse the raw OpenRouter
model IDs: `agents.schema.json`'s `models` `propertyNames` pattern
(`^[a-z0-9_.-]+/[a-z0-9_.-]+$`) allows exactly one `/` but excludes `:` from
its character class entirely, so an ID like
`nvidia/nemotron-3-ultra-550b-a55b:free` cannot be a map key as-is. Every
checked-in key substitutes `-free` for `:free`; `model_arg` carries the
exact, colon-preserving string opencode's `-m` flag actually needs:

```bash
quorum fleet run \
  --agent opencode --model "nvidia/nemotron-3-ultra-550b-a55b-free" \
  --cwd . --input - --no-input --json
```

This resolves internally to opencode arg
`-m openrouter/nvidia/nemotron-3-ultra-550b-a55b:free`. The other five keys
follow the same transform (e.g. `poolside/laguna-xs-2.1-free`,
`cohere/north-mini-code-free`); run
`quorum fleet run --agent opencode --schema` for the authoritative enum.
`openrouter/free` (the auto-router) remains available as the
availability-resilience fallback when a pinned model is saturated or its
`expiration_date` changes (section 7.7).

External projects: point `QUORUM_FLEET_AGENTS` at the Quorum repo's
`.agents/fleet/agents.yaml`, exactly as described in section 1.1 — no extra
setup is needed to reach these models.

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
