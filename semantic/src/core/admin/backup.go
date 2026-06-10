package admin

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mattn/go-sqlite3"
)

// Backup performs an online backup of the SQLite database at srcPath to destPath.
func Backup(ctx context.Context, srcPath string, destPath string) error {
	// We use the standard sqlite3 driver for backup operations to ensure
	// we can access the underlying SQLiteConn via Raw().
	srcDB, err := sql.Open("sqlite3", srcPath)
	if err != nil {
		return fmt.Errorf("failed to open source database: %w", err)
	}
	defer srcDB.Close()

	dstDB, err := sql.Open("sqlite3", destPath)
	if err != nil {
		return fmt.Errorf("failed to open destination database: %w", err)
	}
	defer dstDB.Close()

	srcConn, err := srcDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to get source connection: %w", err)
	}
	defer srcConn.Close()

	dstConn, err := dstDB.Conn(ctx)
	if err != nil {
		return fmt.Errorf("failed to get destination connection: %w", err)
	}
	defer dstConn.Close()

	return dstConn.Raw(func(dstRaw interface{}) error {
		return srcConn.Raw(func(srcRaw interface{}) error {
			dstSQLite, ok := dstRaw.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("destination is not a sqlite3 connection")
			}
			srcSQLite, ok := srcRaw.(*sqlite3.SQLiteConn)
			if !ok {
				return fmt.Errorf("source is not a sqlite3 connection")
			}

			b, err := dstSQLite.Backup("main", srcSQLite, "main")
			if err != nil {
				return fmt.Errorf("failed to initialize backup: %w", err)
			}
			if _, err := b.Step(-1); err != nil {
				return fmt.Errorf("failed to step backup: %w", err)
			}
			return b.Finish()
		})
	})
}
