package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type RunReport struct {
	RunID            string         `json:"run_id"`
	Mode             Mode           `json:"mode"`
	Timestamp        time.Time      `json:"timestamp"`
	Phases           []PhaseResult  `json:"phases"`
	Status           string         `json:"status"`
	HSMEDBPath       string         `json:"hsme_db_path"`
	LegacyDBPath     string         `json:"legacy_db_path"`
	MaxCreatedAt     string         `json:"max_created_at,omitempty"`
}

type PhaseResult struct {
	Name      string            `json:"name"`
	Status    string            `json:"status"`
	Duration  time.Duration     `json:"duration_ms"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Error     string            `json:"error,omitempty"`
}

func (r *RunReport) Save(dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create report directory: %w", err)
	}

	// Save JSON
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "report.json"), data, 0644); err != nil {
		return fmt.Errorf("failed to write report.json: %w", err)
	}

	// Save TXT summary
	txt := fmt.Sprintf("Run ID: %s\nMode: %s\nTimestamp: %s\nStatus: %s\n\nPhases:\n", r.RunID, r.Mode, r.Timestamp, r.Status)
	for _, p := range r.Phases {
		txt += fmt.Sprintf("[%s] %s (%v)\n", p.Name, p.Status, p.Duration)
		for k, v := range p.Metadata {
			txt += fmt.Sprintf("  %s=%s\n", k, v)
		}
		if p.Error != "" {
			txt += fmt.Sprintf("  ERROR: %s\n", p.Error)
		}
	}

	if err := os.WriteFile(filepath.Join(dir, "report.txt"), []byte(txt), 0644); err != nil {
		return fmt.Errorf("failed to write report.txt: %w", err)
	}

	return nil
}
