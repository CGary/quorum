package admin

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestRetryFailedTasks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_retry.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}
	defer db.Close()

	// Create table
	_, err = db.Exec(`
		CREATE TABLE async_tasks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			status TEXT NOT NULL,
			attempt_count INTEGER NOT NULL
		)
	`)
	if err != nil {
		t.Fatalf("failed to create table: %v", err)
	}

	// Insert tasks
	_, err = db.Exec(`
		INSERT INTO async_tasks (status, attempt_count) VALUES
		('pending', 0),
		('failed', 3),
		('pending', 5),
		('completed', 1)
	`)
	if err != nil {
		t.Fatalf("failed to insert tasks: %v", err)
	}

	affected, err := RetryFailedTasks(context.Background(), db)
	if err != nil {
		t.Fatalf("RetryFailedTasks failed: %v", err)
	}

	if affected != 2 {
		t.Errorf("expected 2 affected rows, got %d", affected)
	}

	var pendingCount int
	err = db.QueryRow("SELECT COUNT(*) FROM async_tasks WHERE status = 'pending' AND attempt_count = 0").Scan(&pendingCount)
	if err != nil {
		t.Fatalf("failed to query pending count: %v", err)
	}

	if pendingCount != 3 { // Original pending(0) + retried failed(3) + retried exhausted(5)
		t.Errorf("expected 3 pending tasks, got %d", pendingCount)
	}
}

func TestBackupAndRestore(t *testing.T) {
	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "src.db")
	backupPath := filepath.Join(tmpDir, "backup.db")
	restorePath := filepath.Join(tmpDir, "restore.db")

	// 1. Setup source DB
	srcDB, err := sql.Open("sqlite3", srcPath)
	if err != nil {
		t.Fatalf("failed to open source db: %v", err)
	}
	_, err = srcDB.Exec("CREATE TABLE data (val TEXT); INSERT INTO data VALUES ('hello')")
	if err != nil {
		t.Fatalf("failed to setup source data: %v", err)
	}
	srcDB.Close()

	// 2. Backup
	err = Backup(context.Background(), srcPath, backupPath)
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	// Verify backup file exists
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Fatalf("backup file was not created")
	}

	// 3. Restore
	err = Restore(context.Background(), restorePath, backupPath)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// 4. Verify restored data
	resDB, err := sql.Open("sqlite3", restorePath)
	if err != nil {
		t.Fatalf("failed to open restored db: %v", err)
	}
	defer resDB.Close()

	var val string
	err = resDB.QueryRow("SELECT val FROM data").Scan(&val)
	if err != nil {
		t.Fatalf("failed to query restored data: %v", err)
	}

	if val != "hello" {
		t.Errorf("expected 'hello', got %q", val)
	}
}

func TestRestoreCorrupt(t *testing.T) {
	tmpDir := t.TempDir()
	corruptPath := filepath.Join(tmpDir, "corrupt.db")
	targetPath := filepath.Join(tmpDir, "target.db")

	// Create a corrupt file (just random bytes)
	err := os.WriteFile(corruptPath, []byte("this is not a sqlite database"), 0644)
	if err != nil {
		t.Fatalf("failed to create corrupt file: %v", err)
	}

	err = Restore(context.Background(), targetPath, corruptPath)
	if err == nil {
		t.Error("expected error when restoring corrupt file, got nil")
	}
}
