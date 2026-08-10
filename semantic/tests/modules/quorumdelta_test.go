package modules

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hsme/core/src/core/quorumdelta"
	"github.com/hsme/core/src/storage/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

func setupTestQuorumDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("failed to create test quorum DB: %v", err)
	}

	schema := `
	CREATE TABLE projects (
		id TEXT PRIMARY KEY
	);
	INSERT INTO projects (id) VALUES ('quorum');

	CREATE TABLE memory_entries (
		project_id TEXT NOT NULL,
		id TEXT NOT NULL,
		type TEXT NOT NULL CHECK (type IN ('pattern', 'decision', 'lesson')),
		source_task TEXT NOT NULL,
		title TEXT NOT NULL,
		context TEXT NOT NULL,
		content TEXT NOT NULL,
		created_at TEXT NOT NULL,
		supersedes TEXT,
		source_path TEXT,
		content_hash TEXT NOT NULL,
		imported_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
		raw_json TEXT NOT NULL,
		PRIMARY KEY (project_id, id),
		FOREIGN KEY (project_id) REFERENCES projects(id)
	);
	`

	if _, err := db.Exec(schema); err != nil {
		db.Close()
		t.Fatalf("failed to initialize quorum DB schema: %v", err)
	}

	return db
}

func TestOpenReadOnly(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "quorum_memory.db")

	// Initialize database
	qDB := setupTestQuorumDB(t, dbPath)
	qDB.Close()

	// Open read-only
	roDB, err := quorumdelta.OpenReadOnly(dbPath)
	if err != nil {
		t.Fatalf("OpenReadOnly failed: %v", err)
	}
	defer roDB.Close()

	// Verify reads work
	var count int
	err = roDB.QueryRow("SELECT COUNT(*) FROM memory_entries").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to read from read-only DB: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 rows, got %d", count)
	}

	// Verify write fails
	_, err = roDB.Exec("INSERT INTO projects (id) VALUES ('fail_proj')")
	if err == nil {
		t.Fatal("Expected error when writing to read-only DB, but write succeeded")
	}
	if !strings.Contains(err.Error(), "read-only") && !strings.Contains(err.Error(), "readonly") {
		t.Errorf("Expected read-only error message, got: %v", err)
	}
}

func TestImportProjectValidation(t *testing.T) {
	tempDir := t.TempDir()
	qPath := filepath.Join(tempDir, "quorum.db")
	hPath := filepath.Join(tempDir, "hsme.db")

	qDB := setupTestQuorumDB(t, qPath)
	defer qDB.Close()

	hDB, err := sqlite.InitDB(hPath)
	if err != nil {
		t.Fatalf("failed to init HSME DB: %v", err)
	}
	defer hDB.Close()

	// Seed one entry
	_, err = qDB.Exec(`INSERT INTO memory_entries 
		(project_id, id, type, source_task, title, context, content, created_at, content_hash, raw_json)
		VALUES ('quorum', 'PAT-2026-01-01-1', 'pattern', 'TASK-1', 'Test Title', 'Test Context', 'Test Content 12345678901234567890', '2026-01-01T10:00:00Z', 'hash1', '{}')`)
	if err != nil {
		t.Fatalf("failed to seed quorum entry: %v", err)
	}

	// Import with empty hsmeProject must fail before writing
	_, err = quorumdelta.Import(qDB, hDB, "quorum", "")
	if err == nil {
		t.Fatal("Expected error when hsmeProject is empty, got nil")
	}

	var count int
	err = hDB.QueryRow("SELECT COUNT(*) FROM memories").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query memories table: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 memories written when hsmeProject is empty, got %d", count)
	}
}

func TestImportIdempotency(t *testing.T) {
	tempDir := t.TempDir()
	qPath := filepath.Join(tempDir, "quorum.db")
	hPath := filepath.Join(tempDir, "hsme.db")

	qDB := setupTestQuorumDB(t, qPath)
	defer qDB.Close()

	hDB, err := sqlite.InitDB(hPath)
	if err != nil {
		t.Fatalf("failed to init HSME DB: %v", err)
	}
	defer hDB.Close()

	// Seed entries
	_, err = qDB.Exec(`INSERT INTO memory_entries 
		(project_id, id, type, source_task, title, context, content, created_at, content_hash, raw_json)
		VALUES 
		('quorum', 'PAT-2026-01-01-1', 'pattern', 'TASK-1', 'Title 1', 'Context 1', 'Content 1 long enough for validation', '2026-01-01T10:00:00Z', 'hash1', '{}'),
		('quorum', 'DEC-2026-01-01-2', 'decision', 'TASK-1', 'Title 2', 'Context 2', 'Content 2 long enough for validation', '2026-01-01T11:00:00Z', 'hash2', '{}')`)
	if err != nil {
		t.Fatalf("failed to seed quorum entries: %v", err)
	}

	// First run
	res1, err := quorumdelta.Import(qDB, hDB, "quorum", "hsme-quorum")
	if err != nil {
		t.Fatalf("First Import failed: %v", err)
	}
	if res1.Ingested != 2 || res1.Skipped != 0 {
		t.Errorf("First Import expected Ingested=2, Skipped=0; got Ingested=%d, Skipped=%d", res1.Ingested, res1.Skipped)
	}

	// Second run against unchanged source
	res2, err := quorumdelta.Import(qDB, hDB, "quorum", "hsme-quorum")
	if err != nil {
		t.Fatalf("Second Import failed: %v", err)
	}
	if res2.Ingested != 0 || res2.Skipped != 2 {
		t.Errorf("Second Import expected Ingested=0, Skipped=2; got Ingested=%d, Skipped=%d", res2.Ingested, res2.Skipped)
	}

	// Verify row counts and explicit project / source_type in HSME memories
	rows, err := hDB.Query("SELECT source_type, project FROM memories")
	if err != nil {
		t.Fatalf("Failed to query memories: %v", err)
	}
	defer rows.Close()

	memCount := 0
	for rows.Next() {
		var sourceType, project string
		if err := rows.Scan(&sourceType, &project); err != nil {
			t.Fatalf("Scan failed: %v", err)
		}
		memCount++
		if sourceType != "quorum_curated_memory" {
			t.Errorf("Expected source_type 'quorum_curated_memory', got %s", sourceType)
		}
		if project != "hsme-quorum" {
			t.Errorf("Expected project 'hsme-quorum', got %s", project)
		}
	}
	if memCount != 2 {
		t.Errorf("Expected 2 total memories in HSME, got %d", memCount)
	}
}

func TestImportSupersedesTranslation(t *testing.T) {
	tempDir := t.TempDir()
	qPath := filepath.Join(tempDir, "quorum.db")
	hPath := filepath.Join(tempDir, "hsme.db")

	qDB := setupTestQuorumDB(t, qPath)
	defer qDB.Close()

	hDB, err := sqlite.InitDB(hPath)
	if err != nil {
		t.Fatalf("failed to init HSME DB: %v", err)
	}
	defer hDB.Close()

	// Seed entry 1 and entry 2 (supersedes entry 1)
	_, err = qDB.Exec(`INSERT INTO memory_entries 
		(project_id, id, type, source_task, title, context, content, created_at, supersedes, content_hash, raw_json)
		VALUES 
		('quorum', 'PAT-2026-01-01-1', 'pattern', 'TASK-1', 'Old Pattern Title', 'Old Context', 'Old Content long enough for validation', '2026-01-01T10:00:00Z', NULL, 'hash1', '{}'),
		('quorum', 'LES-2026-01-02-1', 'lesson', 'TASK-2', 'New Lesson Title', 'New Context', 'New Content long enough for validation', '2026-01-02T10:00:00Z', 'PAT-2026-01-01-1', 'hash2', '{}')`)
	if err != nil {
		t.Fatalf("failed to seed quorum entries with supersedes: %v", err)
	}

	res, err := quorumdelta.Import(qDB, hDB, "quorum", "hsme-proj")
	if err != nil {
		t.Fatalf("Import failed: %v", err)
	}
	if res.Ingested != 2 {
		t.Fatalf("Expected 2 ingested entries, got %d", res.Ingested)
	}

	// Verify old memory is superseded and linked to new memory
	var oldID int64
	oldIDPtr, err := quorumdelta.LookupHSMEID(hDB, "hsme-proj", "PAT-2026-01-01-1")
	if err != nil || oldIDPtr == nil {
		t.Fatalf("Failed to lookup old memory HSME ID: %v", err)
	}
	oldID = *oldIDPtr

	var newID int64
	newIDPtr, err := quorumdelta.LookupHSMEID(hDB, "hsme-proj", "LES-2026-01-02-1")
	if err != nil || newIDPtr == nil {
		t.Fatalf("Failed to lookup new memory HSME ID: %v", err)
	}
	newID = *newIDPtr

	var oldStatus string
	var supersededBy sql.NullInt64
	err = hDB.QueryRow("SELECT status, superseded_by FROM memories WHERE id = ?", oldID).Scan(&oldStatus, &supersededBy)
	if err != nil {
		t.Fatalf("Failed to query old memory status: %v", err)
	}
	if oldStatus != "superseded" {
		t.Errorf("Expected old memory status 'superseded', got '%s'", oldStatus)
	}
	if !supersededBy.Valid || supersededBy.Int64 != newID {
		t.Errorf("Expected old memory superseded_by=%d, got %v", newID, supersededBy)
	}

	var newStatus string
	err = hDB.QueryRow("SELECT status FROM memories WHERE id = ?", newID).Scan(&newStatus)
	if err != nil {
		t.Fatalf("Failed to query new memory status: %v", err)
	}
	if newStatus != "active" {
		t.Errorf("Expected new memory status 'active', got '%s'", newStatus)
	}
}

func TestImportDanglingSupersedes(t *testing.T) {
	tempDir := t.TempDir()
	qPath := filepath.Join(tempDir, "quorum.db")
	hPath := filepath.Join(tempDir, "hsme.db")

	qDB := setupTestQuorumDB(t, qPath)
	defer qDB.Close()

	hDB, err := sqlite.InitDB(hPath)
	if err != nil {
		t.Fatalf("failed to init HSME DB: %v", err)
	}
	defer hDB.Close()

	// Entry pointing to non-existent supersedes ID
	_, err = qDB.Exec(`INSERT INTO memory_entries 
		(project_id, id, type, source_task, title, context, content, created_at, supersedes, content_hash, raw_json)
		VALUES 
		('quorum', 'LES-2026-01-01-1', 'lesson', 'TASK-1', 'Title', 'Context', 'Content long enough for validation', '2026-01-01T10:00:00Z', 'NONEXISTENT-999', 'hash1', '{}')`)
	if err != nil {
		t.Fatalf("failed to seed entry with dangling supersedes: %v", err)
	}

	res, err := quorumdelta.Import(qDB, hDB, "quorum", "hsme-proj")
	if err != nil {
		t.Fatalf("Import failed on dangling supersedes: %v", err)
	}
	if res.Ingested != 1 {
		t.Fatalf("Expected 1 ingested entry, got %d", res.Ingested)
	}

	memIDPtr, err := quorumdelta.LookupHSMEID(hDB, "hsme-proj", "LES-2026-01-01-1")
	if err != nil || memIDPtr == nil {
		t.Fatalf("Failed to lookup HSME ID: %v", err)
	}

	var status string
	var supersededBy sql.NullInt64
	err = hDB.QueryRow("SELECT status, superseded_by FROM memories WHERE id = ?", *memIDPtr).Scan(&status, &supersededBy)
	if err != nil {
		t.Fatalf("Failed to query memory: %v", err)
	}
	if status != "active" {
		t.Errorf("Expected status 'active', got '%s'", status)
	}
	if supersededBy.Valid {
		t.Errorf("Expected null superseded_by for dangling target, got %d", supersededBy.Int64)
	}
}
