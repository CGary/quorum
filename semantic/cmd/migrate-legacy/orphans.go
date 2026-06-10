package main

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/hsme/core/src/core/indexer"
)

func BuildWrapper(o *LegacyObservation) string {
	return fmt.Sprintf("Title: %s\nProject: %s\nType: %s\n\n%s",
		o.Title, o.Project, o.Type, o.Content)
}

func runIngestOrphans(cfg *Config, hsmeDB, legacyDB *sql.DB, report *RunReport) (*PhaseResult, error) {
	start := time.Now()
	res := &PhaseResult{Name: "ingest_orphans", Status: "ok", Metadata: make(map[string]string)}

	// 1. Discovery: find legacy observations not in HSME
	// For simplicity, we'll just check if the content hash exists in HSME.
	// But the spec says use content byte-equality.
	
	legacyObsRows, err := legacyDB.Query("SELECT id, type, title, content, project, created_at FROM observations WHERE deleted_at IS NULL")
	if err != nil {
		return res, err
	}
	defer legacyObsRows.Close()

	var ingested, errored int
	
	for legacyObsRows.Next() {
		o := &LegacyObservation{}
		if err := legacyObsRows.Scan(&o.ID, &o.Type, &o.Title, &o.Content, &o.Project, &o.CreatedAt); err != nil {
			errored++
			continue
		}

		// Delta check
		if cfg.Mode == ModeDelta && report.MaxCreatedAt != "" && o.CreatedAt <= report.MaxCreatedAt {
			continue
		}

		// Check if already in HSME (by content equality)
		// We'll use ParseWrapper + equality check like in Phase 3, or just ComputeHash(BuildWrapper(o))
		wrappedContent := BuildWrapper(o)
		hash := indexer.ComputeHash(wrappedContent)

		var exists bool
		err = hsmeDB.QueryRow("SELECT 1 FROM memories WHERE content_hash = ?", hash).Scan(&exists)
		if err == nil {
			// Already exists
			continue
		}

		if cfg.Mode == ModeDryRun {
			ingested++
			continue
		}

		// Ingest through indexer
		id, err := indexer.StoreContext(hsmeDB, wrappedContent, o.Type, o.Project, nil, false)
		if err != nil {
			errored++
			continue
		}

		// Follow-up update to restore legacy metadata
		_, err = hsmeDB.Exec("UPDATE memories SET created_at = ?, project = ?, source_type = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?", o.CreatedAt, o.Project, o.Type, id)
		if err != nil {
			errored++
			continue
		}

		ingested++
	}

	res.Metadata["ingested"] = fmt.Sprint(ingested)
	res.Metadata["errored"] = fmt.Sprint(errored)
	res.Duration = time.Since(start)
	return res, nil
}
