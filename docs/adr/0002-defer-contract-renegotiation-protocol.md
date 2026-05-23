# ADR 0002: Defer Automated Contract Renegotiation Protocol

## Status
Accepted for F-02

## Context
Quorum's contract boundary is intentionally strict: implementation work may only touch files authorized by `02-contract.yaml`. A future v1.2 protocol may allow a blocked implementation to request contract renegotiation, but adding that loop now would change dispatcher behavior, retry policy, and task authority boundaries before enough operational data exists.

## Decision
Defer the full automated contract renegotiation protocol. For now Quorum only standardizes the manual block signal and records passive telemetry in `07-trace.json`.

The manual signal shape is:

```text
BLOCKED: missing_file=<path>; reason=<text>; severity=<critical|minor>
```

The parser maps this text to the future request shape:

- `path`
- `reason`
- `severity`

`blocked_by_contract` telemetry records only occurrence count and requested paths. It must not alter task state, dispatch, routing, retries, model selection, validation finality, or contract scope.

## Section 7 Preconditions for v1.2
Before implementing automated renegotiation, Quorum needs:

1. A dispatcher-owned renegotiation entry point with explicit human or policy authorization.
2. A schema-backed renegotiation request artifact or trace event that preserves append-only history.
3. A clear rule for when `q-blueprint` may revise `01-blueprint.yaml` and `02-contract.yaml` without weakening Constitution Rule #3.
4. Tests proving renegotiation cannot mark a task done, waive `verify.commands`, or bypass review.
5. Routing policy that uses executor levels, never hardcoded model names.

## Consequences
Positive: F-02 enables data collection and parser compatibility without changing lifecycle authority.

Negative: blocked implementations still require manual intervention until the deferred protocol is designed.

Neutral: no new numbered artifact is introduced; telemetry remains inside `07-trace.json`.
