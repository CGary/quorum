package search

import (
	"context"
	"database/sql"
	"fmt"
)

// RecallRecentSession retrieves the most recent active session summaries.
// It uses a pure SQL lookup ordered by created_at DESC and id DESC.
func RecallRecentSession(ctx context.Context, db *sql.DB, limit int, project string) ([]MemorySearchResult, error) {
	if limit <= 0 {
		limit = 5
	}
	if limit > 50 {
		limit = 50
	}

	var rows *sql.Rows
	var err error

	if project != "" {
		query := `
			SELECT id, raw_content
			FROM memories
			WHERE source_type = 'session_summary' AND status = 'active' AND superseded_by IS NULL AND project = ?
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		`
		rows, err = db.QueryContext(ctx, query, project, limit)
	} else {
		query := `
			SELECT id, raw_content
			FROM memories
			WHERE source_type = 'session_summary' AND status = 'active' AND superseded_by IS NULL
			ORDER BY created_at DESC, id DESC
			LIMIT ?
		`
		rows, err = db.QueryContext(ctx, query, limit)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to execute recall query: %w", err)
	}
	defer rows.Close()

	var results []MemorySearchResult
	for rows.Next() {
		var id int64
		var rawContent string
		if err := rows.Scan(&id, &rawContent); err != nil {
			return nil, fmt.Errorf("failed to scan recall result: %w", err)
		}
		
		// Map the full memory to the existing MemorySearchResult structure
		res := MemorySearchResult{
			MemoryID:       id,
			Score:          1.0, // Chronological, not relevance-based
			IsSuperseded:   false, // Query already filters out superseded
			VectorCoverage: "none", // No vectors used
			Highlights: []ChunkHighlight{
				{
					ChunkID:    id, // Using memory ID as a proxy for chunk ID
					ChunkIndex: 0,
					Text:       rawContent, // Full content
				},
			},
		}
		results = append(results, res)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating recall results: %w", err)
	}

	return results, nil
}
