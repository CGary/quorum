# Benchmark Run: Decay OFF vs ON

Run ID: `20260426T-ranking-followup-pass`
Half-Life: 14.00 days
Database: `data/engram.db`
Eval set: `docs/future-missions/mission-3-eval-set.yaml`
Baseline: `docs/future-missions/mission-3-baseline.json`
Queries: 20

## Baseline Drift Check

Compared: 20; matched: 20; mismatched: 0; missing: 0

## Acceptance Thresholds

Overall acceptance: **PASS**

| Criterion | Required | Result | Status |
|---|---:|---:|---|
| pure_recency top-3 | 60% | 100% | PASS |
| adversarial top-3 | 80% | 80% | PASS |
| pure_relevance top-10 | 60% | 60% | PASS |
| mixed top-3 | 60% | 80% | PASS |

## Category Metrics (decay ON, fuzzy search)

| Category | N | Top-1 | Top-3 | Top-10 |
|---|---:|---:|---:|---:|
| adversarial | 5 | 20% | 80% | 80% |
| mixed | 5 | 80% | 80% | 80% |
| pure_recency | 5 | 100% | 100% | 100% |
| pure_relevance | 5 | 40% | 40% | 60% |
| overall | 20 | 60% | 75% | 80% |

## Fuzzy Search Rank Deltas

| ID | Category | Expected | OFF Rank | ON Rank | Delta |
|---|---|---:|---:|---:|---:|
| rec-01 | pure_recency | 994 | - | 1 | - |
| rec-02 | pure_recency | 911 | - | 1 | - |
| rec-03 | pure_recency | 997 | - | 1 | - |
| rec-04 | pure_recency | 1002 | 4 | 1 | +3 |
| rec-05 | pure_recency | 988 | - | 1 | - |
| rel-01 | pure_relevance | 7 | 1 | 1 | +0 |
| rel-02 | pure_relevance | 918 | - | - | - |
| rel-03 | pure_relevance | 918 | 1 | 1 | +0 |
| rel-04 | pure_relevance | 922 | - | - | - |
| rel-05 | pure_relevance | 845 | 4 | 4 | +0 |
| mix-01 | mixed | 978 | 2 | - | - |
| mix-02 | mixed | 991 | 3 | 1 | +2 |
| mix-03 | mixed | 997 | 1 | 1 | +0 |
| mix-04 | mixed | 947 | 4 | 1 | +3 |
| mix-05 | mixed | 988 | - | 1 | - |
| adv-01 | adversarial | 7 | - | - | - |
| adv-02 | adversarial | 234 | 2 | 2 | +0 |
| adv-03 | adversarial | 154 | 2 | 2 | +0 |
| adv-04 | adversarial | 11 | 2 | 2 | +0 |
| adv-05 | adversarial | 188 | 1 | 1 | +0 |

## Exact Search Samples

Exact samples executed: 20

| ID | Category | Expected | OFF Rank | ON Rank |
|---|---|---:|---:|---:|
| rec-01 | pure_recency | 994 | - | - |
| rec-02 | pure_recency | 911 | - | - |
| rec-03 | pure_recency | 997 | - | - |
| rec-04 | pure_recency | 1002 | - | - |
| rec-05 | pure_recency | 988 | - | - |
| rel-01 | pure_relevance | 7 | 1 | 1 |
| rel-02 | pure_relevance | 918 | - | - |
| rel-03 | pure_relevance | 918 | - | - |
| rel-04 | pure_relevance | 922 | - | - |
| rel-05 | pure_relevance | 845 | - | - |
| mix-01 | mixed | 978 | - | - |
| mix-02 | mixed | 991 | - | - |
| mix-03 | mixed | 997 | - | - |
| mix-04 | mixed | 947 | - | - |
| mix-05 | mixed | 988 | - | - |
| adv-01 | adversarial | 7 | - | - |
| adv-02 | adversarial | 234 | - | - |
| adv-03 | adversarial | 154 | - | - |
| adv-04 | adversarial | 11 | - | - |
| adv-05 | adversarial | 188 | - | - |
