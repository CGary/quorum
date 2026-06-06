package core

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"
)

const (
	DefaultMemoryListLimit = 50
	MaxMemoryListLimit     = 100
	memoryExcerptLength    = 240
)

var memoryIDPattern = regexp.MustCompile(`^(PAT|DEC|LES)-[0-9]{4}-[0-9]{2}-[0-9]{2}-([0-9]{9}|[0-9]+)$`)

type MemoryListOptions struct {
	ProjectID string
	Type      string
	Query     string
	Limit     int
	Offset    int
}

type MemoryListResponse struct {
	ProjectID string           `json:"project_id"`
	Counts    MemoryTypeCounts `json:"counts"`
	Items     []MemoryListItem `json:"items"`
}

type MemoryListItem struct {
	ID               string `json:"id"`
	Type             string `json:"type"`
	Title            string `json:"title"`
	SourceTask       string `json:"source_task"`
	Context          string `json:"context"`
	ContentExcerpt   string `json:"content_excerpt"`
	CreatedAt        string `json:"created_at"`
	Supersedes       string `json:"supersedes"`
	RelatedCount     int    `json:"related_count"`
	AntiPatternCount int    `json:"anti_pattern_count"`
}

type MemoryDetail struct {
	ProjectID    string   `json:"project_id"`
	ID           string   `json:"id"`
	Type         string   `json:"type"`
	Title        string   `json:"title"`
	SourceTask   string   `json:"source_task"`
	Context      string   `json:"context"`
	Content      string   `json:"content"`
	CreatedAt    string   `json:"created_at"`
	Supersedes   string   `json:"supersedes"`
	Related      []string `json:"related"`
	AntiPatterns []string `json:"anti_patterns"`
	SupersededBy []string `json:"superseded_by"`
}

func NormalizeMemoryListOptions(opts MemoryListOptions) (MemoryListOptions, error) {
	opts.ProjectID = strings.TrimSpace(opts.ProjectID)
	if opts.ProjectID == "" {
		return opts, fmt.Errorf("project_id is required")
	}
	opts.Type = strings.TrimSpace(opts.Type)
	if opts.Type != "" {
		if err := ValidateMemoryType(opts.Type); err != nil {
			return opts, err
		}
	}
	opts.Query = strings.TrimSpace(opts.Query)
	if opts.Limit == 0 {
		opts.Limit = DefaultMemoryListLimit
	}
	if opts.Limit < 1 {
		return opts, fmt.Errorf("limit must be greater than 0")
	}
	if opts.Limit > MaxMemoryListLimit {
		return opts, fmt.Errorf("limit must be less than or equal to %d", MaxMemoryListLimit)
	}
	if opts.Offset < 0 {
		return opts, fmt.Errorf("offset must be greater than or equal to 0")
	}
	return opts, nil
}

func ValidateMemoryType(typ string) error {
	switch typ {
	case "decision", "pattern", "lesson":
		return nil
	default:
		return fmt.Errorf("invalid memory type %q", typ)
	}
}

func ValidateMemoryID(memoryID string) error {
	memoryID = strings.TrimSpace(memoryID)
	if memoryID == "" {
		return fmt.Errorf("memory_id is required")
	}
	if !memoryIDPattern.MatchString(memoryID) {
		return fmt.Errorf("invalid memory_id %q", memoryID)
	}
	return nil
}

func ListProjectMemories(db *sql.DB, opts MemoryListOptions) (MemoryListResponse, error) {
	opts, err := NormalizeMemoryListOptions(opts)
	if err != nil {
		return MemoryListResponse{}, err
	}
	counts, err := CountProjectMemories(db, opts.ProjectID)
	if err != nil {
		return MemoryListResponse{}, err
	}

	where := []string{"e.project_id = ?"}
	args := []any{opts.ProjectID}
	if opts.Type != "" {
		where = append(where, "e.type = ?")
		args = append(args, opts.Type)
	}
	if opts.Query != "" {
		pattern := "%" + strings.ToLower(opts.Query) + "%"
		where = append(where, `(
			LOWER(COALESCE(e.title, '')) LIKE ? OR
			LOWER(COALESCE(e.context, '')) LIKE ? OR
			LOWER(COALESCE(e.content, '')) LIKE ? OR
			LOWER(COALESCE(e.source_task, '')) LIKE ? OR
			LOWER(COALESCE((SELECT group_concat(ap.content, ' ') FROM memory_anti_patterns ap WHERE ap.project_id = e.project_id AND ap.memory_id = e.id), '')) LIKE ?
		)`)
		args = append(args, pattern, pattern, pattern, pattern, pattern)
	}
	stmtArgs := []any{memoryExcerptLength, memoryExcerptLength}
	stmtArgs = append(stmtArgs, args...)
	stmtArgs = append(stmtArgs, opts.Limit, opts.Offset)

	stmt := `SELECT e.id, e.type, COALESCE(e.title, ''), COALESCE(e.source_task, ''), COALESCE(e.context, ''),
		CASE WHEN length(COALESCE(e.content, '')) > ? THEN substr(e.content, 1, ?) || '…' ELSE COALESCE(e.content, '') END,
		COALESCE(e.created_at, ''), COALESCE(e.supersedes, ''),
		(SELECT COUNT(*) FROM memory_related r WHERE r.project_id = e.project_id AND r.memory_id = e.id),
		(SELECT COUNT(*) FROM memory_anti_patterns ap WHERE ap.project_id = e.project_id AND ap.memory_id = e.id)
		FROM memory_entries e
		WHERE ` + strings.Join(where, " AND ") + `
		ORDER BY e.created_at DESC, e.id ASC
		LIMIT ? OFFSET ?`

	rows, err := db.Query(stmt, stmtArgs...)
	if err != nil {
		return MemoryListResponse{}, err
	}
	defer rows.Close()

	items := []MemoryListItem{}
	for rows.Next() {
		var item MemoryListItem
		if err := rows.Scan(&item.ID, &item.Type, &item.Title, &item.SourceTask, &item.Context, &item.ContentExcerpt, &item.CreatedAt, &item.Supersedes, &item.RelatedCount, &item.AntiPatternCount); err != nil {
			return MemoryListResponse{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return MemoryListResponse{}, err
	}
	return MemoryListResponse{ProjectID: opts.ProjectID, Counts: counts, Items: items}, nil
}

func CountProjectMemories(db *sql.DB, projectID string) (MemoryTypeCounts, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return MemoryTypeCounts{}, fmt.Errorf("project_id is required")
	}
	rows, err := db.Query(`SELECT type, COUNT(*) FROM memory_entries WHERE project_id = ? GROUP BY type`, projectID)
	if err != nil {
		return MemoryTypeCounts{}, err
	}
	defer rows.Close()
	var counts MemoryTypeCounts
	for rows.Next() {
		var typ string
		var count int
		if err := rows.Scan(&typ, &count); err != nil {
			return counts, err
		}
		switch typ {
		case "pattern":
			counts.Pattern = count
		case "decision":
			counts.Decision = count
		case "lesson":
			counts.Lesson = count
		}
	}
	return counts, rows.Err()
}

func GetProjectMemory(db *sql.DB, projectID, memoryID string) (MemoryDetail, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return MemoryDetail{}, fmt.Errorf("project_id is required")
	}
	memoryID = strings.TrimSpace(memoryID)
	if err := ValidateMemoryID(memoryID); err != nil {
		return MemoryDetail{}, err
	}

	var detail MemoryDetail
	detail.ProjectID = projectID
	err := db.QueryRow(`SELECT project_id, id, type, COALESCE(title, ''), COALESCE(source_task, ''), COALESCE(context, ''), COALESCE(content, ''), COALESCE(created_at, ''), COALESCE(supersedes, '')
		FROM memory_entries WHERE project_id = ? AND id = ?`, projectID, memoryID).Scan(&detail.ProjectID, &detail.ID, &detail.Type, &detail.Title, &detail.SourceTask, &detail.Context, &detail.Content, &detail.CreatedAt, &detail.Supersedes)
	if err != nil {
		return MemoryDetail{}, err
	}
	if detail.Related, err = queryStringList(db, `SELECT related_ref FROM memory_related WHERE project_id = ? AND memory_id = ? ORDER BY related_ref ASC`, projectID, memoryID); err != nil {
		return MemoryDetail{}, err
	}
	if detail.AntiPatterns, err = queryStringList(db, `SELECT content FROM memory_anti_patterns WHERE project_id = ? AND memory_id = ? ORDER BY ordinal ASC`, projectID, memoryID); err != nil {
		return MemoryDetail{}, err
	}
	if detail.SupersededBy, err = queryStringList(db, `SELECT from_id FROM memory_supersession_edges WHERE project_id = ? AND to_id = ? ORDER BY from_id ASC`, projectID, memoryID); err != nil {
		return MemoryDetail{}, err
	}
	return detail, nil
}

func queryStringList(db *sql.DB, stmt string, args ...any) ([]string, error) {
	rows, err := db.Query(stmt, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		out = append(out, value)
	}
	return out, rows.Err()
}
