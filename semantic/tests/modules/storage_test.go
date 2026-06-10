package modules

import (
	"os"
	"testing"

	"github.com/hsme/core/src/storage/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

func TestStorageInitialization(t *testing.T) {
	dbPath := "test_hsme.db"
	defer os.Remove(dbPath)

	db, err := sqlite.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// 1. Check if file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Errorf("Database file was not created")
	}

	// 2. Check WAL mode
	var journalMode string
	err = db.QueryRow("PRAGMA journal_mode;").Scan(&journalMode)
	if err != nil {
		t.Fatalf("Failed to query journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Errorf("Expected journal_mode to be 'wal', got '%s'", journalMode)
	}

	// 3. Check if extensions are loaded (vec0 check)
	var vecVersion string
	err = db.QueryRow("SELECT vec_version();").Scan(&vecVersion)
	if err != nil {
		t.Errorf("vec0 extension not loaded: %v", err)
	}

	// 4. Check tables
	tables := []string{
		"memories",
		"memory_chunks",
		"async_tasks",
		"kg_nodes",
		"kg_edge_evidence",
	}

	for _, table := range tables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name=?;", table).Scan(&name)
		if err != nil {
			t.Errorf("Table %s does not exist", table)
		}
	}

	// FTS and VEC tables might be virtual, so check specifically if they exist in sqlite_master
	virtualTables := []string{
		"memory_chunks_fts",
		"memory_chunks_vec",
	}
	for _, table := range virtualTables {
		var name string
		err := db.QueryRow("SELECT name FROM sqlite_master WHERE name=?;", table).Scan(&name)
		if err != nil {
			t.Errorf("Virtual table %s does not exist", table)
		}
	}
}
