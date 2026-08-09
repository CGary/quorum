package quorumdelta

import (
	"database/sql"
	"fmt"
	"strings"
)

func LookupHSMEID(hsmeDB *sql.DB, hsmeProject string, quorumID string) (*int64, error) {
	quorumID = strings.TrimSpace(quorumID)
	if quorumID == "" {
		return nil, nil
	}

	var hsmeID int64
	err := hsmeDB.QueryRow(
		"SELECT hsme_memory_id FROM quorumdelta_id_map WHERE project = ? AND quorum_id = ?",
		hsmeProject, quorumID,
	).Scan(&hsmeID)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to lookup HSME ID mapping: %w", err)
	}

	return &hsmeID, nil
}

func RecordMapping(hsmeDB *sql.DB, hsmeProject string, quorumID string, hsmeMemoryID int64) error {
	quorumID = strings.TrimSpace(quorumID)
	if quorumID == "" {
		return fmt.Errorf("quorumID cannot be empty")
	}

	query := `INSERT INTO quorumdelta_id_map (project, quorum_id, hsme_memory_id, imported_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(project, quorum_id) DO UPDATE SET
			hsme_memory_id = excluded.hsme_memory_id,
			imported_at = excluded.imported_at`

	_, err := hsmeDB.Exec(query, hsmeProject, quorumID, hsmeMemoryID)
	if err != nil {
		return fmt.Errorf("failed to record quorumdelta mapping: %w", err)
	}

	return nil
}
