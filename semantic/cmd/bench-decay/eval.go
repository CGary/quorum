package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hsme/core/src/core/search"
)

type EvalSet struct {
	SchemaVersion int         `json:"schema_version"`
	FrozenAt      string      `json:"frozen_at"`
	TotalQueries  int         `json:"total_queries"`
	Queries       []EvalQuery `json:"queries"`
}

type EvalQuery struct {
	ID       string            `json:"id"`
	Category string            `json:"category"`
	Query    string            `json:"query"`
	Expected ExpectedCriterion `json:"expected_winner_criterion"`
}

type ExpectedCriterion struct {
	ResolvedMemoryID int64 `json:"resolved_memory_id"`
	MemoryID         int64 `json:"memory_id"`
}

type Baseline struct {
	SchemaVersion int              `json:"schema_version"`
	MeasuredAt    string           `json:"measured_at"`
	TotalQueries  int              `json:"total_queries"`
	Results       []BaselineResult `json:"results"`
}

type BaselineResult struct {
	ID             string  `json:"id"`
	ActualTop10IDs []int64 `json:"actual_top_10_ids"`
	ExpectedWinner int64   `json:"expected_winner_id"`
	ExpectedRank   *int    `json:"expected_winner_rank"`
	InTop10        bool    `json:"in_top_10"`
	InTop3         bool    `json:"in_top_3"`
	InTop1         bool    `json:"in_top_1"`
}

type BenchmarkReport struct {
	SchemaVersion   int               `json:"schema_version"`
	RunID           string            `json:"run_id"`
	StartedAt       string            `json:"started_at"`
	FinishedAt      string            `json:"finished_at"`
	Config          Config            `json:"config"`
	EvalTotal       int               `json:"eval_total"`
	BaselineDrift   BaselineDrift     `json:"baseline_drift"`
	FuzzyResults    []PairedResult    `json:"fuzzy_results"`
	ExactSamples    []PairedResult    `json:"exact_samples"`
	CategoryMetrics map[string]Metric `json:"category_metrics"`
	OverallMetrics  Metric            `json:"overall_metrics"`
	Acceptance      Acceptance        `json:"acceptance"`
	Deltas          []RankDelta       `json:"deltas"`
}

type BaselineDrift struct {
	Compared   int      `json:"compared"`
	Matched    int      `json:"matched"`
	Mismatched int      `json:"mismatched"`
	Missing    int      `json:"missing"`
	IDs        []string `json:"mismatched_or_missing_ids,omitempty"`
}

type PairedResult struct {
	ID               string      `json:"id"`
	Category         string      `json:"category"`
	Query            string      `json:"query"`
	ExpectedWinnerID int64       `json:"expected_winner_id"`
	Off              []ResultRow `json:"off"`
	On               []ResultRow `json:"on"`
	OffRank          *int        `json:"off_expected_rank"`
	OnRank           *int        `json:"on_expected_rank"`
	OffTop1          bool        `json:"off_top1"`
	OffTop3          bool        `json:"off_top3"`
	OffTop10         bool        `json:"off_top10"`
	OnTop1           bool        `json:"on_top1"`
	OnTop3           bool        `json:"on_top3"`
	OnTop10          bool        `json:"on_top10"`
}

type ResultRow struct {
	MemoryID int64   `json:"memory_id"`
	Score    float64 `json:"score,omitempty"`
}

type Metric struct {
	N            int     `json:"n"`
	Top1HitRate  float64 `json:"top1_hit_rate"`
	Top3HitRate  float64 `json:"top3_hit_rate"`
	Top10HitRate float64 `json:"top10_hit_rate"`
}

type RankDelta struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Query    string `json:"query"`
	OffRank  *int   `json:"off_rank"`
	OnRank   *int   `json:"on_rank"`
	Delta    *int   `json:"rank_delta,omitempty"`
}

type Acceptance struct {
	Passed                   bool               `json:"passed"`
	PureRecencyTop3Passed    bool               `json:"pure_recency_top3_passed"`
	AdversarialTop3Passed    bool               `json:"adversarial_top3_passed"`
	PureRelevanceTop10Passed bool               `json:"pure_relevance_top10_passed"`
	MixedTop3Passed          bool               `json:"mixed_top3_passed"`
	Thresholds               map[string]float64 `json:"thresholds"`
}

func loadEvalSet(path string) (EvalSet, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return EvalSet{}, err
	}
	var set EvalSet
	if err := json.Unmarshal(b, &set); err != nil {
		return EvalSet{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(set.Queries) == 0 {
		return EvalSet{}, fmt.Errorf("eval set has no queries")
	}
	return set, nil
}

func loadBaseline(path string) (Baseline, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Baseline{}, err
	}
	var baseline Baseline
	if err := json.Unmarshal(b, &baseline); err != nil {
		return Baseline{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return baseline, nil
}

func evalSetFromQueries(raw []string) EvalSet {
	set := EvalSet{SchemaVersion: 1, FrozenAt: "ad-hoc", TotalQueries: len(raw)}
	for i, q := range raw {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		set.Queries = append(set.Queries, EvalQuery{ID: fmt.Sprintf("adhoc-%02d", i+1), Category: "ad_hoc", Query: q})
	}
	set.TotalQueries = len(set.Queries)
	return set
}

func runEval(ctx context.Context, db *sql.DB, embedder search.Embedder, cfg Config, evalSet EvalSet, baseline Baseline) (*BenchmarkReport, error) {
	started := time.Now().UTC()
	report := &BenchmarkReport{
		SchemaVersion:   1,
		RunID:           cfg.RunID,
		StartedAt:       started.Format(time.RFC3339),
		Config:          cfg,
		EvalTotal:       len(evalSet.Queries),
		CategoryMetrics: make(map[string]Metric),
	}

	baselineByID := make(map[string]BaselineResult, len(baseline.Results))
	for _, b := range baseline.Results {
		baselineByID[b.ID] = b
	}

	for i, q := range evalSet.Queries {
		expected := q.Expected.ResolvedMemoryID
		if expected == 0 {
			expected = q.Expected.MemoryID
		}

		search.GlobalDecayConfig = search.DecayConfig{Enabled: false, HalfLifeDays: cfg.HalfLife}
		fuzzyOff, err := search.FuzzySearch(ctx, db, embedder, q.Query, 10, "")
		if err != nil {
			return nil, fmt.Errorf("fuzzy off error for %q: %w", q.Query, err)
		}

		search.GlobalDecayConfig = search.DecayConfig{Enabled: true, HalfLifeDays: cfg.HalfLife}
		fuzzyOn, err := search.FuzzySearch(ctx, db, embedder, q.Query, 10, "")
		if err != nil {
			return nil, fmt.Errorf("fuzzy on error for %q: %w", q.Query, err)
		}

		paired := makePairedResult(q, expected, memoryRows(fuzzyOff), memoryRows(fuzzyOn))
		report.FuzzyResults = append(report.FuzzyResults, paired)
		report.Deltas = append(report.Deltas, makeRankDelta(paired))
		updateMetrics(report.CategoryMetrics, q.Category, paired.OnTop1, paired.OnTop3, paired.OnTop10)
		updateOneMetric(&report.OverallMetrics, paired.OnTop1, paired.OnTop3, paired.OnTop10)

		if b, ok := baselineByID[q.ID]; ok {
			report.BaselineDrift.Compared++
			if equalIDs(b.ActualTop10IDs, idsFromRows(paired.Off)) {
				report.BaselineDrift.Matched++
			} else {
				report.BaselineDrift.Mismatched++
				report.BaselineDrift.IDs = append(report.BaselineDrift.IDs, q.ID)
			}
		} else {
			report.BaselineDrift.Missing++
			report.BaselineDrift.IDs = append(report.BaselineDrift.IDs, q.ID)
		}

		// FR-012 requires at least 5 exact-search samples. Running all queries is cheap and more useful.
		search.GlobalDecayConfig = search.DecayConfig{Enabled: false, HalfLifeDays: cfg.HalfLife}
		exactOff, err := search.ExactSearch(ctx, db, q.Query, 10, "")
		if err != nil {
			return nil, fmt.Errorf("exact off error for %q: %w", q.Query, err)
		}
		search.GlobalDecayConfig = search.DecayConfig{Enabled: true, HalfLifeDays: cfg.HalfLife}
		exactOn, err := search.ExactSearch(ctx, db, q.Query, 10, "")
		if err != nil {
			return nil, fmt.Errorf("exact on error for %q: %w", q.Query, err)
		}
		report.ExactSamples = append(report.ExactSamples, makePairedResult(q, expected, exactRows(exactOff), exactRows(exactOn)))

		_ = i
	}

	finalizeMetricMap(report.CategoryMetrics)
	finalizeMetric(&report.OverallMetrics)
	report.Acceptance = evaluateAcceptance(report.CategoryMetrics)
	report.FinishedAt = time.Now().UTC().Format(time.RFC3339)
	search.GlobalDecayConfig = search.DecayConfig{Enabled: false, HalfLifeDays: cfg.HalfLife}
	return report, nil
}

func memoryRows(results []search.MemorySearchResult) []ResultRow {
	rows := make([]ResultRow, 0, len(results))
	for _, r := range results {
		rows = append(rows, ResultRow{MemoryID: r.MemoryID, Score: r.Score})
	}
	return rows
}

func exactRows(results []search.ExactMatchResult) []ResultRow {
	rows := make([]ResultRow, 0, len(results))
	for _, r := range results {
		rows = append(rows, ResultRow{MemoryID: r.MemoryID, Score: r.Score})
	}
	return rows
}

func makePairedResult(q EvalQuery, expected int64, off, on []ResultRow) PairedResult {
	offRank := rankOf(off, expected)
	onRank := rankOf(on, expected)
	return PairedResult{
		ID: q.ID, Category: q.Category, Query: q.Query, ExpectedWinnerID: expected,
		Off: off, On: on, OffRank: offRank, OnRank: onRank,
		OffTop1: inTopN(offRank, 1), OffTop3: inTopN(offRank, 3), OffTop10: inTopN(offRank, 10),
		OnTop1: inTopN(onRank, 1), OnTop3: inTopN(onRank, 3), OnTop10: inTopN(onRank, 10),
	}
}

func makeRankDelta(p PairedResult) RankDelta {
	d := RankDelta{ID: p.ID, Category: p.Category, Query: p.Query, OffRank: p.OffRank, OnRank: p.OnRank}
	if p.OffRank != nil && p.OnRank != nil {
		delta := *p.OffRank - *p.OnRank
		d.Delta = &delta
	}
	return d
}

func rankOf(rows []ResultRow, id int64) *int {
	if id == 0 {
		return nil
	}
	for i, row := range rows {
		if row.MemoryID == id {
			rank := i + 1
			return &rank
		}
	}
	return nil
}

func inTopN(rank *int, n int) bool { return rank != nil && *rank <= n }

func idsFromRows(rows []ResultRow) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.MemoryID)
	}
	return ids
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func updateMetrics(metrics map[string]Metric, category string, top1, top3, top10 bool) {
	m := metrics[category]
	updateOneMetric(&m, top1, top3, top10)
	metrics[category] = m
}

func updateOneMetric(m *Metric, top1, top3, top10 bool) {
	m.N++
	if top1 {
		m.Top1HitRate++
	}
	if top3 {
		m.Top3HitRate++
	}
	if top10 {
		m.Top10HitRate++
	}
}

func finalizeMetricMap(metrics map[string]Metric) {
	for k, m := range metrics {
		finalizeMetric(&m)
		metrics[k] = m
	}
}

func finalizeMetric(m *Metric) {
	if m.N == 0 {
		return
	}
	n := float64(m.N)
	m.Top1HitRate /= n
	m.Top3HitRate /= n
	m.Top10HitRate /= n
}

func evaluateAcceptance(metrics map[string]Metric) Acceptance {
	a := Acceptance{
		Thresholds: map[string]float64{
			"pure_recency_top3":    0.60,
			"adversarial_top3":     0.80,
			"pure_relevance_top10": 0.60,
			"mixed_top3":           0.60,
		},
	}
	a.PureRecencyTop3Passed = metrics["pure_recency"].Top3HitRate >= a.Thresholds["pure_recency_top3"]
	a.AdversarialTop3Passed = metrics["adversarial"].Top3HitRate >= a.Thresholds["adversarial_top3"]
	a.PureRelevanceTop10Passed = metrics["pure_relevance"].Top10HitRate >= a.Thresholds["pure_relevance_top10"]
	a.MixedTop3Passed = metrics["mixed"].Top3HitRate >= a.Thresholds["mixed_top3"]
	a.Passed = a.PureRecencyTop3Passed && a.AdversarialTop3Passed && a.PureRelevanceTop10Passed && a.MixedTop3Passed
	return a
}
