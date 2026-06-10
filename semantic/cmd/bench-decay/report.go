package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func writeReports(runDir string, report *BenchmarkReport) error {
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(runDir, "report.json"), b, 0644); err != nil {
		return err
	}

	delta := struct {
		RunID         string            `json:"run_id"`
		HalfLifeDays  float64           `json:"half_life_days"`
		BaselineDrift BaselineDrift     `json:"baseline_drift"`
		Deltas        []RankDelta       `json:"deltas"`
		Metrics       map[string]Metric `json:"category_metrics"`
	}{
		RunID:         report.RunID,
		HalfLifeDays:  report.Config.HalfLife,
		BaselineDrift: report.BaselineDrift,
		Deltas:        report.Deltas,
		Metrics:       report.CategoryMetrics,
	}
	d, err := json.MarshalIndent(delta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(runDir, "delta.json"), d, 0644); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(runDir, "report.md"), []byte(renderMarkdown(report)), 0644); err != nil {
		return err
	}
	return nil
}

func renderMarkdown(report *BenchmarkReport) string {
	md := fmt.Sprintf("# Benchmark Run: Decay OFF vs ON\n\nRun ID: `%s`\nHalf-Life: %.2f days\nDatabase: `%s`\nEval set: `%s`\nBaseline: `%s`\nQueries: %d\n\n",
		report.RunID, report.Config.HalfLife, report.Config.DBPath, report.Config.EvalPath, report.Config.BaselinePath, report.EvalTotal)

	md += "## Baseline Drift Check\n\n"
	md += fmt.Sprintf("Compared: %d; matched: %d; mismatched: %d; missing: %d\n\n",
		report.BaselineDrift.Compared, report.BaselineDrift.Matched, report.BaselineDrift.Mismatched, report.BaselineDrift.Missing)
	if len(report.BaselineDrift.IDs) > 0 {
		md += fmt.Sprintf("Mismatched/missing IDs: `%v`\n\n", report.BaselineDrift.IDs)
	}

	md += "## Acceptance Thresholds\n\n"
	md += fmt.Sprintf("Overall acceptance: **%s**\n\n", passFail(report.Acceptance.Passed))
	md += "| Criterion | Required | Result | Status |\n|---|---:|---:|---|\n"
	md += fmt.Sprintf("| pure_recency top-3 | %.0f%% | %.0f%% | %s |\n", report.Acceptance.Thresholds["pure_recency_top3"]*100, report.CategoryMetrics["pure_recency"].Top3HitRate*100, passFail(report.Acceptance.PureRecencyTop3Passed))
	md += fmt.Sprintf("| adversarial top-3 | %.0f%% | %.0f%% | %s |\n", report.Acceptance.Thresholds["adversarial_top3"]*100, report.CategoryMetrics["adversarial"].Top3HitRate*100, passFail(report.Acceptance.AdversarialTop3Passed))
	md += fmt.Sprintf("| pure_relevance top-10 | %.0f%% | %.0f%% | %s |\n", report.Acceptance.Thresholds["pure_relevance_top10"]*100, report.CategoryMetrics["pure_relevance"].Top10HitRate*100, passFail(report.Acceptance.PureRelevanceTop10Passed))
	md += fmt.Sprintf("| mixed top-3 | %.0f%% | %.0f%% | %s |\n\n", report.Acceptance.Thresholds["mixed_top3"]*100, report.CategoryMetrics["mixed"].Top3HitRate*100, passFail(report.Acceptance.MixedTop3Passed))

	md += "## Category Metrics (decay ON, fuzzy search)\n\n"
	md += "| Category | N | Top-1 | Top-3 | Top-10 |\n|---|---:|---:|---:|---:|\n"
	keys := make([]string, 0, len(report.CategoryMetrics))
	for k := range report.CategoryMetrics {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		m := report.CategoryMetrics[k]
		md += fmt.Sprintf("| %s | %d | %.0f%% | %.0f%% | %.0f%% |\n", k, m.N, m.Top1HitRate*100, m.Top3HitRate*100, m.Top10HitRate*100)
	}
	md += fmt.Sprintf("| overall | %d | %.0f%% | %.0f%% | %.0f%% |\n\n", report.OverallMetrics.N, report.OverallMetrics.Top1HitRate*100, report.OverallMetrics.Top3HitRate*100, report.OverallMetrics.Top10HitRate*100)

	md += "## Fuzzy Search Rank Deltas\n\n"
	md += "| ID | Category | Expected | OFF Rank | ON Rank | Delta |\n|---|---|---:|---:|---:|---:|\n"
	for _, r := range report.FuzzyResults {
		md += fmt.Sprintf("| %s | %s | %d | %s | %s | %s |\n", r.ID, r.Category, r.ExpectedWinnerID, rankString(r.OffRank), rankString(r.OnRank), deltaString(r.OffRank, r.OnRank))
	}

	md += "\n## Exact Search Samples\n\n"
	md += fmt.Sprintf("Exact samples executed: %d\n\n", len(report.ExactSamples))
	md += "| ID | Category | Expected | OFF Rank | ON Rank |\n|---|---|---:|---:|---:|\n"
	for _, r := range report.ExactSamples {
		md += fmt.Sprintf("| %s | %s | %d | %s | %s |\n", r.ID, r.Category, r.ExpectedWinnerID, rankString(r.OffRank), rankString(r.OnRank))
	}
	return md
}

func rankString(rank *int) string {
	if rank == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *rank)
}

func deltaString(off, on *int) string {
	if off == nil || on == nil {
		return "-"
	}
	return fmt.Sprintf("%+d", *off-*on)
}

func passFail(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}
