package core

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
)

func seedSearchMemory(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO projects (id, name) VALUES (?, ?), (?, ?)`, "quorum", "Quorum", "other", "Other")
	if err != nil {
		t.Fatalf("insert projects failed: %v", err)
	}

	entries := []struct {
		id, projectID, typ, title, context, content string
	}{
		{"DEC-2026-05-31-1", "quorum", "decision", "Schema validation path", "Artifact validation", "Use JSON schema before saving artifacts."},
		{"PAT-2026-05-31-1", "quorum", "pattern", "SQLite memory search", "Central memory", "Search curated memory entries with deterministic SQL."},
		{"LES-2026-05-31-1", "other", "lesson", "Other project lesson", "Remote project", "Keep project scope private by default."},
	}
	for _, e := range entries {
		_, err := db.Exec(`INSERT INTO memory_entries (id, project_id, type, source_task, title, context, content, created_at, content_hash, raw_json)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, e.id, e.projectID, e.typ, "SQL-05", e.title, e.context, e.content, "2026-05-31", e.id+"-hash", "{}")
		if err != nil {
			t.Fatalf("insert memory entry failed: %v", err)
		}
	}
	_, err = db.Exec(`INSERT INTO memory_anti_patterns (project_id, memory_id, ordinal, content) VALUES (?, ?, ?, ?), (?, ?, ?, ?)`,
		"quorum", "PAT-2026-05-31-1", 1, "Do not use embeddings for basic search.",
		"quorum", "PAT-2026-05-31-1", 2, "Do not search every project by default.")
	if err != nil {
		t.Fatalf("insert anti-patterns failed: %v", err)
	}
}

func TestSearchMemoryEntriesFindsTextAndAntiPatterns(t *testing.T) {
	db, err := OpenMemoryDB(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("OpenMemoryDB failed: %v", err)
	}
	defer db.Close()
	seedSearchMemory(t, db)

	results, err := SearchMemoryEntries(db, MemorySearchOptions{Query: "embeddings", ProjectID: "quorum"})
	if err != nil {
		t.Fatalf("SearchMemoryEntries failed: %v", err)
	}
	if len(results) != 1 || results[0].ID != "PAT-2026-05-31-1" {
		t.Fatalf("expected PAT anti-pattern match, got %+v", results)
	}
	if len(results[0].AntiPatterns) != 2 {
		t.Fatalf("expected two anti-patterns, got %+v", results[0].AntiPatterns)
	}
}

func TestSearchMemoryEntriesProjectScopeAndAllProjects(t *testing.T) {
	db, err := OpenMemoryDB(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("OpenMemoryDB failed: %v", err)
	}
	defer db.Close()
	seedSearchMemory(t, db)

	results, err := SearchMemoryEntries(db, MemorySearchOptions{Query: "project", ProjectID: "quorum"})
	if err != nil {
		t.Fatalf("project search failed: %v", err)
	}
	for _, r := range results {
		if r.ProjectID != "quorum" {
			t.Fatalf("default search leaked project %q", r.ProjectID)
		}
	}

	all, err := SearchMemoryEntries(db, MemorySearchOptions{Query: "project", AllProjects: true})
	if err != nil {
		t.Fatalf("all projects search failed: %v", err)
	}
	foundOther := false
	for _, r := range all {
		foundOther = foundOther || r.ProjectID == "other"
	}
	if !foundOther {
		t.Fatalf("expected all-projects search to include other project, got %+v", all)
	}
}

func TestSearchMemoryEntriesValidationAndLimit(t *testing.T) {
	db, err := OpenMemoryDB(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("OpenMemoryDB failed: %v", err)
	}
	defer db.Close()
	seedSearchMemory(t, db)

	cases := []MemorySearchOptions{
		{Query: "", ProjectID: "quorum"},
		{Query: "schema", ProjectID: "quorum", Type: "invalid"},
		{Query: "schema", ProjectID: "quorum", Limit: -1},
		{Query: "schema"},
	}
	for _, tc := range cases {
		if _, err := SearchMemoryEntries(db, tc); err == nil {
			t.Fatalf("expected validation error for %+v", tc)
		}
	}

	results, err := SearchMemoryEntries(db, MemorySearchOptions{Query: "search", ProjectID: "quorum", Type: "pattern", Limit: 1})
	if err != nil {
		t.Fatalf("limited search failed: %v", err)
	}
	if len(results) != 1 || results[0].Type != "pattern" {
		t.Fatalf("expected one pattern result, got %+v", results)
	}
}

func TestSearchMemoryEntriesLikeFallbackWithoutFTS(t *testing.T) {
	db, err := OpenMemoryDB(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatalf("OpenMemoryDB failed: %v", err)
	}
	defer db.Close()
	seedSearchMemory(t, db)
	_, _ = db.Exec("DROP TABLE IF EXISTS memory_fts")

	results, err := SearchMemoryEntries(db, MemorySearchOptions{Query: "schema", ProjectID: "quorum"})
	if err != nil {
		t.Fatalf("fallback search failed: %v", err)
	}
	if len(results) != 1 || !strings.Contains(results[0].Title, "Schema") {
		t.Fatalf("expected LIKE fallback schema result, got %+v", results)
	}
}
