package core

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
	id TEXT PRIMARY KEY,
	project_id TEXT,
	type TEXT NOT NULL,
	content TEXT NOT NULL,
	raw_json TEXT,
	hash TEXT NOT NULL,
	content_hash TEXT,
	source_path TEXT,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
	FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS memory_related (
	entry_id TEXT NOT NULL,
	related_id TEXT NOT NULL,
	PRIMARY KEY (entry_id, related_id),
	FOREIGN KEY (entry_id) REFERENCES memory_entries(id) ON DELETE CASCADE
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

CREATE TABLE IF NOT EXISTS init_migrations (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id TEXT NOT NULL,
	started_at TEXT NOT NULL,
	completed_at TEXT,
	status TEXT NOT NULL,
	files_seen INTEGER NOT NULL DEFAULT 0,
	files_inserted INTEGER NOT NULL DEFAULT 0,
	files_deleted INTEGER NOT NULL DEFAULT 0,
	message TEXT,
	FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_memory_entries_hash ON memory_entries(hash);
CREATE INDEX IF NOT EXISTS idx_memory_entries_content_hash ON memory_entries(content_hash);
CREATE INDEX IF NOT EXISTS idx_memory_entries_project_id ON memory_entries(project_id);
`
	if _, err := tx.Exec(schema); err != nil {
		tx.Rollback()
		return err
	}
	if err := ensureColumn(tx, "memory_entries", "project_id", "TEXT"); err != nil {
		tx.Rollback()
		return err
	}
	if err := ensureColumn(tx, "memory_entries", "raw_json", "TEXT"); err != nil {
		tx.Rollback()
		return err
	}
	if err := ensureColumn(tx, "memory_entries", "content_hash", "TEXT"); err != nil {
		tx.Rollback()
		return err
	}
	if err := ensureColumn(tx, "memory_entries", "source_path", "TEXT"); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_memory_entries_content_hash ON memory_entries(content_hash)"); err != nil {
		tx.Rollback()
		return err
	}
	if _, err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_memory_entries_project_id ON memory_entries(project_id)"); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

type schemaExecer interface {
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

func ensureColumn(db schemaExecer, table, column, decl string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	_, err = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + decl)
	return err
}

func EnsureMemoryProject(db *sql.DB, config *QuorumConfig, rootPath, gitRemote string) error {
	if err := ValidateQuorumConfig(config); err != nil {
		return err
	}
	var existingName, existingRoot, existingRemote string
	err := db.QueryRow("SELECT name, COALESCE(root_path, ''), COALESCE(git_remote, '') FROM projects WHERE id = ?", config.ProjectID).Scan(&existingName, &existingRoot, &existingRemote)
	if err == sql.ErrNoRows {
		_, err = db.Exec("INSERT INTO projects (id, name, root_path, git_remote, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)", config.ProjectID, config.ProjectName, rootPath, gitRemote, time.Now().UTC().Format(time.RFC3339), time.Now().UTC().Format(time.RFC3339))
		return err
	}
	if err != nil {
		return err
	}
	if !projectMetadataCompatible(existingRoot, existingRemote, rootPath, gitRemote) {
		return fmt.Errorf("project_id %q already exists for incompatible root or remote", config.ProjectID)
	}
	_, err = db.Exec("UPDATE projects SET name = ?, root_path = ?, git_remote = ?, updated_at = ? WHERE id = ?", config.ProjectName, rootPath, gitRemote, time.Now().UTC().Format(time.RFC3339), config.ProjectID)
	return err
}

func projectMetadataCompatible(existingRoot, existingRemote, rootPath, gitRemote string) bool {
	if existingRemote != "" && gitRemote != "" {
		return existingRemote == gitRemote
	}
	if existingRoot != "" && rootPath != "" {
		return filepath.Clean(existingRoot) == filepath.Clean(rootPath)
	}
	return true
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

func canonicalMemoryJSON(payload interface{}) (string, error) {
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
	return string(canonicalBytes), nil
}

func memoryEntryID(payload map[string]any) string {
	id, _ := payload["id"].(string)
	return strings.TrimSpace(id)
}
