package modules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hsme/core/src/core/capsule"
	"github.com/hsme/core/src/storage/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

func TestImportValidation(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "hsme.db")
	hDB, err := sqlite.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init HSME DB: %v", err)
	}
	defer hDB.Close()

	tasksRoot := filepath.Join("testdata", "capsule_scan")

	// Empty project
	_, err = capsule.Import(hDB, tasksRoot, "")
	if err == nil {
		t.Fatal("Expected error for empty project, got nil")
	}

	// Whitespace project
	_, err = capsule.Import(hDB, tasksRoot, "   ")
	if err == nil {
		t.Fatal("Expected error for whitespace project, got nil")
	}

	// Empty tasksRoot
	_, err = capsule.Import(hDB, "", "test-project")
	if err == nil {
		t.Fatal("Expected error for empty tasksRoot, got nil")
	}

	// Nil DB
	_, err = capsule.Import(nil, tasksRoot, "test-project")
	if err == nil {
		t.Fatal("Expected error for nil db, got nil")
	}

	// Verify no memories written
	var count int
	err = hDB.QueryRow("SELECT COUNT(*) FROM memories").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query memories table: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 memories written on validation failures, got %d", count)
	}
}

func TestImport_IdempotencyAndExplicitFields(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "hsme.db")
	hDB, err := sqlite.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init HSME DB: %v", err)
	}
	defer hDB.Close()

	tasksRoot := filepath.Join("testdata", "capsule_scan")
	projectName := "test-capsule-project"

	// First Import run
	res1, err := capsule.Import(hDB, tasksRoot, projectName)
	if err != nil {
		t.Fatalf("First Import failed: %v", err)
	}

	if res1.Scanned != 4 {
		t.Errorf("Expected Scanned=4, got %d", res1.Scanned)
	}
	if res1.Ingested != 4 {
		t.Errorf("Expected Ingested=4 on first run, got %d", res1.Ingested)
	}
	if res1.Skipped != 0 {
		t.Errorf("Expected Skipped=0 on first run, got %d", res1.Skipped)
	}

	// AC-5: Verify source_type='note' and explicit project set on every row
	rows, err := hDB.Query("SELECT source_type, project, superseded_by FROM memories")
	if err != nil {
		t.Fatalf("Failed to query memories: %v", err)
	}
	defer rows.Close()

	rowCount := 0
	for rows.Next() {
		var sourceType, proj string
		var supersededBy *int64
		if err := rows.Scan(&sourceType, &proj, &supersededBy); err != nil {
			t.Fatalf("Failed to scan memory row: %v", err)
		}
		rowCount++
		if sourceType != capsule.CapsuleSourceType {
			t.Errorf("Expected source_type '%s', got '%s'", capsule.CapsuleSourceType, sourceType)
		}
		if proj != projectName {
			t.Errorf("Expected project '%s', got '%s'", projectName, proj)
		}
		if supersededBy != nil {
			t.Errorf("Expected superseded_by to be nil for capsules, got %v", *supersededBy)
		}
	}
	if rowCount != 4 {
		t.Errorf("Expected 4 memories in DB, got %d", rowCount)
	}

	// AC-3: Second Import run against unchanged fixture set
	res2, err := capsule.Import(hDB, tasksRoot, projectName)
	if err != nil {
		t.Fatalf("Second Import failed: %v", err)
	}

	if res2.Ingested != 0 {
		t.Errorf("Expected Ingested=0 on second run, got %d", res2.Ingested)
	}
	if res2.Skipped != 4 {
		t.Errorf("Expected Skipped=4 on second run, got %d", res2.Skipped)
	}

	// Verify DB row count remains 4
	var countAfter int
	err = hDB.QueryRow("SELECT COUNT(*) FROM memories").Scan(&countAfter)
	if err != nil {
		t.Fatalf("Failed to query count: %v", err)
	}
	if countAfter != 4 {
		t.Errorf("Expected 4 memories after re-import, got %d", countAfter)
	}
}

func TestImport_ReusesCapsuleFixtures(t *testing.T) {
	// Exercise capsule.Import against a tasksRoot pointing to testdata/capsule fixtures
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "hsme.db")
	hDB, err := sqlite.InitDB(dbPath)
	if err != nil {
		t.Fatalf("Failed to init HSME DB: %v", err)
	}
	defer hDB.Close()

	// Build a tasksRoot layout using directory paths
	fullFixtureDir := filepath.Join("testdata", "capsule", "full")
	doneRoot := filepath.Join(tempDir, "done")
	if err := createSymlinkOrCopy(fullFixtureDir, filepath.Join(doneRoot, "HSME-FULL")); err != nil {
		t.Skipf("Failed to setup fixture dir: %v", err)
	}

	res, err := capsule.Import(hDB, tempDir, "fixture-test")
	if err != nil {
		t.Fatalf("Import failed on reused capsule fixture: %v", err)
	}
	if res.Ingested != 1 {
		t.Errorf("Expected Ingested=1 for full capsule fixture, got %d", res.Ingested)
	}
}

func createSymlinkOrCopy(src, dst string) error {
	absSrc, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	parent := filepath.Dir(dst)
	if err := os.MkdirAll(parent, 0755); err != nil {
		return err
	}
	return os.Symlink(absSrc, dst)
}
