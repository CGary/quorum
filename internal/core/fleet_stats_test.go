package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFleetStatsCompute(t *testing.T) {
	cause := "quota"
	records := []DispatchRecord{
		{Agent: "agy", Model: "model/a", Level: "0", Band: "S", OutcomeClass: "attempt", Applied: true, DurationS: 10, UsageSource: "none"},
		{Agent: "agy", Model: "model/a", Level: "0", Band: "S", OutcomeClass: "attempt", Applied: false, TimedOut: true, DurationS: 20, UsageSource: "none"},
		{Agent: "opencode", Model: "model/b", Level: "1", Band: "M", OutcomeClass: "reroute", Cause: cause, DurationS: 30, UsageSource: "none"},
		{Agent: "opencode", Model: "model/b", Level: "1", Band: "M", OutcomeClass: "blocked", DurationS: 40, UsageSource: "none"},
		{Agent: "opencode", Model: "model/b", Level: "1", Band: "M", OutcomeClass: "attempt", Applied: true, Noop: true, DurationS: 50, UsageSource: "cli_reported"},
	}

	report := ComputeFleetStats(records, FleetStatsGroupByCell)
	if report.GroupBy != FleetStatsGroupByCell || report.TotalRecords != 5 || len(report.Groups) != 2 {
		t.Fatalf("unexpected cell report: %+v", report)
	}
	agy := findFleetStatsGroup(t, report, "agy/model/a")
	if agy.N != 2 || agy.Success != 1 || agy.Failure != 1 || agy.Timeout != 1 {
		t.Fatalf("unexpected agy counts: %+v", agy)
	}
	if agy.SuccessRate != 0.5 || *agy.DurationP50S != 10 || *agy.DurationP95S != 20 {
		t.Fatalf("unexpected agy rates/durations: %+v", agy)
	}
	if agy.UsageMetricsAvailable {
		t.Fatal("usage.source none must be unavailable, not fabricated")
	}
	opencode := findFleetStatsGroup(t, report, "opencode/model/b")
	if opencode.N != 3 || opencode.Success != 1 || opencode.Reroute != 1 || opencode.Blocked != 1 || opencode.Noop != 1 {
		t.Fatalf("unexpected opencode counts: %+v", opencode)
	}
	if !opencode.UsageMetricsAvailable {
		t.Fatal("non-none usage source should mark usage metrics available")
	}

	byLevel := ComputeFleetStats(records, FleetStatsGroupByLevel)
	if findFleetStatsGroup(t, byLevel, "0").N != 2 || findFleetStatsGroup(t, byLevel, "1").N != 3 {
		t.Fatalf("unexpected level grouping: %+v", byLevel)
	}

	byBand := ComputeFleetStats(records, FleetStatsGroupByBand)
	if findFleetStatsGroup(t, byBand, "S").N != 2 || findFleetStatsGroup(t, byBand, "M").N != 3 {
		t.Fatalf("unexpected band grouping: %+v", byBand)
	}

	empty := ComputeFleetStats(nil, "")
	if empty.GroupBy != FleetStatsGroupByCell || empty.TotalRecords != 0 || len(empty.Groups) != 0 {
		t.Fatalf("unexpected empty report: %+v", empty)
	}
}

func TestFleetStatsCollectDispatchRecords(t *testing.T) {
	root := t.TempDir()
	writeFleetStatsFixture(t, root, "active", "ACTIVE-1", "active-dispatch", "agy", "model/a", "2026-07-14T00:00:00Z")
	writeFleetStatsFixture(t, root, "inbox", "INBOX-1", "inbox-dispatch", "agy", "model/a", "2026-07-14T00:00:00Z")
	doneTask := writeFleetStatsFixture(t, root, "done", "DONE-1", "done-1", "agy", "model/a", "2026-07-15T00:00:00Z")
	writeFleetStatsResult(t, filepath.Join(doneTask, "dispatch", "done-2", "result.json"), DispatchResult{
		TaskID: "DONE-1", DispatchID: "done-2", Agent: "opencode", Model: "model/b",
		StartedAt: "2026-07-16T00:00:00Z", DurationS: 22, Outcome: DispatchOutcome{Class: "attempt"}, Applied: false,
		Usage: DispatchUsage{Source: "none"},
	})
	writeFleetStatsResult(t, filepath.Join(doneTask, "dispatch", "done-3", "result.json"), DispatchResult{
		TaskID: "DONE-1", DispatchID: "done-3", Agent: "agy", Model: "model/a",
		StartedAt: "2026-07-17T00:00:00Z", DurationS: 33, Outcome: DispatchOutcome{Class: "reroute"}, Usage: DispatchUsage{Source: "none"},
	})
	writeFleetStatsTrace(t, doneTask, []map[string]any{
		routingDecisionEvent("2026-07-15T00:00:00Z", 0, "S"),
		routingDecisionEvent("2026-07-16T00:00:00Z", 1, "M"),
	})

	failedTask := writeFleetStatsFixture(t, root, "failed", "FAILED-1", "failed-1", "aider", "model/c", "2026-07-18T00:00:00Z")
	writeFleetStatsTrace(t, failedTask, nil)

	corruptDir := filepath.Join(root, ".ai", "tasks", "done", "CORRUPT-1", "dispatch", "bad")
	if err := os.MkdirAll(corruptDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(corruptDir, "result.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".ai", "tasks", "done", "MISSING-1", "dispatch", "missing"), 0o755); err != nil {
		t.Fatal(err)
	}

	records, err := CollectDispatchRecords(root, StatsFilter{})
	if err != nil {
		t.Fatalf("CollectDispatchRecords failed: %v", err)
	}
	if len(records) != 4 {
		t.Fatalf("records len = %d, want 4: %+v", len(records), records)
	}
	for _, record := range records {
		if record.DispatchID == "active-dispatch" || record.DispatchID == "inbox-dispatch" {
			t.Fatalf("non-terminal task was collected: %+v", record)
		}
	}
	if records[0].DispatchID != "done-1" || records[0].Level != "0" || records[0].Band != "S" {
		t.Fatalf("first dispatch not enriched positionally: %+v", records[0])
	}
	if records[1].DispatchID != "done-2" || records[1].Level != "1" || records[1].Band != "M" {
		t.Fatalf("second dispatch not enriched positionally: %+v", records[1])
	}
	if records[2].DispatchID != "done-3" || records[2].Level != FleetStatsUnknown || records[2].Band != FleetStatsUnknown {
		t.Fatalf("unpaired dispatch should be unknown: %+v", records[2])
	}
	if records[3].DispatchID != "failed-1" || records[3].Level != FleetStatsUnknown || records[3].Band != FleetStatsUnknown {
		t.Fatalf("task without routing decisions should be unknown: %+v", records[3])
	}

	filtered, err := CollectDispatchRecords(root, StatsFilter{Agent: "agy", Model: "model/a", Since: "2026-07-16"})
	if err != nil {
		t.Fatalf("filtered CollectDispatchRecords failed: %v", err)
	}
	if len(filtered) != 1 || filtered[0].DispatchID != "done-3" {
		t.Fatalf("unexpected filtered records: %+v", filtered)
	}

	if _, err := CollectDispatchRecords(root, StatsFilter{Since: "not-a-date"}); err == nil {
		t.Fatal("invalid since should return an error")
	}
}

func findFleetStatsGroup(t *testing.T, report FleetStatsReport, key string) FleetStatsGroup {
	t.Helper()
	for _, group := range report.Groups {
		if group.Key == key {
			return group
		}
	}
	t.Fatalf("group %q not found in %+v", key, report)
	return FleetStatsGroup{}
}

func writeFleetStatsFixture(t *testing.T, root, state, taskID, dispatchID, agent, model, startedAt string) string {
	t.Helper()
	taskDir := filepath.Join(root, ".ai", "tasks", state, taskID)
	writeFleetStatsResult(t, filepath.Join(taskDir, "dispatch", dispatchID, "result.json"), DispatchResult{
		TaskID: taskID, DispatchID: dispatchID, Agent: agent, Model: model,
		StartedAt: startedAt, DurationS: 11, Outcome: DispatchOutcome{Class: "attempt"}, Applied: true,
		Usage: DispatchUsage{Source: "none"},
	})
	writeFleetStatsTrace(t, taskDir, []map[string]any{routingDecisionEvent(startedAt, 0, "S")})
	return taskDir
}

func writeFleetStatsResult(t *testing.T, path string, result DispatchResult) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeFleetStatsTrace(t *testing.T, taskDir string, events []map[string]any) {
	t.Helper()
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if events == nil {
		events = []map[string]any{}
	}
	raw, err := json.Marshal(map[string]any{"events": events})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(taskDir, "07-trace.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func routingDecisionEvent(ts string, level int, band string) map[string]any {
	return map[string]any{
		"type": "routing_decision",
		"ts":   ts,
		"inputs_snapshot": map[string]any{
			"level":           level,
			"complexity_band": band,
		},
	}
}
