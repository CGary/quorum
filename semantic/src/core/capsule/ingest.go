package capsule

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/hsme/core/src/core/indexer"
)

// CapsuleSourceType is the source_type convention used when storing task capsules in HSME.
const CapsuleSourceType = "note"

// ImportResult contains counters and warnings resulting from a task capsule import run.
type ImportResult struct {
	Scanned  int
	Ingested int
	Skipped  int
	Errored  int
	Warnings []string
}

// Import scans tasksRoot for archived tasks, renders each into a capsule, and ingests
// it into db via indexer.StoreContext. Ingestion is content-hash idempotent with no mapping table.
// Returns an error before touching db if db is nil, or project or tasksRoot is empty.
func Import(db *sql.DB, tasksRoot string, project string) (*ImportResult, error) {
	if db == nil {
		return nil, fmt.Errorf("db cannot be nil")
	}
	if strings.TrimSpace(project) == "" {
		return nil, fmt.Errorf("project cannot be empty")
	}
	if strings.TrimSpace(tasksRoot) == "" {
		return nil, fmt.Errorf("tasksRoot cannot be empty")
	}

	tasks, scanWarnings, err := ScanArchivedTasks(tasksRoot)
	if err != nil {
		return nil, fmt.Errorf("failed to scan archived tasks: %w", err)
	}

	result := &ImportResult{
		Scanned:  len(tasks),
		Warnings: append([]string(nil), scanWarnings...),
	}

	for _, task := range tasks {
		capDoc, err := RenderCapsule(task.Dir, task.State)
		if err != nil {
			result.Errored++
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to render capsule for %s: %v", task.TaskID, err))
			continue
		}

		hash := indexer.ComputeHash(capDoc.Text)

		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM memories WHERE content_hash = ? AND status = 'active'", hash).Scan(&count)
		alreadyIngested := (err == nil && count > 0)

		_, err = indexer.StoreContext(db, capDoc.Text, CapsuleSourceType, project, nil, false)
		if err != nil {
			result.Errored++
			result.Warnings = append(result.Warnings, fmt.Sprintf("failed to store capsule for %s: %v", task.TaskID, err))
			continue
		}

		if alreadyIngested {
			result.Skipped++
		} else {
			result.Ingested++
		}
	}

	return result, nil
}
