# Contract: `cmd/bench-decay`

**Mission**: `universal-time-decay-for-search-results-01KQ4631`

The benchmark harness is the operator-facing executable that proves the time-decay change is both useful and safe.

---

## Purpose

`cmd/bench-decay` runs the frozen evaluation set in paired mode:
1. decay OFF
2. decay ON

It then writes side-by-side results and metric deltas under `data/benchmarks/<run_id>/`.

---

## Invocation

```bash
go run -tags "sqlite_fts5 sqlite_vec" ./cmd/bench-decay   --db data/engram.db   --eval docs/future-missions/mission-3-eval-set.yaml   --baseline docs/future-missions/mission-3-baseline.json   --half-life 14
```

### Flags

| Flag | Required | Meaning |
|------|----------|---------|
| `--db` | yes | SQLite database path. Harness opens it read-only. |
| `--eval` | yes | Frozen eval-set YAML file. |
| `--baseline` | yes | Frozen baseline JSON used for decay-off comparison. |
| `--half-life` | no | Candidate half-life in days for the decay-on pass. Default `14`. |
| `--run-id` | no | Explicit run identifier. If omitted, generated from UTC timestamp. |
| `--out` | no | Output directory root. Default `data/benchmarks`. |

---

## Read-only requirement

The harness MUST open SQLite in read-only mode (`mode=ro`) and SHOULD request immutable mode when the runtime supports it. The benchmark must not mutate the production corpus.

---

## Processing contract

For each frozen query:

1. Execute the decay-off search path.
2. Execute the decay-on search path with the selected half-life.
3. Capture top-N ranks and identifiers for comparison.
4. For at least 5 frozen queries, also run `search_exact` and record its paired ordering.
5. Compute rank deltas and category metrics.

The harness MUST preserve the frozen eval set as input-only. It cannot rewrite, normalize, or augment that file in place.

---

## Output files

### `report.json`
Machine-readable aggregate report.

Minimum fields:
- `schema_version`
- `run_id`
- `started_at`
- `finished_at`
- `db_path`
- `eval_set_path`
- `baseline_path`
- `half_life_days`
- `decay_off`
- `decay_on`
- `category_metrics`
- `exact_search_samples`

### `delta.json`
Paired comparison of OFF vs ON.

Minimum fields:
- per-query rank changes
- promoted/demoted memory ids
- top-3/top-10 hit changes
- category summaries

### `report.md`
Human-readable summary suitable for code review and mission acceptance.

It MUST call out:
- selected half-life
- pure-recency performance
- adversarial preservation
- any regression in pure-relevance or mixed queries
- byte-equivalence result for decay-off
- exact-search sample observations

---

## Success criteria

The harness is considered mission-ready when a run can demonstrate:

- decay-off output matches the frozen baseline exactly
- pure-recency top-3 hit-rate reaches at least 60%
- adversarial top-3 hit-rate remains at least 80%
- exact-search samples are included in the output

---

## Failure behavior

The harness MUST exit non-zero when:
- the DB cannot be opened read-only
- eval or baseline files are missing/unparseable
- the selected half-life is `<= 0`
- decay-off comparison detects a byte-equivalence break and `--allow-drift` was not explicitly provided (optional future escape hatch)

---

## Non-goals

- No corpus mutation
- No online tuning loop
- No automatic half-life search across many values in the first version
- No changes to `recall_recent_session`
