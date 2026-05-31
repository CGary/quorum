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
	db.SetMaxOpenConns(1)

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
CREATE TABLE IF NOT EXISTS projects (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	root_path TEXT,
	git_remote TEXT,
	created_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
	updated_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE IF NOT EXISTS memory_entries (
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

CREATE TABLE IF NOT EXISTS memory_related (
	project_id TEXT NOT NULL,
	memory_id TEXT NOT NULL,
	related_ref TEXT NOT NULL,
	PRIMARY KEY (project_id, memory_id, related_ref),
	FOREIGN KEY (project_id, memory_id) REFERENCES memory_entries(project_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS memory_anti_patterns (
	project_id TEXT NOT NULL,
	memory_id TEXT NOT NULL,
	ordinal INTEGER NOT NULL,
	content TEXT NOT NULL,
	PRIMARY KEY (project_id, memory_id, ordinal),
	FOREIGN KEY (project_id, memory_id) REFERENCES memory_entries(project_id, id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS memory_supersession_edges (
	project_id TEXT NOT NULL,
	from_id TEXT NOT NULL,
	to_id TEXT NOT NULL,
	PRIMARY KEY (project_id, from_id, to_id),
	FOREIGN KEY (project_id, from_id) REFERENCES memory_entries(project_id, id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_memory_entries_type ON memory_entries(type);
CREATE INDEX IF NOT EXISTS idx_memory_entries_project_type ON memory_entries(project_id, type);
CREATE INDEX IF NOT EXISTS idx_memory_entries_source_task ON memory_entries(project_id, source_task);
CREATE INDEX IF NOT EXISTS idx_memory_entries_created_at ON memory_entries(created_at);
CREATE INDEX IF NOT EXISTS idx_memory_entries_hash ON memory_entries(content_hash);
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

	var generic interface{}
	if err := json.Unmarshal(b, &generic); err != nil {
		return "", err
	}

	canonicalBytes, err := json.Marshal(generic)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256(canonicalBytes)
	return fmt.Sprintf("%x", hash), nil
}
