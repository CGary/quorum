package core

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
)

type MemoryEntry struct {
	ID           string   `json:"id"`
	SourceTask   string   `json:"source_task"`
	Type         string   `json:"type"`
	Title        string   `json:"title"`
	Context      string   `json:"context"`
	Content      string   `json:"content"`
	Related      []string `json:"related,omitempty"`
	AntiPatterns []string `json:"anti_patterns,omitempty"`
	CreatedAt    string   `json:"created_at"`
	Supersedes   string   `json:"supersedes,omitempty"`
}

type MemorySaveResult struct {
	ProjectID   string `json:"project_id"`
	MemoryID    string `json:"memory_id"`
	Status      string `json:"status"`
	ContentHash string `json:"content_hash"`
}

type MemoryTypeCounts struct {
	Pattern  int `json:"pattern"`
	Decision int `json:"decision"`
	Lesson   int `json:"lesson"`
}

type MemoryStatusResult struct {
	DBPath            string           `json:"db_path"`
	ProjectID         string           `json:"project_id"`
	ProjectName       string           `json:"project_name"`
	ProjectRegistered bool             `json:"project_registered"`
	Counts            MemoryTypeCounts `json:"counts"`
}

func SaveMemoryEntry(raw []byte) (MemorySaveResult, error) {
	config, err := ReadQuorumConfig()
	if err != nil {
		return MemorySaveResult{}, fmt.Errorf("failed to load .quorumrc; run quorum init: %w", err)
	}
	dbPath, err := MemoryDBPath()
	if err != nil {
		return MemorySaveResult{}, fmt.Errorf("failed to resolve memory database path: %w", err)
	}
	db, err := OpenMemoryDB(dbPath)
	if err != nil {
		return MemorySaveResult{}, fmt.Errorf("failed to open memory database: %w", err)
	}
	defer db.Close()
	return SaveMemoryEntryWithDB(db, config, raw)
}

func SaveMemoryEntryWithDB(db *sql.DB, config *QuorumConfig, raw []byte) (MemorySaveResult, error) {
	payload, canonicalRaw, err := parseAndValidateMemory(raw)
	if err != nil {
		return MemorySaveResult{}, err
	}
	hash, err := CanonicalMemoryHash(payload)
	if err != nil {
		return MemorySaveResult{}, fmt.Errorf("failed to calculate content hash: %w", err)
	}

	var entry MemoryEntry
	if err := json.Unmarshal(canonicalRaw, &entry); err != nil {
		return MemorySaveResult{}, fmt.Errorf("failed to decode memory payload: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return MemorySaveResult{}, fmt.Errorf("failed to begin memory save transaction: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`INSERT INTO projects (id, name, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP)
ON CONFLICT(id) DO UPDATE SET name=excluded.name, updated_at=CURRENT_TIMESTAMP`, config.ProjectID, config.ProjectName); err != nil {
		return MemorySaveResult{}, fmt.Errorf("failed to register project: %w", err)
	}

	var existingHash string
	err = tx.QueryRow(`SELECT content_hash FROM memory_entries WHERE project_id = ? AND id = ?`, config.ProjectID, entry.ID).Scan(&existingHash)
	if err == nil {
		if existingHash == hash {
			return MemorySaveResult{ProjectID: config.ProjectID, MemoryID: entry.ID, Status: "unchanged", ContentHash: hash}, tx.Commit()
		}
		return MemorySaveResult{}, fmt.Errorf("memory conflict for project_id=%s id=%s: existing content_hash differs", config.ProjectID, entry.ID)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return MemorySaveResult{}, fmt.Errorf("failed to check existing memory entry: %w", err)
	}

	if _, err := tx.Exec(`INSERT INTO memory_entries
(project_id, id, type, source_task, title, context, content, created_at, supersedes, content_hash, raw_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		config.ProjectID, entry.ID, entry.Type, entry.SourceTask, entry.Title, entry.Context, entry.Content, entry.CreatedAt, nullableString(entry.Supersedes), hash, string(canonicalRaw)); err != nil {
		return MemorySaveResult{}, fmt.Errorf("failed to insert memory entry: %w", err)
	}

	for _, related := range entry.Related {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO memory_related (project_id, memory_id, related_ref) VALUES (?, ?, ?)`, config.ProjectID, entry.ID, related); err != nil {
			return MemorySaveResult{}, fmt.Errorf("failed to insert related memory reference: %w", err)
		}
	}
	for i, antiPattern := range entry.AntiPatterns {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO memory_anti_patterns (project_id, memory_id, ordinal, content) VALUES (?, ?, ?, ?)`, config.ProjectID, entry.ID, i, antiPattern); err != nil {
			return MemorySaveResult{}, fmt.Errorf("failed to insert memory anti-pattern: %w", err)
		}
	}
	if entry.Supersedes != "" {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO memory_supersession_edges (project_id, from_id, to_id) VALUES (?, ?, ?)`, config.ProjectID, entry.ID, entry.Supersedes); err != nil {
			return MemorySaveResult{}, fmt.Errorf("failed to insert memory supersession edge: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return MemorySaveResult{}, fmt.Errorf("failed to commit memory entry: %w", err)
	}
	return MemorySaveResult{ProjectID: config.ProjectID, MemoryID: entry.ID, Status: "inserted", ContentHash: hash}, nil
}

func MemoryStatus() (MemoryStatusResult, error) {
	config, err := ReadQuorumConfig()
	if err != nil {
		return MemoryStatusResult{}, fmt.Errorf("failed to load .quorumrc; run quorum init: %w", err)
	}
	dbPath, err := MemoryDBPath()
	if err != nil {
		return MemoryStatusResult{}, fmt.Errorf("failed to resolve memory database path: %w", err)
	}
	db, err := OpenMemoryDB(dbPath)
	if err != nil {
		return MemoryStatusResult{}, fmt.Errorf("failed to open memory database: %w", err)
	}
	defer db.Close()
	return MemoryStatusWithDB(db, dbPath, config)
}

func MemoryStatusWithDB(db *sql.DB, dbPath string, config *QuorumConfig) (MemoryStatusResult, error) {
	result := MemoryStatusResult{DBPath: dbPath, ProjectID: config.ProjectID, ProjectName: config.ProjectName}
	var exists int
	if err := db.QueryRow(`SELECT COUNT(*) FROM projects WHERE id = ?`, config.ProjectID).Scan(&exists); err != nil {
		return result, fmt.Errorf("failed to inspect project registration: %w", err)
	}
	result.ProjectRegistered = exists > 0

	rows, err := db.Query(`SELECT type, COUNT(*) FROM memory_entries WHERE project_id = ? GROUP BY type`, config.ProjectID)
	if err != nil {
		return result, fmt.Errorf("failed to count memory entries: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var typ string
		var count int
		if err := rows.Scan(&typ, &count); err != nil {
			return result, fmt.Errorf("failed to scan memory count: %w", err)
		}
		switch typ {
		case "pattern":
			result.Counts.Pattern = count
		case "decision":
			result.Counts.Decision = count
		case "lesson":
			result.Counts.Lesson = count
		}
	}
	if err := rows.Err(); err != nil {
		return result, fmt.Errorf("failed to iterate memory counts: %w", err)
	}
	return result, nil
}

func parseAndValidateMemory(raw []byte) (map[string]any, []byte, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, nil, fmt.Errorf("invalid memory JSON: %w", err)
	}
	if err := ValidateArtifact(filepath.ToSlash(filepath.Join("memory", "entry.json")), payload); err != nil {
		return nil, nil, err
	}
	canonical, err := canonicalMemoryJSON(payload)
	if err != nil {
		return nil, nil, err
	}
	return payload, canonical, nil
}

func canonicalMemoryJSON(payload map[string]any) ([]byte, error) {
	keys := make([]string, 0, len(payload))
	for k := range payload {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	ordered := make(map[string]any, len(payload))
	for _, k := range keys {
		ordered[k] = payload[k]
	}
	return json.Marshal(ordered)
}

func nullableString(s string) any {
	if s == "" {
		return nil
	}
	return s
}
