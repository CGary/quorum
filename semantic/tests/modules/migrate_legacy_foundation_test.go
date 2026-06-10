package modules

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/hsme/core/src/storage/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

func TestMigrateLegacyFoundation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "hsme-migrate-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	dbPath := filepath.Join(tempDir, "test-hsme.db")

	// 1. Initial creation (schema should include project)
	db, err := sqlite.InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to init DB: %v", err)
	}

	// Verify column exists
	var columnExists bool
	err = db.QueryRow("SELECT count(*) FROM pragma_table_info('memories') WHERE name='project'").Scan(&columnExists)
	if err != nil {
		t.Fatalf("failed to check column: %v", err)
	}
	if !columnExists {
		t.Error("expected project column to exist in new DB")
	}
	db.Close()

	// 2. Idempotency check (rerun InitDB)
	db, err = sqlite.InitDB(dbPath)
	if err != nil {
		t.Fatalf("failed to re-init DB: %v", err)
	}
	db.Close()

	// 3. Migration check (create DB without project column first)
	dbPathMigrate := filepath.Join(tempDir, "test-migrate.db")
	oldDB, err := sql.Open("sqlite3", dbPathMigrate)
	if err != nil {
		t.Fatalf("failed to open old DB: %v", err)
	}
	_, err = oldDB.Exec("CREATE TABLE memories (id INTEGER PRIMARY KEY, raw_content TEXT, content_hash TEXT, source_type TEXT, created_at DATETIME, updated_at DATETIME, superseded_by INTEGER, status TEXT)")
	if err != nil {
		t.Fatalf("failed to create old table: %v", err)
	}
	oldDB.Close()

	// Now run InitDB which should migrate it
	db, err = sqlite.InitDB(dbPathMigrate)
	if err != nil {
		t.Fatalf("failed to migrate DB: %v", err)
	}
	defer db.Close()

	err = db.QueryRow("SELECT count(*) FROM pragma_table_info('memories') WHERE name='project'").Scan(&columnExists)
	if err != nil {
		t.Fatalf("failed to check column after migration: %v", err)
	}
	if !columnExists {
		t.Error("expected project column to be added via migration")
	}
}
