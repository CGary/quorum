package quorumdelta

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

type QuorumMemoryEntry struct {
	ProjectID  string
	ID         string
	Type       string
	SourceTask string
	Title      string
	Context    string
	Content    string
	CreatedAt  string
	Supersedes *string
}

func OpenReadOnly(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro", path)
	if strings.HasPrefix(path, "file:") {
		if strings.Contains(path, "?") {
			dsn = path + "&mode=ro"
		} else {
			dsn = path + "?mode=ro"
		}
	}
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open read-only db: %w", err)
	}
	return db, nil
}

func FetchEntries(db *sql.DB, quorumProjectID string) ([]QuorumMemoryEntry, error) {
	query := `SELECT project_id, id, type, source_task, title, context, content, created_at, supersedes
		FROM memory_entries
		WHERE project_id = ?
		ORDER BY created_at ASC, id ASC`

	rows, err := db.Query(query, quorumProjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to query memory_entries: %w", err)
	}
	defer rows.Close()

	var entries []QuorumMemoryEntry
	for rows.Next() {
		var entry QuorumMemoryEntry
		var supersedes sql.NullString
		err := rows.Scan(
			&entry.ProjectID,
			&entry.ID,
			&entry.Type,
			&entry.SourceTask,
			&entry.Title,
			&entry.Context,
			&entry.Content,
			&entry.CreatedAt,
			&supersedes,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan memory entry: %w", err)
		}
		if supersedes.Valid && strings.TrimSpace(supersedes.String) != "" {
			val := strings.TrimSpace(supersedes.String)
			entry.Supersedes = &val
		}
		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating memory_entries rows: %w", err)
	}

	return entries, nil
}
