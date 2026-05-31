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

	if err := ensureMemoryEntryColumns(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ensure memory entry columns: %w", err)
	}

	if err := initMemoryFTS(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize memory FTS: %w", err)
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
`
	if _, err := tx.Exec(schema); err != nil {
		tx.Rollback()
		return err
	}
	if err := ensureColumn(tx, "memory_entries", "project_id", "TEXT"); err != nil {
		tx.Rollback()
		return err
	}
	if err := ensureColumn(tx, "memory_entries", "source_task", "TEXT"); err != nil {
		tx.Rollback()
		return err
	}
	if err := ensureColumn(tx, "memory_entries", "title", "TEXT"); err != nil {
		tx.Rollback()
		return err
	}
	if err := ensureColumn(tx, "memory_entries", "context", "TEXT"); err != nil {
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

	indices := []string{
		"CREATE INDEX IF NOT EXISTS idx_memory_entries_type ON memory_entries(type)",
		"CREATE INDEX IF NOT EXISTS idx_memory_entries_project_type ON memory_entries(project_id, type)",
		"CREATE INDEX IF NOT EXISTS idx_memory_entries_source_task ON memory_entries(project_id, source_task)",
		"CREATE INDEX IF NOT EXISTS idx_memory_entries_created_at ON memory_entries(created_at)",
		"CREATE INDEX IF NOT EXISTS idx_memory_entries_content_hash ON memory_entries(content_hash)",
		"CREATE INDEX IF NOT EXISTS idx_memory_entries_project_id ON memory_entries(project_id)",
	}
	for _, idx := range indices {
		if _, err := tx.Exec(idx); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
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
		"project_id":   "TEXT",
		"source_task":  "TEXT",
		"title":        "TEXT",
		"context":      "TEXT",
		"raw_json":     "TEXT",
		"content_hash": "TEXT",
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
				COALESCE((SELECT group_concat(content, char(10)) FROM memory_anti_patterns WHERE memory_id = new.id AND project_id = new.project_id), ''));
		END;`,
		`CREATE TRIGGER IF NOT EXISTS memory_entries_fts_ad AFTER DELETE ON memory_entries BEGIN
			DELETE FROM memory_fts WHERE id = old.id AND project_id = old.project_id;
		END;`,
		`CREATE TRIGGER IF NOT EXISTS memory_entries_fts_au AFTER UPDATE ON memory_entries BEGIN
			DELETE FROM memory_fts WHERE id = old.id AND project_id = old.project_id;
			INSERT INTO memory_fts(id, project_id, type, title, context, content, anti_patterns)
			VALUES (new.id, new.project_id, new.type, COALESCE(new.title, ''), COALESCE(new.context, ''), new.content,
				COALESCE((SELECT group_concat(content, char(10)) FROM memory_anti_patterns WHERE memory_id = new.id AND project_id = new.project_id), ''));
		END;`,
		`CREATE TRIGGER IF NOT EXISTS memory_anti_patterns_fts_ai AFTER INSERT ON memory_anti_patterns BEGIN
			UPDATE memory_fts SET anti_patterns = COALESCE((SELECT group_concat(content, char(10)) FROM memory_anti_patterns WHERE memory_id = new.memory_id AND project_id = new.project_id), '') WHERE id = new.memory_id AND project_id = new.project_id;
		END;`,
		`CREATE TRIGGER IF NOT EXISTS memory_anti_patterns_fts_ad AFTER DELETE ON memory_anti_patterns BEGIN
			UPDATE memory_fts SET anti_patterns = COALESCE((SELECT group_concat(content, char(10)) FROM memory_anti_patterns WHERE memory_id = old.memory_id AND project_id = old.project_id), '') WHERE id = old.memory_id AND project_id = old.project_id;
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

func memoryEntryID(payload map[string]any) string {
	id, _ := payload["id"].(string)
	return strings.TrimSpace(id)
}
