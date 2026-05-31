package core

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeMemoryFixture(t *testing.T, root, dir, name, id, content string) string {
	t.Helper()
	path := filepath.Join(root, "memory", dir, name)
	raw := `{"id":"` + id + `","source_task":"SQL-03","type":"decision","title":"Valid migration memory","context":"migration test","content":"` + content + `","created_at":"2026-05-31"}`
	writeFile(t, path, raw)
	return path
}

func openMigrationTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := OpenMemoryDB(filepath.Join(t.TempDir(), "memory.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestRunInitMemoryMigrationMigratesAndDeletesVerifiedFiles(t *testing.T) {
	useSchemas(t)
	root := t.TempDir()
	db := openMigrationTestDB(t)
	config := &QuorumConfig{ProjectID: "sql-03", ProjectName: "SQL 03"}
	if err := EnsureMemoryProject(db, config, root, ""); err != nil {
		t.Fatal(err)
	}
	memoryPath := writeMemoryFixture(t, root, "decisions", "DEC-2026-05-31-1.json", "DEC-2026-05-31-1", "This valid memory entry should be migrated and safely removed.")

	result, err := RunInitMemoryMigration(db, root, config)
	if err != nil {
		t.Fatalf("RunInitMemoryMigration failed: %v", err)
	}
	if result.FilesSeen != 1 || result.FilesInserted != 1 || result.FilesDeleted != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(memoryPath); !os.IsNotExist(err) {
		t.Fatalf("expected migrated file to be removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "memory")); !os.IsNotExist(err) {
		t.Fatalf("expected empty legacy memory root to be removed, stat err=%v", err)
	}
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM memory_entries WHERE project_id = ? AND id = ? AND content_hash IS NOT NULL", "sql-03", "DEC-2026-05-31-1").Scan(&count); err != nil || count != 1 {
		t.Fatalf("expected migrated DB entry, count=%d err=%v", count, err)
	}
}

func TestRunInitMemoryMigrationInvalidMemoryPreservesFiles(t *testing.T) {
	useSchemas(t)
	root := t.TempDir()
	db := openMigrationTestDB(t)
	config := &QuorumConfig{ProjectID: "sql-03", ProjectName: "SQL 03"}
	if err := EnsureMemoryProject(db, config, root, ""); err != nil {
		t.Fatal(err)
	}
	valid := writeMemoryFixture(t, root, "decisions", "DEC-2026-05-31-1.json", "DEC-2026-05-31-1", "This valid memory entry should not be deleted when the batch fails.")
	invalid := filepath.Join(root, "memory", "lessons", "LES-2026-05-31-1.json")
	writeFile(t, invalid, `{"id":"LES-2026-05-31-1","type":"lesson","title":"bad","content":"too short","created_at":"2026-05-31"}`)

	_, err := RunInitMemoryMigration(db, root, config)
	if err == nil {
		t.Fatal("expected invalid memory to fail migration")
	}
	for _, path := range []string{valid, invalid} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("expected %s to remain after failed migration: %v", path, statErr)
		}
	}
}

func TestRunInitMemoryMigrationNormalizesLegacyMemoryShape(t *testing.T) {
	useSchemas(t)
	root := t.TempDir()
	db := openMigrationTestDB(t)
	config := &QuorumConfig{ProjectID: "sql-03", ProjectName: "SQL 03"}
	if err := EnsureMemoryProject(db, config, root, ""); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "memory", "patterns", "pat-go-cli-parity.json")
	writeFile(t, path, `{
		"id": "pat-go-cli-parity",
		"type": "pattern",
		"title": "Strict CLI Invariant Parity",
		"context": "When porting CLI state-mutating commands from Python to Go.",
		"resolution": "State mutations in Go must enforce identical invariants and preserve Python-compatible error output formats.",
		"task_ref": "F-03-d"
	}`)
	mtime := time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}

	result, err := RunInitMemoryMigration(db, root, config)
	if err != nil {
		t.Fatalf("RunInitMemoryMigration failed: %v", err)
	}
	if result.FilesSeen != 1 || result.FilesInserted != 1 || result.FilesDeleted != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected normalized legacy file to be removed, stat err=%v", err)
	}

	expectedID := legacyMemoryID(path, "pattern", "pat-go-cli-parity", "2026-05-26")
	var gotID, sourceTask, content, createdAt, rawJSON string
	if err := db.QueryRow(`SELECT id, source_task, content, created_at, raw_json
		FROM memory_entries WHERE project_id = ?`, "sql-03").Scan(&gotID, &sourceTask, &content, &createdAt, &rawJSON); err != nil {
		t.Fatal(err)
	}
	if gotID != expectedID {
		t.Fatalf("expected generated id %q, got %q", expectedID, gotID)
	}
	if sourceTask != "F-03-d" {
		t.Fatalf("expected task_ref to become source_task, got %q", sourceTask)
	}
	if !strings.Contains(content, "identical invariants") {
		t.Fatalf("expected resolution to become content, got %q", content)
	}
	if createdAt != "2026-05-26" {
		t.Fatalf("expected created_at from file mtime, got %q", createdAt)
	}
	if strings.Contains(rawJSON, "task_ref") || strings.Contains(rawJSON, "resolution") {
		t.Fatalf("expected raw_json to store normalized schema payload, got %s", rawJSON)
	}
}

func TestRunInitMemoryMigrationDuplicateHandling(t *testing.T) {
	useSchemas(t)
	root := t.TempDir()
	db := openMigrationTestDB(t)
	config := &QuorumConfig{ProjectID: "sql-03", ProjectName: "SQL 03"}
	if err := EnsureMemoryProject(db, config, root, ""); err != nil {
		t.Fatal(err)
	}
	writeMemoryFixture(t, root, "decisions", "DEC-2026-05-31-1.json", "DEC-2026-05-31-1", "This content is the first version of the duplicated memory.")
	writeMemoryFixture(t, root, "lessons", "DEC-2026-05-31-1.json", "DEC-2026-05-31-1", "This content is the first version of the duplicated memory.")
	if result, err := RunInitMemoryMigration(db, root, config); err != nil || result.FilesDeleted != 2 {
		t.Fatalf("expected same-hash duplicates to migrate idempotently, result=%+v err=%v", result, err)
	}

	root = t.TempDir()
	writeMemoryFixture(t, root, "decisions", "DEC-2026-05-31-2.json", "DEC-2026-05-31-2", "This content is the first version of another duplicated memory.")
	kept := writeMemoryFixture(t, root, "lessons", "DEC-2026-05-31-2.json", "DEC-2026-05-31-2", "This content is a conflicting version of another duplicated memory.")
	_, err := RunInitMemoryMigration(db, root, config)
	if err == nil || !strings.Contains(err.Error(), "different content hashes") {
		t.Fatalf("expected duplicate hash conflict, got %v", err)
	}
	if _, statErr := os.Stat(kept); statErr != nil {
		t.Fatalf("expected conflicting duplicate file to remain: %v", statErr)
	}
}

func TestRunInitMemoryMigrationUnexpectedFilesAndDeleteFailurePreserveSafety(t *testing.T) {
	useSchemas(t)
	root := t.TempDir()
	db := openMigrationTestDB(t)
	config := &QuorumConfig{ProjectID: "sql-03", ProjectName: "SQL 03"}
	if err := EnsureMemoryProject(db, config, root, ""); err != nil {
		t.Fatal(err)
	}
	unexpected := filepath.Join(root, "memory", "notes.txt")
	writeFile(t, unexpected, "do not delete")
	if _, err := RunInitMemoryMigration(db, root, config); err == nil || !strings.Contains(err.Error(), "unexpected file") {
		t.Fatalf("expected unexpected file error, got %v", err)
	}
	if _, err := os.Stat(unexpected); err != nil {
		t.Fatalf("unexpected file must remain: %v", err)
	}

	root = t.TempDir()
	victim := writeMemoryFixture(t, root, "decisions", "DEC-2026-05-31-3.json", "DEC-2026-05-31-3", "This valid memory entry should remain when deletion fails.")
	result, err := RunInitMemoryMigrationWithOptions(db, root, config, InitMigrationOptions{RemoveFile: func(path string) error { return os.ErrPermission }})
	if err == nil || !strings.Contains(err.Error(), "failed deleting") {
		t.Fatalf("expected deletion failure, result=%+v err=%v", result, err)
	}
	if _, statErr := os.Stat(victim); statErr != nil {
		t.Fatalf("file should remain after deletion failure: %v", statErr)
	}
}
