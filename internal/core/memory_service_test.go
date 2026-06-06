package core

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validMemoryJSON(id string) []byte {
	payload := map[string]any{
		"id":            id,
		"source_task":   "SQL-02",
		"type":          "lesson",
		"title":         "Memory CLI behavior",
		"context":       "While implementing SQL-02.",
		"content":       "The memory CLI saves validated entries through SQLite and reports deterministic JSON output.",
		"related":       []string{"SQL-01", "external-note"},
		"anti_patterns": []string{"Do not overwrite conflicting memory entries."},
		"created_at":    "2026-05-31",
		"supersedes":    "LES-2026-05-30-100000001",
	}
	b, _ := json.Marshal(payload)
	return b
}

func setupMemoryServiceTest(t *testing.T) (string, *QuorumConfig) {
	t.Helper()
	useSchemas(t)
	root := initGitRepo(t)
	chdir(t, root)
	config := &QuorumConfig{ProjectID: "quorum", ProjectName: "Quorum"}
	if err := WriteQuorumConfigTo(config, root); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUORUM_MEMORY_DB", filepath.Join(root, "tmp", "memory.db"))
	return root, config
}

func TestSaveMemoryEntryPersistsEntryAndSatellites(t *testing.T) {
	_, config := setupMemoryServiceTest(t)
	result, err := SaveMemoryEntry(validMemoryJSON("LES-2026-05-31-100000001"))
	if err != nil {
		t.Fatalf("SaveMemoryEntry failed: %v", err)
	}
	if result.ProjectID != config.ProjectID || result.MemoryID != "LES-2026-05-31-100000001" || result.Status != "inserted" || result.ContentHash == "" {
		t.Fatalf("unexpected result: %#v", result)
	}

	db, err := OpenMemoryDB("")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var title string
	if err := db.QueryRow(`SELECT title FROM memory_entries WHERE project_id = ? AND id = ?`, "quorum", "LES-2026-05-31-100000001").Scan(&title); err != nil {
		t.Fatal(err)
	}
	if title != "Memory CLI behavior" {
		t.Fatalf("title = %q", title)
	}

	assertCount(t, db, `SELECT COUNT(*) FROM memory_related WHERE project_id = ? AND memory_id = ?`, 2, "quorum", "LES-2026-05-31-100000001")
	assertCount(t, db, `SELECT COUNT(*) FROM memory_anti_patterns WHERE project_id = ? AND memory_id = ?`, 1, "quorum", "LES-2026-05-31-100000001")
	assertCount(t, db, `SELECT COUNT(*) FROM memory_supersession_edges WHERE project_id = ? AND from_id = ? AND to_id = ?`, 1, "quorum", "LES-2026-05-31-100000001", "LES-2026-05-30-100000001")
}

func TestSaveMemoryEntryIdempotencyAndConflict(t *testing.T) {
	setupMemoryServiceTest(t)
	first, err := SaveMemoryEntry(validMemoryJSON("LES-2026-05-31-200000002"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := SaveMemoryEntry(validMemoryJSON("LES-2026-05-31-200000002"))
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != "unchanged" || second.ContentHash != first.ContentHash {
		t.Fatalf("unexpected idempotent result: %#v", second)
	}

	changed := strings.Replace(string(validMemoryJSON("LES-2026-05-31-200000002")), "deterministic JSON output", "different deterministic JSON output", 1)
	_, err = SaveMemoryEntry([]byte(changed))
	if err == nil || !strings.Contains(err.Error(), "existing content_hash differs") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestSaveMemoryEntryValidationFailures(t *testing.T) {
	setupMemoryServiceTest(t)
	if _, err := SaveMemoryEntry([]byte(`{"id":`)); err == nil || !strings.Contains(err.Error(), "invalid memory JSON") {
		t.Fatalf("expected invalid JSON error, got %v", err)
	}
	if _, err := SaveMemoryEntry([]byte(`{"id":"LES-2026-05-31-300000003"}`)); err == nil || !strings.Contains(err.Error(), "required property") {
		t.Fatalf("expected schema error, got %v", err)
	}
}

func TestSaveMemoryEntryRejectsMissingQuorumConfig(t *testing.T) {
	useSchemas(t)
	root := initGitRepo(t)
	chdir(t, root)
	t.Setenv("QUORUM_MEMORY_DB", filepath.Join(root, "memory.db"))
	_, err := SaveMemoryEntry(validMemoryJSON("LES-2026-05-31-400000004"))
	if err == nil || !strings.Contains(err.Error(), "run quorum init") {
		t.Fatalf("expected .quorumrc error, got %v", err)
	}
}

func TestMemoryStatusWithAndWithoutRegisteredProject(t *testing.T) {
	root, _ := setupMemoryServiceTest(t)
	status, err := MemoryStatus()
	if err != nil {
		t.Fatal(err)
	}
	if status.DBPath != filepath.Join(root, "tmp", "memory.db") || status.ProjectID != "quorum" || status.ProjectName != "Quorum" {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status.ProjectRegistered {
		t.Fatalf("project should not be registered before save: %#v", status)
	}

	if _, err := SaveMemoryEntry(validMemoryJSON("LES-2026-05-31-500000005")); err != nil {
		t.Fatal(err)
	}
	status, err = MemoryStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.ProjectRegistered || status.Counts.Lesson != 1 || status.Counts.Pattern != 0 || status.Counts.Decision != 0 {
		t.Fatalf("unexpected registered status: %#v", status)
	}
}

func TestSaveMemoryEntryAllowsAbsentOptionalArraysAndSupersedesTarget(t *testing.T) {
	setupMemoryServiceTest(t)
	payload := map[string]any{
		"id":          "DEC-2026-05-31-100000001",
		"source_task": "SQL-02",
		"type":        "decision",
		"title":       "Minimal save command",
		"context":     "While implementing SQL-02.",
		"content":     "The save command accepts optional relationships without requiring target records to exist.",
		"created_at":  "2026-05-31",
		"supersedes":  "DEC-2026-01-01-100000001",
	}
	raw, _ := json.Marshal(payload)
	if _, err := SaveMemoryEntry(raw); err != nil {
		t.Fatalf("SaveMemoryEntry failed: %v", err)
	}
}

func TestMemoryStatusRejectsInvalidQuorumConfig(t *testing.T) {
	root := initGitRepo(t)
	chdir(t, root)
	if err := os.WriteFile(filepath.Join(root, ".quorumrc"), []byte(`{"project_id":"Bad ID","project_name":"Quorum"}`), 0644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("QUORUM_MEMORY_DB", filepath.Join(root, "memory.db"))
	_, err := MemoryStatus()
	if err == nil || !strings.Contains(err.Error(), "project_id must be slug-like") {
		t.Fatalf("expected invalid .quorumrc error, got %v", err)
	}
}

func assertCount(t *testing.T, db *sql.DB, query string, want int, args ...any) {
	t.Helper()
	var got int
	if err := db.QueryRow(query, args...).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("count for %q = %d, want %d", query, got, want)
	}
}

func TestSaveMemoryEntrySessionSentinelIdempotency(t *testing.T) {
	setupMemoryServiceTest(t)
	payload := validMemoryJSON("LES-2026-06-06-300000003")
	
	var m map[string]any
	json.Unmarshal(payload, &m)
	m["source_task"] = "SESSION-2026-06-06"
	newPayload, _ := json.Marshal(m)

	first, err := SaveMemoryEntry(newPayload)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "inserted" {
		t.Fatalf("expected inserted, got %s", first.Status)
	}

	second, err := SaveMemoryEntry(newPayload)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != "unchanged" {
		t.Fatalf("expected unchanged, got %s", second.Status)
	}

	db, err := OpenMemoryDB("")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	
	var sourceTask string
	if err := db.QueryRow(`SELECT source_task FROM memory_entries WHERE project_id = ? AND id = ?`, "quorum", "LES-2026-06-06-300000003").Scan(&sourceTask); err != nil {
		t.Fatal(err)
	}
	if sourceTask != "SESSION-2026-06-06" {
		t.Fatalf("expected source_task = SESSION-2026-06-06, got %q", sourceTask)
	}
}
