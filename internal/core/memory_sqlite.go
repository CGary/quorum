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
	project_id TEXT,
	type TEXT NOT NULL,
	source_task TEXT,
	title TEXT,
	context TEXT,
	content TEXT NOT NULL,
	hash TEXT NOT NULL,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	raw_json TEXT
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

`

	if _, err := tx.Exec(schema); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}
	if err := ensureMemoryEntryColumns(db); err != nil {
		return err
	}
	if _, err := db.Exec(`
CREATE INDEX IF NOT EXISTS idx_memory_entries_hash ON memory_entries(hash);
CREATE INDEX IF NOT EXISTS idx_memory_entries_project_type ON memory_entries(project_id, type);
`); err != nil {
		return err
	}

	return initMemoryFTS(db)
}

func ensureMemoryEntryColumns(db *sql.DB) error {
	rows, err := db.Query("PRAGMA table_info(memory_entries);")
	if err != nil {
		return err
	}
	defer rows.Close()
	existing := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		existing[name] = true
	}
	if err := rows.Err(); err != nil {
		return err
	}
	columns := map[string]string{
		"project_id":  "TEXT",
		"source_task": "TEXT",
		"title":       "TEXT",
		"context":     "TEXT",
		"raw_json":    "TEXT",
	}
	for name, typ := range columns {
		if !existing[name] {
			if _, err := db.Exec(fmt.Sprintf("ALTER TABLE memory_entries ADD COLUMN %s %s", name, typ)); err != nil {
				return err
			}
		}
	}
	return nil
}

// initMemoryFTS creates an optional FTS5 read index when the SQLite build supports it.
func initMemoryFTS(db *sql.DB) error {
	if _, err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(
		id UNINDEXED,
		project_id UNINDEXED,
		type UNINDEXED,
		title,
		context,
		content,
		anti_patterns
	);`); err != nil {
		// FTS5 is optional. Search falls back to deterministic LIKE queries.
		return nil
	}

	triggers := []string{
		`CREATE TRIGGER IF NOT EXISTS memory_entries_fts_ai AFTER INSERT ON memory_entries BEGIN
			INSERT INTO memory_fts(id, project_id, type, title, context, content, anti_patterns)
			VALUES (new.id, new.project_id, new.type, COALESCE(new.title, ''), COALESCE(new.context, ''), new.content,
				COALESCE((SELECT group_concat(anti_pattern, char(10)) FROM memory_anti_patterns WHERE entry_id = new.id), ''));
		END;`,
		`CREATE TRIGGER IF NOT EXISTS memory_entries_fts_ad AFTER DELETE ON memory_entries BEGIN
			DELETE FROM memory_fts WHERE id = old.id;
		END;`,
		`CREATE TRIGGER IF NOT EXISTS memory_entries_fts_au AFTER UPDATE ON memory_entries BEGIN
			DELETE FROM memory_fts WHERE id = old.id;
			INSERT INTO memory_fts(id, project_id, type, title, context, content, anti_patterns)
			VALUES (new.id, new.project_id, new.type, COALESCE(new.title, ''), COALESCE(new.context, ''), new.content,
				COALESCE((SELECT group_concat(anti_pattern, char(10)) FROM memory_anti_patterns WHERE entry_id = new.id), ''));
		END;`,
		`CREATE TRIGGER IF NOT EXISTS memory_anti_patterns_fts_ai AFTER INSERT ON memory_anti_patterns BEGIN
			UPDATE memory_fts SET anti_patterns = COALESCE((SELECT group_concat(anti_pattern, char(10)) FROM memory_anti_patterns WHERE entry_id = new.entry_id), '') WHERE id = new.entry_id;
		END;`,
		`CREATE TRIGGER IF NOT EXISTS memory_anti_patterns_fts_ad AFTER DELETE ON memory_anti_patterns BEGIN
			UPDATE memory_fts SET anti_patterns = COALESCE((SELECT group_concat(anti_pattern, char(10)) FROM memory_anti_patterns WHERE entry_id = old.entry_id), '') WHERE id = old.entry_id;
		END;`,
	}
	for _, trigger := range triggers {
		if _, err := db.Exec(trigger); err != nil {
			return nil
		}
	}
	return nil
}

func memoryFTSAvailable(db *sql.DB) bool {
	var name string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='memory_fts';").Scan(&name)
	return err == nil && name == "memory_fts"
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
