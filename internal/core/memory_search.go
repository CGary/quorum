package core

import (
	"database/sql"
	"fmt"
	"strings"
)

const DefaultMemorySearchLimit = 20

// MemorySearchOptions describes a read-only memory search request.
type MemorySearchOptions struct {
	Query       string
	Type        string
	ProjectID   string
	AllProjects bool
	Limit       int
}

// MemorySearchResult is a stable DTO returned by memory search.
type MemorySearchResult struct {
	ProjectID    string   `json:"project_id,omitempty"`
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	Title        string   `json:"title,omitempty"`
	Context      string   `json:"context,omitempty"`
	Content      string   `json:"content"`
	AntiPatterns []string `json:"anti_patterns"`
}

// SearchMemoryEntries searches memory entries without mutating the database.
func SearchMemoryEntries(db *sql.DB, opts MemorySearchOptions) ([]MemorySearchResult, error) {
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	if opts.Type != "" && opts.Type != "decision" && opts.Type != "pattern" && opts.Type != "lesson" {
		return nil, fmt.Errorf("invalid memory type %q", opts.Type)
	}
	limit := opts.Limit
	if limit == 0 {
		limit = DefaultMemorySearchLimit
	}
	if limit < 1 {
		return nil, fmt.Errorf("limit must be greater than 0")
	}
	if !opts.AllProjects && strings.TrimSpace(opts.ProjectID) == "" {
		return nil, fmt.Errorf("project_id is required unless all-projects is set")
	}

	if memoryFTSAvailable(db) {
		results, err := searchMemoryFTS(db, query, opts, limit)
		if err == nil && len(results) > 0 {
			return results, nil
		}
	}
	return searchMemoryLike(db, query, opts, limit)
}

func searchMemoryFTS(db *sql.DB, query string, opts MemorySearchOptions, limit int) ([]MemorySearchResult, error) {
	where := []string{"memory_fts MATCH ?"}
	args := []any{query}
	if opts.Type != "" {
		where = append(where, "type = ?")
		args = append(args, opts.Type)
	}
	if !opts.AllProjects {
		where = append(where, "project_id = ?")
		args = append(args, opts.ProjectID)
	}
	args = append(args, limit)

	stmt := `SELECT id, COALESCE(project_id, ''), type, COALESCE(title, ''), COALESCE(context, ''), content, COALESCE(anti_patterns, '')
		FROM memory_fts WHERE ` + strings.Join(where, " AND ") + ` ORDER BY rank LIMIT ?`
	return scanMemorySearchRows(db.Query(stmt, args...))
}

func searchMemoryLike(db *sql.DB, query string, opts MemorySearchOptions, limit int) ([]MemorySearchResult, error) {
	pattern := "%" + strings.ToLower(query) + "%"
	where := []string{`(
		LOWER(COALESCE(e.title, '')) LIKE ? OR
		LOWER(COALESCE(e.context, '')) LIKE ? OR
		LOWER(e.content) LIKE ? OR
		LOWER(COALESCE((SELECT group_concat(ap.content, ' ') FROM memory_anti_patterns ap WHERE ap.memory_id = e.id AND ap.project_id = e.project_id), '')) LIKE ?
	)`}
	args := []any{pattern, pattern, pattern, pattern}
	if opts.Type != "" {
		where = append(where, "e.type = ?")
		args = append(args, opts.Type)
	}
	if !opts.AllProjects {
		where = append(where, "e.project_id = ?")
		args = append(args, opts.ProjectID)
	}
	args = append(args, limit)

	stmt := `SELECT e.id, COALESCE(e.project_id, ''), e.type, COALESCE(e.title, ''), COALESCE(e.context, ''), e.content,
		COALESCE((SELECT group_concat(ap.content, char(10)) FROM memory_anti_patterns ap WHERE ap.memory_id = e.id AND ap.project_id = e.project_id), '')
		FROM memory_entries e WHERE ` + strings.Join(where, " AND ") + ` ORDER BY e.created_at DESC, e.id ASC LIMIT ?`
	return scanMemorySearchRows(db.Query(stmt, args...))
}

func scanMemorySearchRows(rows *sql.Rows, err error) ([]MemorySearchResult, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []MemorySearchResult{}
	for rows.Next() {
		var r MemorySearchResult
		var anti string
		if err := rows.Scan(&r.ID, &r.ProjectID, &r.Type, &r.Title, &r.Context, &r.Content, &anti); err != nil {
			return nil, err
		}
		r.AntiPatterns = splitAntiPatterns(anti)
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func splitAntiPatterns(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	parts := strings.Split(raw, "\n")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
