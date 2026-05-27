package core

import (
	"fmt"
	"path/filepath"
	"strings"
)

type Blueprint struct {
	AffectedFiles []string `json:"affected_files" yaml:"affected_files"`
	Symbols       []string `json:"symbols" yaml:"symbols"`
}

type RiskPolicy struct {
	SensitivePaths []string `json:"sensitive_paths" yaml:"sensitive_paths"`
}

type RiskSignals struct {
	FilesCount       int      `json:"files_count"`
	SymbolsCount     int      `json:"symbols_count"`
	SensitiveMatches []string `json:"sensitive_matches"`
}

type RiskResult struct {
	Level   string      `json:"level"`
	Reasons []string    `json:"reasons"`
	Signals RiskSignals `json:"signals"`
}

func AssignRiskLevel(blueprint Blueprint, riskPolicy RiskPolicy) RiskResult {
	affected := blueprint.AffectedFiles
	if affected == nil {
		affected = []string{}
	}
	symbols := blueprint.Symbols
	if symbols == nil {
		symbols = []string{}
	}
	globs := riskPolicy.SensitivePaths
	if globs == nil {
		globs = []string{}
	}

	var sensitiveHits []string
	for _, f := range affected {
		for _, g := range globs {
			if safeGlobMatch(f, g) {
				sensitiveHits = append(sensitiveHits, f)
				break
			}
		}
	}

	signals := RiskSignals{
		FilesCount:       len(affected),
		SymbolsCount:     len(symbols),
		SensitiveMatches: sensitiveHits,
	}

	if signals.SensitiveMatches == nil {
		signals.SensitiveMatches = []string{}
	}

	if len(sensitiveHits) > 0 {
		return RiskResult{
			Level:   "high",
			Reasons: []string{fmt.Sprintf("sensitive_path_match: %v", sensitiveHits)},
			Signals: signals,
		}
	}

	var reasons []string
	if len(affected) > 5 {
		reasons = append(reasons, fmt.Sprintf("file_count_high: %d", len(affected)))
	}
	if len(symbols) > 2 {
		reasons = append(reasons, fmt.Sprintf("symbols_count_high: %d", len(symbols)))
	}

	if len(reasons) > 0 {
		return RiskResult{
			Level:   "medium",
			Reasons: reasons,
			Signals: signals,
		}
	}

	return RiskResult{
		Level:   "low",
		Reasons: []string{"no_signals_matched"},
		Signals: signals,
	}
}

type RiskTraceEvent struct {
	Event      string      `json:"event"`
	Level      string      `json:"level,omitempty"`
	Reasons    []string    `json:"reasons"`
	Signals    RiskSignals `json:"signals,omitempty"`
	Declared   string      `json:"declared,omitempty"`
	Calculated string      `json:"calculated,omitempty"`
}

func BuildRiskTraceEvents(declaredRisk string, calculated RiskResult) []RiskTraceEvent {
	events := []RiskTraceEvent{
		{
			Event:   "risk_level_calculated",
			Level:   calculated.Level,
			Reasons: calculated.Reasons,
			Signals: calculated.Signals,
		},
	}

	if declaredRisk != "" && declaredRisk != calculated.Level {
		events = append(events, RiskTraceEvent{
			Event:      "risk_level_divergence",
			Declared:   declaredRisk,
			Calculated: calculated.Level,
			Reasons:    calculated.Reasons,
		})
	}

	return events
}

func safeGlobMatch(path string, pattern string) bool {
	if pattern == "" {
		return false
	}
	matched, err := filepath.Match(pattern, filepath.Base(path))
	if err == nil && matched {
		return true
	}
	// filepath.Match doesn't support "**" out of the box in the same way as pathlib.PurePath.match in python.
	// Python's PurePath.match("**/auth/**") matches if ANY part of the path matches "auth".
	// Let's implement a simplified glob match to match Python's behavior for this specific use case.
	// A pattern like "**/auth/**" means checking if "auth" is in the path components.
	
	if strings.HasPrefix(pattern, "**/") && strings.HasSuffix(pattern, "/**") {
		target := pattern[3 : len(pattern)-3]
		parts := strings.Split(path, string(filepath.Separator))
		for _, p := range parts {
			if p == target {
				return true
			}
		}
	}
	
	if strings.HasPrefix(pattern, "**/") {
		target := pattern[3:]
		parts := strings.Split(path, string(filepath.Separator))
		for _, p := range parts {
			matched, _ := filepath.Match(target, p)
			if matched {
				return true
			}
		}
	}
	
	if strings.Contains(pattern, "**") {
		// Just a fallback since filepath doesn't support ** natively.
		// For Quorum's risk policy, this is generally sufficient.
		target := strings.ReplaceAll(pattern, "**", "*")
		matched, _ = filepath.Match(target, path)
		return matched
	}

	matched, _ = filepath.Match(pattern, path)
	return matched
}
