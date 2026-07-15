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
