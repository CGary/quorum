package admin

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Restore performs an atomic restore of the database from srcPath to dbPath.
// It includes an integrity check of the source and cleanup of WAL/SHM sidecars.
func Restore(ctx context.Context, dbPath string, srcPath string) error {
	// 1. Integrity check of the source backup
	srcDB, err := sql.Open("sqlite3", srcPath)
	if err != nil {
		return fmt.Errorf("failed to open backup file: %w", err)
	}
	defer srcDB.Close()

	var integrity string
	if err := srcDB.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil {
		return fmt.Errorf("integrity check failed: %w", err)
	}
	if integrity != "ok" {
		return fmt.Errorf("backup file is corrupt: %s", integrity)
	}
	// Explicitly close before copying the file
	srcDB.Close()

	// 2. Atomic swap using a temporary file
	dbDir := filepath.Dir(dbPath)
	tmpPath := filepath.Join(dbDir, "restore.db.tmp")

	if err := copyFile(srcPath, tmpPath); err != nil {
		return fmt.Errorf("failed to copy backup to temp: %w", err)
	}

	// 3. Sidecar cleanup
	// Removing these ensures that the next connection doesn't try to use
	// stale WAL/SHM data from the previous database instance.
	_ = os.Remove(dbPath + "-wal")
	_ = os.Remove(dbPath + "-shm")

	// Rename is atomic on Linux/Unix
	if err := os.Rename(tmpPath, dbPath); err != nil {
	        // Cleanup temp file if rename fails
	        os.Remove(tmpPath)
	        return fmt.Errorf("failed to swap database file: %w", err)
	}

	return nil
	}
// copyFile is a helper to copy a file from src to dst.
func copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	_, err = io.Copy(destination, source)
	return err
}
