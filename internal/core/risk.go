package core

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
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

// safeGlobMatch reports whether path matches the glob pattern.
//
// Non-recursive patterns (single *, ?, and exact literals) behave
// byte-identically to the previous implementation: filepath.Match is tried
// against filepath.Base(path) first (so a bare pattern like "*_test.go"
// matches at any depth, e.g. "src/core/foo_test.go"), then against the full
// path as a fallback.
//
// Patterns containing "**" delegate to doublestar.Match, which implements
// true recursive semantics: "**" matches zero or more path segments.
// Convention (pinned by tests): "dir/**" matches every path at any depth
// under dir/ AND dir itself, because doublestar's "**" can match zero
// segments. Interior "**" (e.g. "**/auth/**", "**/*.schema.*") likewise
// matches at any nesting depth.
//
// doublestar.Match uses POSIX "/" separators regardless of platform; this is
// acceptable for the core module's CGO_ENABLED=0 pure-Go POSIX target.
func safeGlobMatch(path string, pattern string) bool {
	if pattern == "" {
		return false
	}
	matched, err := filepath.Match(pattern, filepath.Base(path))
	if err == nil && matched {
		return true
	}

	if strings.Contains(pattern, "**") {
		matched, err = doublestar.Match(pattern, filepath.Clean(path))
		if err == nil && matched {
			return true
		}
		return false
	}

	matched, err = filepath.Match(pattern, path)
	return err == nil && matched
}
