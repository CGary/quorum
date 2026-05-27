package core

import (
	"testing"
)

func TestAssignRiskLevel(t *testing.T) {
	policy := RiskPolicy{
		SensitivePaths: []string{"**/auth/**", "*.schema.*"},
	}

	t.Run("high risk via glob match", func(t *testing.T) {
		bp := Blueprint{AffectedFiles: []string{"src/auth/login.py"}}
		res := AssignRiskLevel(bp, policy)
		if res.Level != "high" {
			t.Errorf("expected high, got %q", res.Level)
		}
		if len(res.Signals.SensitiveMatches) != 1 || res.Signals.SensitiveMatches[0] != "src/auth/login.py" {
			t.Errorf("unexpected matches: %v", res.Signals.SensitiveMatches)
		}
	})

	t.Run("high risk via exact match", func(t *testing.T) {
		bp := Blueprint{AffectedFiles: []string{"spec.schema.json"}}
		res := AssignRiskLevel(bp, policy)
		if res.Level != "high" {
			t.Errorf("expected high, got %q", res.Level)
		}
	})

	t.Run("medium risk via file count", func(t *testing.T) {
		bp := Blueprint{AffectedFiles: []string{"1", "2", "3", "4", "5", "6"}}
		res := AssignRiskLevel(bp, policy)
		if res.Level != "medium" {
			t.Errorf("expected medium, got %q", res.Level)
		}
		if res.Reasons[0] != "file_count_high: 6" {
			t.Errorf("unexpected reason: %q", res.Reasons[0])
		}
	})

	t.Run("medium risk via symbol count", func(t *testing.T) {
		bp := Blueprint{
			AffectedFiles: []string{"1"},
			Symbols:       []string{"A", "B", "C"},
		}
		res := AssignRiskLevel(bp, policy)
		if res.Level != "medium" {
			t.Errorf("expected medium, got %q", res.Level)
		}
		if res.Reasons[0] != "symbols_count_high: 3" {
			t.Errorf("unexpected reason: %q", res.Reasons[0])
		}
	})

	t.Run("low risk", func(t *testing.T) {
		bp := Blueprint{AffectedFiles: []string{"1"}}
		res := AssignRiskLevel(bp, policy)
		if res.Level != "low" {
			t.Errorf("expected low, got %q", res.Level)
		}
	})
}

func TestBuildRiskTraceEvents(t *testing.T) {
	calculated := RiskResult{Level: "medium", Reasons: []string{"foo"}}
	
	t.Run("no divergence", func(t *testing.T) {
		events := BuildRiskTraceEvents("medium", calculated)
		if len(events) != 1 {
			t.Errorf("expected 1 event, got %d", len(events))
		}
		if events[0].Event != "risk_level_calculated" {
			t.Errorf("expected risk_level_calculated, got %q", events[0].Event)
		}
	})

	t.Run("divergence", func(t *testing.T) {
		events := BuildRiskTraceEvents("high", calculated)
		if len(events) != 2 {
			t.Errorf("expected 2 events, got %d", len(events))
		}
		if events[1].Event != "risk_level_divergence" {
			t.Errorf("expected divergence event, got %q", events[1].Event)
		}
		if events[1].Declared != "high" || events[1].Calculated != "medium" {
			t.Errorf("unexpected declared/calculated values")
		}
	})
}

func TestSafeGlobMatch(t *testing.T) {
	cases := []struct {
		path    string
		pattern string
		match   bool
	}{
		{"src/auth/login.py", "**/auth/**", true},
		{"src/login.py", "**/auth/**", false},
		{"spec.schema.json", "*.schema.*", true},
		{"tests/test_something.py", "tests/**", true},
		{"package.json", "package.json", true},
	}

	for _, tc := range cases {
		if got := safeGlobMatch(tc.path, tc.pattern); got != tc.match {
			t.Errorf("safeGlobMatch(%q, %q) = %v; want %v", tc.path, tc.pattern, got, tc.match)
		}
	}
}
