# Bug: `quorum fleet dispatch` fails with E2BIG on large bundles (agy/opencode inline-argv prompt)

**Detected:** 2026-07-21, MAINT-003 dispatch `8efa85d73f27` (agy / gemini-3.1-pro-low).
**Evidence:** `notes.txt`: `dispatch could not start delegate: fork/exec /home/gary/.local/bin/agy: argument list too long`; `duration_s: 0.019`, `exit_code: null`, outcome `attempt` with empty diff (`applied: false`). Preserved in MAINT-003's `07-trace.json` (done/).

## Root cause

The agy (and opencode) transports pass the bundle prompt inline as a single argv token. Linux caps each individual argv string at `MAX_ARG_STRLEN` = 128 KiB (32 pages), independent of the 2 MiB `ARG_MAX` total. MAINT-003's `prompt.md` was 160 KB → `fork/exec` fails with E2BIG before the delegate starts. FLEET-023-a's smaller bundle passed.

## Consequences

- Any task with a rich blueprint/contract context bundle (> ~120 KB prompt) cannot be dispatched via inline-argv transports.
- The failure is misclassified as `attempt` (attempt_failed) — arguably it should be `reroute:wrapper_broken`-like (infra, not model), so it currently consumes an attempt without any model ever running.

## Fix sketch (needs its own task + brief)

1. Prefer file/stdin delivery: agy supports prompt via stdin/file in `fleet run` (`--input`); the dispatch argv template should use the same mechanism (aider already uses `prompt_file`).
2. Guard in `Dispatch()`: if any rendered argv token exceeds a safe threshold (e.g. 100 KB), fail fast with a distinct outcome/cause instead of `fork/exec` noise — or auto-switch to file delivery when the transport supports it.
3. Consider ADR 0011 taxonomy: infra-failure-before-start should not be an `attempt`.

## Workaround (current)

Implement internally (`/q-implement`), as done for MAINT-003, or trim the bundle.
