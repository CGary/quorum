# 0013: Task Capsules — HSME Ingestion of Lifecycle Artifacts

**Date:** 2026-08-09
**Status:** Accepted

## Context

ADR 0008 merged HSME into this repository as the subordinate semantic layer and defined a
unidirectional data frontier: a semantic-side importer reads Quorum's curated memory DB
(`~/.quorum/memory.db`, `?mode=ro`) and never writes back. Its execution order (steps 2–4:
delta importer, duplicate advisor, native near-dup) is still pending.

Curated memory is not the only knowledge Quorum produces. Every task leaves lifecycle
artifacts — intent (`00-spec.yaml`), design decisions (`01-blueprint.yaml`), bounded contract
(`02-contract.yaml`), outcome (`05-validation.json`, `06-review.json`), and evidence trail
(`07-trace.json`). Failure knowledge is especially valuable: today `internal/core/failure_lookup.go`
finds related failed tasks lexically (affected-file overlap ≥ 50%); semantically similar failures
with disjoint file sets are invisible to it. These artifacts live in `.ai/tasks/{done,failed}/`,
which is gitignored local state.

ADR 0008 specifies only curated memory as an import source. Extending ingestion to lifecycle
artifacts is not prohibited — it flows in the permitted direction (Quorum → HSME) — but it is
unspecified. This ADR specifies it.

## Decision

### 1. One importer, two sources

The semantic-side importer (ADR 0008 step 2) is designed from the start with two sources behind
one architecture:

- **Curated memory delta** — unchanged from ADR 0008: reads `~/.quorum/memory.db` read-only,
  maps `project_id` → HSME `project`, renders deterministic `raw_content`, maintains the
  Quorum-ID→HSME-ID translation table for `supersedes` edges.
- **Task capsules** — one deterministic compact document per archived task (in `done/` or
  `failed/`), rendered from its artifacts.

### 2. Capsule shape

A capsule is a deterministic plain-text rendering, NOT a raw YAML/JSON dump. It contains, in
fixed order: task ID and final state; intent summary (from `00-spec`); key design decisions
(from `01-blueprint`); contract surface (touch list, from `02-contract`); outcome (validation
exit status, review verdict); and, for failed tasks, the failure evidence (validation excerpts,
review notes, relevant trace events). Contracts are never ingested standalone — they only
appear inside their task's capsule.

Idempotency: one capsule per `task_id`; re-import is deduplicated by content hash (HSME computes
its own hash per ADR 0008). Every ingestion call sets `source_type` and `project` explicitly.

### 3. Trigger is semantic-side only

The importer runs as a manual command, a scheduled job, or an `hsme-worker` task — always on the
semantic side. The `quorum` binary never invokes HSME (ADR 0008 query frontier), and no task
transition (`quorum task clean`, `task split`, etc.) may call back into the importer. The
importer discovers archived tasks by scanning `.ai/tasks/{done,failed}/` read-only.

### 4. Non-rebuildability is declared and accepted

Task directories are gitignored runtime state. The capsule corpus therefore CANNOT be rebuilt
from a clean clone — unlike the curated-memory import, whose source DB is user-managed. This is
accepted because HSME is advisory recall, never truth: losing capsules degrades suggestion
quality, never correctness. The curated `q-memory` library remains the durable, rebuildable
knowledge store.

### 5. Additive to deterministic paths

`internal/core/failure_lookup.go` and every other deterministic core path stay untouched and
authoritative. Semantic recall over capsules is additive at the skill layer (ADR 0008 step 3+),
with explicit timeouts and graceful degradation: HSME down → skills behave exactly as today.

### 6. SDC governance for semantic/ tasks

Tasks whose contract touches `semantic/` run the standard `/q-*` lifecycle in this repo (the
worktree covers the whole monorepo), with two calibrations learned from HSME-001: their
`verify.commands` MUST run the semantic module's own suite (`cd semantic && just test`, CGO +
build tags), and blueprints touching provider/extractor code MUST include
`semantic/src/bootstrap/` wiring in the `context_bundle` so reviews can see cross-file wiring
(`semantic/src/bootstrap/**` is now a sensitive path in `.agents/policies/risk.yaml`).

## Consequences

- **Positive:** the duplicate advisor and read hooks (ADR 0008 steps 3–4) gain a corpus that
  includes real task history — "this resembles failed task X" becomes answerable semantically.
  One importer architecture avoids building two.
- **Negative / accepted:** capsule corpus is not rebuildable from Git (declared above);
  embeddings are eventually consistent (a fresh capsule may be invisible to the advisor until
  the worker processes it); the importer must tolerate schema drift in old archived tasks
  (missing artifacts → render what exists, never fail the batch).
- **Unchanged:** all ADR 0008 prohibitions (no cross-module imports, no reverse data flow, no
  HSME ingestion into curated memory, HSME never a gate), Constitution rules 1–10, and the
  append-only trace.
