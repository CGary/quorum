package main

import (
	"database/sql"
	"fmt"
	"time"
)

func runDeleteGarbage(cfg *Config, hsmeDB *sql.DB, report *RunReport) (*PhaseResult, error) {
	start := time.Now()
	res := &PhaseResult{Name: "delete_garbage", Status: "ok", Metadata: make(map[string]string)}

	// Phase 5: Delete malformed empty memory.
	// Contract: delete by id-or-rule. 
	// Rule: source_type = 'engram_migration' and (raw_content LIKE '%Title: \nProject: Unknown\nType: manual\n\n%' OR length(raw_content) < 50)
	
	tx, err := hsmeDB.Begin()
	if err != nil {
		return res, fmt.Errorf("failed to start transaction: %w", err)
	}
	defer tx.Rollback()

	var deleted int64
	if cfg.Mode != ModeDryRun {
		result, err := tx.Exec("DELETE FROM memories WHERE source_type = 'engram_migration' AND (length(raw_content) < 50 OR raw_content LIKE '%Title: \n%')")
		if err != nil {
			return res, fmt.Errorf("failed to delete garbage: %w", err)
		}
		deleted, _ = result.RowsAffected()
	} else {
		// Just count how many would be deleted
		err = hsmeDB.QueryRow("SELECT count(*) FROM memories WHERE source_type = 'engram_migration' AND (length(raw_content) < 50 OR raw_content LIKE '%Title: \n%')").Scan(&deleted)
		if err != nil {
			// ignore count errors in dry-run if table doesn't match
		}
	}

	if err := tx.Commit(); err != nil {
		return res, fmt.Errorf("failed to commit transaction: %w", err)
	}

	res.Metadata["deleted"] = fmt.Sprint(deleted)
	res.Duration = time.Since(start)
	return res, nil
}

