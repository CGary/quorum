# Specification Quality Checklist: Universal Time-Decay for Search Results

**Purpose**: Validate specification completeness and quality before proceeding to planning
**Created**: 2026-04-26
**Feature**: [spec.md](../spec.md)

## Content Quality

- [x] No implementation details (languages, frameworks, APIs)
- [x] Focused on user value and business needs
- [x] Written for non-technical stakeholders
- [x] All mandatory sections completed

## Requirement Completeness

- [x] No [NEEDS CLARIFICATION] markers remain
- [x] Requirements are testable and unambiguous
- [x] Requirement types are separated (Functional / Non-Functional / Constraints)
- [x] IDs are unique across FR-###, NFR-###, and C-### entries
- [x] All requirement rows include a non-empty Status value
- [x] Non-functional requirements include measurable thresholds
- [x] Success criteria are measurable
- [x] Success criteria are technology-agnostic (no implementation details)
- [x] All acceptance scenarios are defined
- [x] Edge cases are identified
- [x] Scope is clearly bounded
- [x] Dependencies and assumptions identified

## Feature Readiness

- [x] All functional requirements have clear acceptance criteria
- [x] User scenarios cover primary flows
- [x] Feature meets measurable outcomes defined in Success Criteria
- [x] No implementation details leak into specification

## Notes

### Validation pass — 2026-04-26

- 15 FRs, 7 NFRs, 8 constraints — all complete with status fields.
- All NFR thresholds are anchored to measured baseline numbers from `docs/future-missions/mission-3-baseline.json`. No invented thresholds.
  - NFR-002 target ≥60% top-3 vs baseline 0% — defendible improvement target.
  - NFR-003 ≥80% top-3 — exact baseline number, must NOT regress.
  - NFR-004 ≥60% top-10 — exact baseline number, must NOT regress.
  - NFR-005 ≥60% top-3 — slightly below baseline (60%) to allow for the cost of decay; if regression observed, half-life is the tuning lever.
  - NFR-007 100% byte equality on decay-off vs baseline — non-negotiable safety check.
- The operator's official directive (Universal Implementation, Option B) is captured in the spec's Purpose context AND in Assumption #1 with explicit interpretation of the "tie-breaker" framing.
- search_exact baseline measurement is acknowledged as a NEW deliverable of this mission (Assumption #4) — it does not exist in the frozen baseline.
- The frozen eval set is treated as immutable input (FR-015, C-006).
- recall_recent_session (Mission 2) is explicitly out of scope for modification (C-007).
- All assumptions and out-of-scope items derive from explicit statements in the operator directive or the mission-3 draft.

### Open advisories (non-blocking, for plan phase)

- The plan must specify HOW the decay is wired into `search_fuzzy` and `search_exact` without breaking the byte-equivalence invariant (NFR-007). The simplest path is: branch on the flag at the top of each function and skip the decay code path entirely when off.
- The plan must specify WHERE the A/B harness lives — likely under `cmd/bench-decay/` matching the convention of other binaries, or as a `tests/benchmarks/` subdirectory.
- search_exact's BM25 score is not directly exposed by the SQLite FTS5 wrapper used in `src/core/search/fuzzy.go:ExactSearch`. The plan must decide whether to use the `bm25(memory_chunks_fts)` virtual column or to compute decay against the rank position instead.
