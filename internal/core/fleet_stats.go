package core

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	FleetStatsGroupByCell  = "cell"
	FleetStatsGroupByLevel = "level"
	FleetStatsGroupByBand  = "band"
	FleetStatsUnknown      = "unknown"
)

type DispatchRecord struct {
	TaskID       string  `json:"task_id"`
	DispatchID   string  `json:"dispatch_id"`
	Agent        string  `json:"agent"`
	Model        string  `json:"model"`
	Level        string  `json:"level"`
	Band         string  `json:"band"`
	OutcomeClass string  `json:"outcome_class"`
	Applied      bool    `json:"applied"`
	Noop         bool    `json:"noop"`
	Cause        string  `json:"cause,omitempty"`
	TimedOut     bool    `json:"timed_out"`
	DurationS    float64 `json:"duration_s"`
	UsageSource  string  `json:"usage_source"`
	StartedAt    string  `json:"started_at"`
}

type StatsFilter struct {
	Agent   string `json:"agent,omitempty"`
	Model   string `json:"model,omitempty"`
	Since   string `json:"since,omitempty"`
	GroupBy string `json:"group_by,omitempty"`
}

type FleetStatsReport struct {
	GroupBy      string            `json:"group_by"`
	TotalRecords int               `json:"total_records"`
	Groups       []FleetStatsGroup `json:"groups"`
}

type FleetStatsGroup struct {
	Key                   string   `json:"key"`
	Agent                 string   `json:"agent,omitempty"`
	Model                 string   `json:"model,omitempty"`
	Level                 string   `json:"level,omitempty"`
	Band                  string   `json:"band,omitempty"`
	N                     int      `json:"n"`
	Success               int      `json:"success"`
	Failure               int      `json:"failure"`
	Reroute               int      `json:"reroute"`
	Blocked               int      `json:"blocked"`
	Noop                  int      `json:"noop"`
	Timeout               int      `json:"timeout"`
	SuccessRate           float64  `json:"success_rate"`
	DurationN             int      `json:"duration_n"`
	DurationP50S          *float64 `json:"duration_p50_s"`
	DurationP95S          *float64 `json:"duration_p95_s"`
	DurationMeanS         *float64 `json:"duration_mean_s"`
	UsageMetricsAvailable bool     `json:"usage_metrics_available"`
}

func CollectDispatchRecords(projectRoot string, filter StatsFilter) ([]DispatchRecord, error) {
	since, err := parseFleetStatsSince(filter.Since)
	if err != nil {
		return nil, err
	}

	var records []DispatchRecord
	for _, state := range []string{"done", "failed"} {
		stateRoot := filepath.Join(projectRoot, ".ai", "tasks", state)
		entries, err := os.ReadDir(stateRoot)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			taskRecords, err := collectTaskDispatchRecords(filepath.Join(stateRoot, entry.Name()), filter, since)
			if err != nil {
				return nil, err
			}
			records = append(records, taskRecords...)
		}
	}

	sort.SliceStable(records, func(i, j int) bool {
		if records[i].StartedAt != records[j].StartedAt {
			return records[i].StartedAt < records[j].StartedAt
		}
		if records[i].TaskID != records[j].TaskID {
			return records[i].TaskID < records[j].TaskID
		}
		return records[i].DispatchID < records[j].DispatchID
	})
	return records, nil
}

func ComputeFleetStats(records []DispatchRecord, groupBy string) FleetStatsReport {
	groupBy = normalizeFleetStatsGroupBy(groupBy)
	groups := map[string]*FleetStatsGroup{}
	durations := map[string][]float64{}

	for _, record := range records {
		key, group := fleetStatsGroupKey(record, groupBy)
		existing, ok := groups[key]
		if !ok {
			existing = &group
			groups[key] = existing
		}

		existing.N++
		switch record.OutcomeClass {
		case "attempt":
			if record.Applied {
				existing.Success++
			} else {
				existing.Failure++
			}
		case "reroute":
			existing.Reroute++
		case "blocked":
			existing.Blocked++
		case "noop":
			existing.Noop++
		case "spawn_failed", "staging_failed":
			// Infra-level dispatch failures (process never spawned, or working-tree
			// staging failed) never produce an "attempt" outcome, but they are still
			// a failed dispatch: count them as Failure so N stays reconciled with
			// Success+Failure+Reroute+Blocked+Noop instead of silently vanishing
			// from the reliability picture this command exists to surface.
			existing.Failure++
		}
		// Deliberate non-exclusive tagging (blueprint step 2): an attempt that is
		// both unsuccessful (OutcomeClass "attempt" + !Applied, counted above as
		// Failure) and a no-op result (record.Noop) is counted in both Failure and
		// Noop. This double count is intentional, not a bug -- Noop answers "did
		// this dispatch produce no diff" independently of whether it also failed.
		if record.Noop && record.OutcomeClass != "noop" {
			existing.Noop++
		}
		if record.TimedOut {
			existing.Timeout++
		}
		if record.DurationS > 0 {
			durations[key] = append(durations[key], record.DurationS)
		}
		if record.UsageSource != "" && record.UsageSource != "none" {
			existing.UsageMetricsAvailable = true
		}
	}

	out := make([]FleetStatsGroup, 0, len(groups))
	for key, group := range groups {
		attempts := group.Success + group.Failure
		if attempts > 0 {
			group.SuccessRate = float64(group.Success) / float64(attempts)
		}
		ds := durations[key]
		if len(ds) > 0 {
			sort.Float64s(ds)
			group.DurationN = len(ds)
			group.DurationP50S = floatPtr(percentileNearestRank(ds, 0.50))
			group.DurationP95S = floatPtr(percentileNearestRank(ds, 0.95))
			group.DurationMeanS = floatPtr(meanFloat64(ds))
		}
		out = append(out, *group)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })

	return FleetStatsReport{
		GroupBy:      groupBy,
		TotalRecords: len(records),
		Groups:       out,
	}
}

type fleetStatsRoutingDecision struct {
	level string
	band  string
	ts    string
	index int
}

type fleetStatsParsedResult struct {
	record DispatchRecord
	ts     string
	path   string
}

func collectTaskDispatchRecords(taskDir string, filter StatsFilter, since *time.Time) ([]DispatchRecord, error) {
	decisions := loadFleetStatsRoutingDecisions(filepath.Join(taskDir, "07-trace.json"))
	dispatches := loadFleetStatsDispatches(filepath.Join(taskDir, "dispatch"))
	sort.SliceStable(dispatches, func(i, j int) bool {
		if dispatches[i].ts != dispatches[j].ts {
			return dispatches[i].ts < dispatches[j].ts
		}
		return dispatches[i].path < dispatches[j].path
	})

	var records []DispatchRecord
	for i, dispatch := range dispatches {
		record := dispatch.record
		record.Level = FleetStatsUnknown
		record.Band = FleetStatsUnknown
		if i < len(decisions) {
			record.Level = decisions[i].level
			record.Band = decisions[i].band
		}
		if !fleetStatsRecordMatches(record, filter, since) {
			continue
		}
		records = append(records, record)
	}
	return records, nil
}

func loadFleetStatsDispatches(dispatchRoot string) []fleetStatsParsedResult {
	entries, err := os.ReadDir(dispatchRoot)
	if err != nil {
		return nil
	}
	var out []fleetStatsParsedResult
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dispatchRoot, entry.Name(), "result.json")
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var result DispatchResult
		if err := json.Unmarshal(raw, &result); err != nil {
			continue
		}
		cause := ""
		if result.Outcome.Cause != nil {
			cause = *result.Outcome.Cause
		}
		usageSource := result.Usage.Source
		if usageSource == "" {
			usageSource = "none"
		}
		out = append(out, fleetStatsParsedResult{
			record: DispatchRecord{
				TaskID:       result.TaskID,
				DispatchID:   result.DispatchID,
				Agent:        result.Agent,
				Model:        result.Model,
				OutcomeClass: result.Outcome.Class,
				Applied:      result.Applied,
				Noop:         result.Outcome.Noop,
				Cause:        cause,
				TimedOut:     result.TimedOut,
				DurationS:    result.DurationS,
				UsageSource:  usageSource,
				StartedAt:    result.StartedAt,
			},
			ts:   result.StartedAt,
			path: path,
		})
	}
	return out
}

func loadFleetStatsRoutingDecisions(tracePath string) []fleetStatsRoutingDecision {
	raw, err := os.ReadFile(tracePath)
	if err != nil {
		return nil
	}
	var trace struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(raw, &trace); err != nil {
		return nil
	}
	var out []fleetStatsRoutingDecision
	for i, event := range trace.Events {
		if event["type"] != "routing_decision" {
			continue
		}
		decision := fleetStatsRoutingDecision{
			level: FleetStatsUnknown,
			band:  FleetStatsUnknown,
			index: i,
		}
		if ts, ok := event["ts"].(string); ok {
			decision.ts = ts
		}
		if snapshot, ok := event["inputs_snapshot"].(map[string]any); ok {
			decision.level = stringifyFleetStatsValue(snapshot["level"])
			decision.band = stringifyFleetStatsValue(snapshot["complexity_band"])
		}
		if decision.level == "" {
			decision.level = FleetStatsUnknown
		}
		if decision.band == "" {
			decision.band = FleetStatsUnknown
		}
		out = append(out, decision)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].ts != out[j].ts {
			return out[i].ts < out[j].ts
		}
		return out[i].index < out[j].index
	})
	return out
}

func fleetStatsRecordMatches(record DispatchRecord, filter StatsFilter, since *time.Time) bool {
	if filter.Agent != "" && record.Agent != filter.Agent {
		return false
	}
	if filter.Model != "" && record.Model != filter.Model {
		return false
	}
	if since != nil {
		startedAt, ok := parseFleetStatsTime(record.StartedAt)
		if !ok || startedAt.Before(*since) {
			return false
		}
	}
	return true
}

func parseFleetStatsSince(raw string) (*time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	parsed, ok := parseFleetStatsTime(raw)
	if !ok {
		return nil, fmt.Errorf("invalid --since %q: expected YYYY-MM-DD or RFC3339", raw)
	}
	return &parsed, nil
}

func parseFleetStatsTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func normalizeFleetStatsGroupBy(groupBy string) string {
	switch groupBy {
	case FleetStatsGroupByLevel, FleetStatsGroupByBand:
		return groupBy
	default:
		return FleetStatsGroupByCell
	}
}

func fleetStatsGroupKey(record DispatchRecord, groupBy string) (string, FleetStatsGroup) {
	switch groupBy {
	case FleetStatsGroupByLevel:
		level := record.Level
		if level == "" {
			level = FleetStatsUnknown
		}
		return level, FleetStatsGroup{Key: level, Level: level}
	case FleetStatsGroupByBand:
		band := record.Band
		if band == "" {
			band = FleetStatsUnknown
		}
		return band, FleetStatsGroup{Key: band, Band: band}
	default:
		key := record.Agent + "/" + record.Model
		return key, FleetStatsGroup{Key: key, Agent: record.Agent, Model: record.Model}
	}
}

// percentileNearestRank implements the nearest-rank percentile method: for a
// pre-sorted ascending slice, it takes the value at rank ceil(p*n) (1-indexed),
// clamped to [1, n]. This is a simple, deterministic method suited to the small
// sample sizes fleet stats groups typically have; it is not linear interpolation.
func percentileNearestRank(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p * float64(len(sorted))))
	if rank < 1 {
		rank = 1
	}
	if rank > len(sorted) {
		rank = len(sorted)
	}
	return sorted[rank-1]
}

func meanFloat64(values []float64) float64 {
	sum := 0.0
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func floatPtr(value float64) *float64 {
	return &value
}

func stringifyFleetStatsValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case float64:
		if math.Trunc(v) == v {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}
