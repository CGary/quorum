package main

import (
	"database/sql"
	"fmt"
	"time"
)

func runRestoreMatched(cfg *Config, hsmeDB, legacyDB *sql.DB, report *RunReport) (*PhaseResult, error) {
	start := time.Now()
	res := &PhaseResult{Name: "backfill_matched", Status: "ok", Metadata: make(map[string]string)}

	// 1. Load legacy observations
	legacyObs, err := LoadLegacyObservations(legacyDB)
	if err != nil {
		return res, err
	}

	// 2. Query HSME for rows with source_type LIKE 'engram_migration%'
	rows, err := hsmeDB.Query("SELECT id, raw_content, source_type, created_at, project FROM memories WHERE source_type LIKE 'engram_migration%'")
	if err != nil {
		return res, fmt.Errorf("failed to query HSME memories: %w", err)
	}
	defer rows.Close()

	var matched, unmatched, errored int
	
	// Start transaction for updates
	tx, err := hsmeDB.Begin()
	if err != nil {
		return res, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	for rows.Next() {
		var id int
		var rawContent, sourceType, createdAt string
		var project sql.NullString
		if err := rows.Scan(&id, &rawContent, &sourceType, &createdAt, &project); err != nil {
			errored++
			continue
		}

		wrapped, err := ParseWrapper(rawContent)
		if err != nil {
			unmatched++
			continue
		}

		if obs, ok := legacyObs[wrapped.Content]; ok {
			// Match found! 
			if cfg.Mode != ModeDryRun {
				// Update HSME row with legacy metadata
				_, err := tx.Exec(`
					UPDATE memories 
					SET source_type = ?, 
					    project = ?, 
					    created_at = ?, 
					    updated_at = CURRENT_TIMESTAMP 
					WHERE id = ?`,
					obs.Type, obs.Project, obs.CreatedAt, id)
				if err != nil {
					errored++
				} else {
					matched++
				}
			} else {
				matched++
			}
		} else {
			unmatched++
		}
	}

	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("failed to commit transaction: %w", err)
	}

	res.Metadata["matched"] = fmt.Sprint(matched)
	res.Metadata["unmatched"] = fmt.Sprint(unmatched)
	res.Metadata["errored"] = fmt.Sprint(errored)

	// Threshold check
	total := matched + unmatched
	if total > 0 && float64(unmatched)/float64(total) > cfg.UnmatchedThreshold && cfg.Mode != ModeDryRun {
		return res, fmt.Errorf("unmatched ratio (%.2f) exceeds threshold (%.2f)", float64(unmatched)/float64(total), cfg.UnmatchedThreshold)
	}

	res.Duration = time.Since(start)
	return res, nil
}

func runRetagBornInHSME(cfg *Config, hsmeDB *sql.DB, report *RunReport) (*PhaseResult, error) {
	start := time.Now()
	res := &PhaseResult{Name: "retag_born_in_hsme", Status: "ok", Metadata: make(map[string]string)}

	// Phase 4: Identify rows tagged 'engram_session_migration'
	// and retag them as 'session_summary', parsing project from wrapper.
	rows, err := hsmeDB.Query("SELECT id, raw_content FROM memories WHERE source_type = 'engram_session_migration'")
	if err != nil {
		return res, fmt.Errorf("failed to query session migrations: %w", err)
	}
	defer rows.Close()

	tx, err := hsmeDB.Begin()
	if err != nil {
		return res, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	var retagged int
	for rows.Next() {
		var id int
		var rawContent string
		if err := rows.Scan(&id, &rawContent); err != nil {
			continue
		}

		wrapped, err := ParseWrapper(rawContent)
		if err != nil {
			continue
		}

		if cfg.Mode != ModeDryRun {
			_, err = tx.Exec("UPDATE memories SET source_type = 'session_summary', project = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", wrapped.Project, id)
			if err != nil {
				continue
			}
		}
		retagged++
	}

	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("failed to commit transaction: %w", err)
	}

	res.Metadata["retagged"] = fmt.Sprint(retagged)
	res.Duration = time.Since(start)
	return res, nil
}
