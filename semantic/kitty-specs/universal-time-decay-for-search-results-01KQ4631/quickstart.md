# Quickstart: Universal Time-Decay for Search Results

**Mission**: `universal-time-decay-for-search-results-01KQ4631`
**Date**: 2026-04-26

This runbook shows the minimum operator flow for validating and using the universal search time-decay feature once implemented.

---

## Preconditions

- Build tags available: `sqlite_fts5 sqlite_vec`
- SQLite database present at `data/engram.db`
- Frozen eval set present at `docs/future-missions/mission-3-eval-set.yaml`
- Mission implementation merged into the local checkout

---

## 1. Verify default-safe startup (decay OFF)

```bash
RRF_TIME_DECAY=off RRF_HALF_LIFE_DAYS=14 go run -tags "sqlite_fts5 sqlite_vec" ./cmd/hsme
```

Expected outcome:
- Server starts cleanly.
- `search_fuzzy` and `search_exact` remain byte-identical to pre-mission behavior.
- No benchmark output is created yet.

---

## 2. Validate config guardrails

### Invalid flag value
```bash
RRF_TIME_DECAY=true go run -tags "sqlite_fts5 sqlite_vec" ./cmd/hsme
```
Expected: process exits non-zero with a clear message explaining that only `on` or `off` are accepted.

### Invalid half-life
```bash
RRF_TIME_DECAY=on RRF_HALF_LIFE_DAYS=0 go run -tags "sqlite_fts5 sqlite_vec" ./cmd/hsme
```
Expected: process exits non-zero with a clear message explaining that `RRF_HALF_LIFE_DAYS` must be `> 0`.

---

## 3. Run the A/B benchmark harness

```bash
RRF_TIME_DECAY=off RRF_HALF_LIFE_DAYS=14 go run -tags "sqlite_fts5 sqlite_vec" ./cmd/bench-decay   --db data/engram.db   --eval docs/future-missions/mission-3-eval-set.yaml   --baseline docs/future-missions/mission-3-baseline.json   --half-life 14
```

Expected outputs:
- `data/benchmarks/<run_id>/report.json`
- `data/benchmarks/<run_id>/report.md`
- `data/benchmarks/<run_id>/delta.json`

The harness must run both modes internally:
- decay OFF (baseline-preservation pass)
- decay ON (candidate ranking pass)

And it must exercise:
- all frozen `search_fuzzy` eval queries
- at least 5 `search_exact` samples drawn from the same frozen set

---

## 4. Inspect success metrics

Open `report.md` and confirm:

- pure-recency queries: top-3 hit-rate **≥ 60%**
- adversarial queries: top-3 hit-rate **≥ 80%**
- pure-relevance and mixed queries: no regression beyond spec thresholds
- decay-off section states output is byte-equivalent to the frozen baseline

If thresholds fail, adjust only:
- `--half-life <days>` for benchmark experiments
- `RRF_HALF_LIFE_DAYS=<days>` for runtime use

Do **not** change the frozen eval set or baseline artifacts in this mission.

---

## 5. Start the server with decay ON

```bash
RRF_TIME_DECAY=on RRF_HALF_LIFE_DAYS=14 go run -tags "sqlite_fts5 sqlite_vec" ./cmd/hsme
```

Manual spot checks to run after startup:

1. `search_fuzzy("latest changes to the schema")`
   - expect a newer schema memory to move into the top 3.
2. `search_exact("Session summary: hsme")`
   - expect the newest duplicate-title memory to rank first.
3. Re-run the same queries with `RRF_TIME_DECAY=off`
   - expect the old ordering to return.

---

## 6. Operational rollback

Rollback requires only an env var flip and process restart:

```bash
RRF_TIME_DECAY=off go run -tags "sqlite_fts5 sqlite_vec" ./cmd/hsme
```

No migration, cleanup, or data rewrite is involved.

---

## Evidence to preserve

For acceptance, keep at least one benchmark run directory under `data/benchmarks/` (or reference the exact run in the merge/verification notes) showing the selected half-life and the achieved metrics.
