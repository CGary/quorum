package core

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func setupMemoryQueryDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenMemoryDB(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("open memory db: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	_, err = db.Exec(`INSERT INTO projects (id, name, root_path, created_at, updated_at) VALUES
		('proj-a', 'Project A', '/tmp/proj-a', '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z'),
		('proj-b', 'Project B', '/tmp/proj-b', '2026-06-01T00:00:00Z', '2026-06-01T00:00:00Z')`)
	if err != nil {
		t.Fatalf("insert projects: %v", err)
	}
	entries := []struct{ project, id, typ, title, task, context, content, created string }{
		{"proj-a", "DEC-2026-06-01-1", "decision", "Decision memory", "FEAT-001", "Serve viewer", "Allow read-only memory browsing in the local viewer.", "2026-06-03"},
		{"proj-a", "PAT-2026-06-01-1", "pattern", "Pattern memory", "FEAT-002", "SQLite queries", "Use normalized tables for memory list and detail views.", "2026-06-02"},
		{"proj-a", "LES-2026-06-01-1", "lesson", "Lesson memory", "FEAT-003", "Validation", "Reject writes from serve handlers and preserve q-memory ingestion.", "2026-06-01"},
		{"proj-b", "DEC-2026-06-01-1", "decision", "Other project decision", "FEAT-999", "Isolation", "This entry must not leak into proj-a results.", "2026-06-04"},
	}
	for _, e := range entries {
		_, err := db.Exec(`INSERT INTO memory_entries (project_id, id, type, source_task, title, context, content, created_at, supersedes, content_hash, raw_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '{}')`, e.project, e.id, e.typ, e.task, e.title, e.context, e.content, e.created, nil, e.id+"-hash")
		if err != nil {
			t.Fatalf("insert memory %s/%s: %v", e.project, e.id, err)
		}
	}
	_, err = db.Exec(`UPDATE memory_entries SET supersedes = ? WHERE project_id = ? AND id = ?`, "PAT-2026-06-01-1", "proj-a", "LES-2026-06-01-1")
	if err != nil {
		t.Fatalf("update supersedes: %v", err)
	}
	_, err = db.Exec(`INSERT INTO memory_related (project_id, memory_id, related_ref) VALUES
		('proj-a', 'DEC-2026-06-01-1', 'PAT-2026-06-01-1'),
		('proj-a', 'DEC-2026-06-01-1', 'LES-2026-06-01-1')`)
	if err != nil {
		t.Fatalf("insert related: %v", err)
	}
	_, err = db.Exec(`INSERT INTO memory_anti_patterns (project_id, memory_id, ordinal, content) VALUES
		('proj-a', 'DEC-2026-06-01-1', 0, 'Do not write memory from serve handlers.'),
		('proj-a', 'DEC-2026-06-01-1', 1, 'Do not expose lifecycle evidence artifacts.')`)
	if err != nil {
		t.Fatalf("insert anti patterns: %v", err)
	}
	_, err = db.Exec(`INSERT INTO memory_supersession_edges (project_id, from_id, to_id) VALUES
		('proj-a', 'LES-2026-06-01-1', 'PAT-2026-06-01-1')`)
	if err != nil {
		t.Fatalf("insert supersession edge: %v", err)
	}
	return db
}

func TestListProjectMemoriesIsolatedAndOrderedWithCounts(t *testing.T) {
	db := setupMemoryQueryDB(t)
	got, err := ListProjectMemories(db, MemoryListOptions{ProjectID: "proj-a"})
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if got.ProjectID != "proj-a" || got.Counts.Decision != 1 || got.Counts.Pattern != 1 || got.Counts.Lesson != 1 {
		t.Fatalf("unexpected response metadata: %+v", got)
	}
	if len(got.Items) != 3 {
		t.Fatalf("expected 3 isolated items, got %d: %+v", len(got.Items), got.Items)
	}
	if got.Items[0].ID != "DEC-2026-06-01-1" || got.Items[1].ID != "PAT-2026-06-01-1" || got.Items[2].ID != "LES-2026-06-01-1" {
		t.Fatalf("unexpected ordering: %+v", got.Items)
	}
	if got.Items[0].RelatedCount != 2 || got.Items[0].AntiPatternCount != 2 {
		t.Fatalf("expected relation counts on decision item, got %+v", got.Items[0])
	}
}

func TestListProjectMemoriesFiltersTypeAndQueryButCountsStayProjectWide(t *testing.T) {
	db := setupMemoryQueryDB(t)
	got, err := ListProjectMemories(db, MemoryListOptions{ProjectID: "proj-a", Type: "decision", Query: "viewer"})
	if err != nil {
		t.Fatalf("list memories: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].Type != "decision" {
		t.Fatalf("unexpected filtered items: %+v", got.Items)
	}
	if got.Counts.Decision != 1 || got.Counts.Pattern != 1 || got.Counts.Lesson != 1 {
		t.Fatalf("counts should ignore filters, got %+v", got.Counts)
	}
}

func TestListProjectMemoriesEmptyProject(t *testing.T) {
	db := setupMemoryQueryDB(t)
	_, err := db.Exec(`INSERT INTO projects (id, name, root_path) VALUES ('empty', 'Empty', '/tmp/empty')`)
	if err != nil {
		t.Fatalf("insert empty project: %v", err)
	}
	got, err := ListProjectMemories(db, MemoryListOptions{ProjectID: "empty"})
	if err != nil {
		t.Fatalf("list empty memories: %v", err)
	}
	if len(got.Items) != 0 || got.Counts.Decision != 0 || got.Counts.Pattern != 0 || got.Counts.Lesson != 0 {
		t.Fatalf("unexpected empty response: %+v", got)
	}
}

func TestMemoryQueryValidation(t *testing.T) {
	if _, err := NormalizeMemoryListOptions(MemoryListOptions{ProjectID: "proj-a", Type: "bad"}); err == nil {
		t.Fatal("expected invalid type error")
	}
	if _, err := NormalizeMemoryListOptions(MemoryListOptions{ProjectID: "proj-a", Limit: 101}); err == nil {
		t.Fatal("expected invalid limit error")
	}
	if err := ValidateMemoryID("../DEC-2026-06-01-1"); err == nil {
		t.Fatal("expected invalid path-like memory id")
	}
}

func TestGetProjectMemoryFullDetail(t *testing.T) {
	db := setupMemoryQueryDB(t)
	got, err := GetProjectMemory(db, "proj-a", "DEC-2026-06-01-1")
	if err != nil {
		t.Fatalf("get memory detail: %v", err)
	}
	if got.ProjectID != "proj-a" || got.ID != "DEC-2026-06-01-1" || got.Type != "decision" {
		t.Fatalf("unexpected detail identity: %+v", got)
	}
	if len(got.Related) != 2 || len(got.AntiPatterns) != 2 {
		t.Fatalf("expected related and anti patterns, got %+v", got)
	}
	if len(got.SupersededBy) != 0 {
		t.Fatalf("decision should not be superseded, got %+v", got.SupersededBy)
	}

	pattern, err := GetProjectMemory(db, "proj-a", "PAT-2026-06-01-1")
	if err != nil {
		t.Fatalf("get superseded memory: %v", err)
	}
	if len(pattern.SupersededBy) != 1 || pattern.SupersededBy[0] != "LES-2026-06-01-1" {
		t.Fatalf("expected superseded_by edge, got %+v", pattern.SupersededBy)
	}

	if _, err := GetProjectMemory(db, "proj-a", "DEC-2026-06-01-404"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected sql.ErrNoRows for missing memory, got %v", err)
	}
}
