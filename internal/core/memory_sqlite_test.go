package core

import (
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestMemoryDBPath(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "quorum_db_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "custom", "memory.db")
	os.Setenv("QUORUM_MEMORY_DB", dbPath)
	defer os.Unsetenv("QUORUM_MEMORY_DB")

	resolvedPath, err := MemoryDBPath()
	if err != nil {
		t.Fatalf("MemoryDBPath failed: %v", err)
	}

	if resolvedPath != dbPath {
		t.Errorf("Expected %s, got %s", dbPath, resolvedPath)
	}

	if _, err := os.Stat(filepath.Dir(dbPath)); os.IsNotExist(err) {
		t.Errorf("Directory %s was not created", filepath.Dir(dbPath))
	}
}

func TestOpenMemoryDBAndSchema(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "quorum_db_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "memory.db")
	db, err := OpenMemoryDB(dbPath)
	if err != nil {
		t.Fatalf("OpenMemoryDB failed: %v", err)
	}
	defer db.Close()

	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode;").Scan(&journalMode)
	if err != nil || (journalMode != "wal" && journalMode != "WAL") {
		t.Errorf("Expected journal_mode WAL, got %s (err: %v)", journalMode, err)
	}

	var foreignKeys int
	err = db.QueryRow("PRAGMA foreign_keys;").Scan(&foreignKeys)
	if err != nil || foreignKeys != 1 {
		t.Errorf("Expected foreign_keys ON (1), got %d (err: %v)", foreignKeys, err)
	}

	tables := []string{"projects", "memory_entries", "memory_related", "memory_anti_patterns", "memory_supersession_edges"}
	for _, table := range tables {
		var name string
		err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?;", table).Scan(&name)
		if err != nil {
			t.Errorf("Table %s not found: %v", table, err)
		}
	}

	columns := map[string]bool{}
	rows, err := db.Query("PRAGMA table_info(memory_entries);")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			t.Fatal(err)
		}
		columns[name] = true
	}
	for _, required := range []string{"project_id", "id", "source_task", "title", "context", "content_hash", "raw_json"} {
		if !columns[required] {
			t.Fatalf("memory_entries missing column %s", required)
		}
	}
}

func TestCanonicalMemoryHash(t *testing.T) {
	json1 := map[string]interface{}{"a": 1, "b": "test"}
	json2 := map[string]interface{}{"b": "test", "a": 1}

	hash1, err1 := CanonicalMemoryHash(json1)
	hash2, err2 := CanonicalMemoryHash(json2)

	if err1 != nil || err2 != nil {
		t.Fatalf("CanonicalMemoryHash failed: %v, %v", err1, err2)
	}

	if hash1 != hash2 {
		t.Errorf("Hashes do not match for functionally identical JSON: %s != %s", hash1, hash2)
	}
}

func TestConcurrentWrites(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "quorum_db_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "memory.db")
	db, err := OpenMemoryDB(dbPath)
	if err != nil {
		t.Fatalf("OpenMemoryDB failed: %v", err)
	}
	defer db.Close()

	_, err = db.Exec("INSERT INTO projects (id, name) VALUES (?, ?)", "quorum", "Quorum")
	if err != nil {
		t.Fatalf("project insert failed: %v", err)
	}

	var wg sync.WaitGroup
	numWorkers := 10
	wg.Add(numWorkers)

	for i := 0; i < numWorkers; i++ {
		go func(id int) {
			defer wg.Done()
			_, err := db.Exec(`INSERT INTO memory_entries
(project_id, id, type, source_task, title, context, content, created_at, content_hash, raw_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "quorum", filepath.Base(string(rune('a'+id))), "lesson", "SQL-02", "Concurrent write", "Context", "Content with enough length", "2026-05-31", "hash", "{}")
			if err != nil {
				t.Errorf("Concurrent write failed: %v", err)
			}
		}(i)
	}

	wg.Wait()
}
