# ADR 0014 — Jev advisory model pick (`quorum fleet pick`), outside the lifecycle router

- Status: accepted (2026-10-01, human decision)
- Context owner: fleet

## Context

Outside the SDC lifecycle, the global skills `fleet-delegate` and `think-cheap`
choose an external OpenCode Go cell by Claude's own judgment: Claude estimates
LOW/MEDIUM/HIGH, maps it to a capability class (`cheap`/`standard`/`strong`)
and `rung0-cells.py` picks a cell by ledger evidence. The human wants the
choice made per MODEL instead — each model with its own profile (task kind x
difficulty, backed by public benchmarks) — and made by Jev, TypeSafe's
"System One" classifier (typed questions in, probabilities out), with full
logging of every decision for later analysis.

## Decision

1. New command `quorum fleet pick` (`cmd/fleet_pick.go`,
   `internal/core/fleet_pick.go`, `internal/core/fleet_pick_client.go`):
   non-lifecycle, read-only, advisory. It asks Jev two `choice` questions over
   the task (`target_model`, `difficulty`) and returns a `PickDecision`
   (primary, backup, Claude fallback tier, probabilities, confidence, Jev
   version, usage/cost, latency, dropped candidates). It never executes the
   task, never writes task state, and fails OPEN to the policy's
   `fallback_pair` on any Jev error.
2. Policy as data: `.agents/policies/jev-router.yaml` holds providers
   (TypeSafe direct, OpenRouter Decisions, OpenRouter System One — swappable by
   config), the questions, the candidate profiles (ordered: order is the
   tie-break because the API does not guarantee key order; Jev's `choice` wins
   any tie), the difficulty levels with their Claude tier, and the fallback
   pair. Candidates absent from the live transport or kill-switched are dropped
   before Jev is asked, so catalog churn is a data edit.
3. New global skill `/fleet-auto` composes `fleet pick` + `fleet run`:
   Jev's pick -> its paired backup -> internal Claude subagent sized by
   Jev's difficulty, and logs every run (prompt, Jev answer, attempts, outcome) to
   `~/.claude/skills/fleet-auto/runs/YYYY-MM.jsonl`.
4. `quorum fleet route`, `/q-dispatch` and `/q-orchestrate` are NOT changed:
   the lifecycle router stays deterministic and reconstructible from
   `07-trace.json`. Jev is probabilistic and its `latest` alias moves without
   notice, so it never enters `core.Route`.

## Revision (v2, 2026-10-01, human decision)

Ranking candidates by an intelligence/price ratio was rejected: Go prices vary
~34x while the AA index varies ~2.4x, so the ratio rewarded cheap weak models
and admitted dominated ones. v2 policy:

- Candidates (7) = the Pareto frontier of the Go catalog, computed per metric
  (AA Intelligence, Terminal-Bench 2.1, SWE-bench Verified/Pro, LiveCodeBench;
  same benchmark version only; ~1-point differences are ties, cheaper wins).
- Each candidate has a `cost_tier` shown to Jev (prefer the lowest cost among
  suitable candidates) and a `max_difficulty` enforced in code against Jev's
  own difficulty answer (`ceiling_applied`).
- Each candidate has ONE paired backup (7) that Jev never sees: the best
  non-candidate on the same metric and cost tier. A 7-vs-14 visible-options
  experiment showed backups winning as primary (2/6) and lower confidence (4/6)
  when shown.
- Profiles state only evidenced claims; structured `meta` (scores, prices,
  caps, ledger) is kept for a future cost-per-solved-task ranking (rejected
  for now: public success rates are missing for half the catalog; the
  /fleet-auto run log is the data source that will make it viable).

## Consequences

- First outbound HTTP client in the core binary (stdlib `net/http`, no CGO, no
  daemon — compatible with ADR 0008). It is optional: no command other than
  `fleet pick` uses it, and `fleet pick` degrades to the fallback pair.
- Privacy (accepted by the human): the full task prompt is sent to TypeSafe or
  OpenRouter and stored in the run log. The API key is read from the
  environment (`TYPESAFE_API_KEY` / `OPENROUTER_API_KEY`) and never printed.
- Cost: Jev bills input tokens only (TypeSafe: $0.042/Mtok on 2026-10-01);
  OpenRouter reports `usage.cost` per call, which is logged.
- Profiles are opinions backed by public benchmarks, not measured routing
  accuracy. Measurement starts from the run log; a hit/miss labelling scheme is
  explicitly deferred.
