package core

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// MemoryDBPath resolves the path to the SQLite database.
func MemoryDBPath() (string, error) {
	if envPath := os.Getenv("QUORUM_MEMORY_DB"); envPath != "" {
		if err := os.MkdirAll(filepath.Dir(envPath), 0755); err != nil {
			return "", err
		}
		return envPath, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dbDir := filepath.Join(home, ".quorum")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dbDir, "memory.db"), nil
}

// OpenMemoryDB opens a connection to the SQLite database and runs migrations.
func OpenMemoryDB(dbPath string) (*sql.DB, error) {
	if dbPath == "" {
		var err error
		dbPath, err = MemoryDBPath()
		if err != nil {
			return nil, err
		}
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	// PRAGMAs required by the invariants
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA foreign_keys = ON;",
	}
	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to execute pragma %s: %w", pragma, err)
		}
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return db, nil
}

func initSchema(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}

	schema := `
CREATE TABLE IF NOT EXISTS memory_entries (
	id TEXT PRIMARY KEY,
	type TEXT NOT NULL,
	content TEXT NOT NULL,
	hash TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS memory_related (
	entry_id TEXT NOT NULL,
	related_id TEXT NOT NULL,
	PRIMARY KEY (entry_id, related_id),
	FOREIGN KEY (entry_id) REFERENCES memory_entries(id) ON DELETE CASCADE,
	FOREIGN KEY (related_id) REFERENCES memory_entries(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS memory_anti_patterns (
	entry_id TEXT NOT NULL,
	anti_pattern TEXT NOT NULL,
	PRIMARY KEY (entry_id, anti_pattern),
	FOREIGN KEY (entry_id) REFERENCES memory_entries(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS memory_supersession_edges (
	supersedes_id TEXT NOT NULL,
	superseded_by_id TEXT NOT NULL,
	PRIMARY KEY (supersedes_id, superseded_by_id),
	FOREIGN KEY (supersedes_id) REFERENCES memory_entries(id) ON DELETE CASCADE,
	FOREIGN KEY (superseded_by_id) REFERENCES memory_entries(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_memory_entries_hash ON memory_entries(hash);
`

	if _, err := tx.Exec(schema); err != nil {
		tx.Rollback()
		return err
	}

	return tx.Commit()
}

// CanonicalMemoryHash calculates the canonical SHA256 of JSON payloads.
func CanonicalMemoryHash(payload interface{}) (string, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	
	// Unmarshal back to an interface{} to ensure ordering/whitespace is ignored when re-marshaling canonically
	var generic interface{}
	if err := json.Unmarshal(b, &generic); err != nil {
		return "", err
	}
	
	// Marshal canonically
	canonicalBytes, err := json.Marshal(generic)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(canonicalBytes)
	return fmt.Sprintf("%x", hash), nil
}
