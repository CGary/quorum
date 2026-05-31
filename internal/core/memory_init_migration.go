package core

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type InitMigrationOptions struct {
	RemoveFile func(string) error
}

type InitMigrationResult struct {
	FilesSeen     int
	FilesInserted int
	FilesDeleted  int
	Skipped       []string
	Warnings      []string
}

type legacyMemoryFile struct {
	Path    string
	Payload map[string]any
	Hash    string
	RawJSON string
}

var legacyMemoryDirs = []string{"decisions", "patterns", "lessons"}
var currentMemoryIDRegex = regexp.MustCompile(`^(PAT|DEC|LES)-[0-9]{4}-[0-9]{2}-[0-9]{2}-[0-9]+$`)

func RunInitMemoryMigration(db *sql.DB, projectRoot string, config *QuorumConfig) (*InitMigrationResult, error) {
	return RunInitMemoryMigrationWithOptions(db, projectRoot, config, InitMigrationOptions{})
}

func RunInitMemoryMigrationWithOptions(db *sql.DB, projectRoot string, config *QuorumConfig, opts InitMigrationOptions) (*InitMigrationResult, error) {
	if db == nil {
		return nil, fmt.Errorf("memory database is required")
	}
	if err := ValidateQuorumConfig(config); err != nil {
		return nil, err
	}
	removeFile := opts.RemoveFile
	if removeFile == nil {
		removeFile = os.Remove
	}
	result := &InitMigrationResult{}
	memoryRoot := filepath.Join(projectRoot, "memory")
	if _, err := os.Stat(memoryRoot); os.IsNotExist(err) {
		return result, nil
	} else if err != nil {
		return nil, err
	}

	files, err := collectLegacyMemoryFiles(memoryRoot)
	if err != nil {
		return result, err
	}
	result.FilesSeen = len(files)
	if len(files) == 0 {
		_ = removeEmptyLegacyDirs(memoryRoot)
		return result, nil
	}

	seenHashes := map[string]string{}
	for _, f := range files {
		if old, ok := seenHashes[f.Payload["id"].(string)]; ok && old != f.Hash {
			return result, fmt.Errorf("duplicate memory id %q has different content hashes", f.Payload["id"])
		}
		seenHashes[f.Payload["id"].(string)] = f.Hash
	}

	tx, err := db.Begin()
	if err != nil {
		return result, err
	}
	migrationID, err := beginInitMigration(tx, config.ProjectID, len(files))
	if err != nil {
		tx.Rollback()
		return result, err
	}
	insertedIDs := map[string]bool{}
	for _, f := range files {
		inserted, err := insertLegacyMemory(tx, config.ProjectID, f)
		if err != nil {
			tx.Rollback()
			return result, err
		}
		if inserted {
			result.FilesInserted++
			insertedIDs[memoryEntryID(f.Payload)] = true
		} else {
			result.Skipped = append(result.Skipped, f.Path)
		}
	}
	if err := completeInitMigration(tx, migrationID, "committed", result.FilesInserted, 0, ""); err != nil {
		tx.Rollback()
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}

	for _, f := range files {
		verified, err := verifyMigratedMemory(db, config.ProjectID, memoryEntryID(f.Payload), f.Hash)
		if err != nil {
			return result, err
		}
		if !verified {
			return result, fmt.Errorf("migrated memory %q could not be verified by project_id, id, and content_hash", memoryEntryID(f.Payload))
		}
	}

	for _, f := range files {
		if err := removeFile(f.Path); err != nil {
			_, _ = db.Exec("UPDATE init_migrations SET status = ?, files_deleted = ?, message = ?, completed_at = ? WHERE id = ?", "partial_delete", result.FilesDeleted, err.Error(), time.Now().UTC().Format(time.RFC3339), migrationID)
			return result, fmt.Errorf("migration inserted and verified entries, but failed deleting %s: %w", f.Path, err)
		}
		result.FilesDeleted++
	}
	_ = removeEmptyLegacyDirs(memoryRoot)
	_, _ = db.Exec("UPDATE init_migrations SET files_deleted = ?, completed_at = ? WHERE id = ?", result.FilesDeleted, time.Now().UTC().Format(time.RFC3339), migrationID)
	_ = insertedIDs
	return result, nil
}

func collectLegacyMemoryFiles(memoryRoot string) ([]legacyMemoryFile, error) {
	var out []legacyMemoryFile
	entries, err := os.ReadDir(memoryRoot)
	if err != nil {
		return nil, err
	}
	allowedDirs := map[string]bool{"decisions": true, "patterns": true, "lessons": true}
	for _, entry := range entries {
		name := entry.Name()
		if name == ".gitkeep" {
			continue
		}
		if entry.IsDir() {
			if !allowedDirs[name] {
				return nil, fmt.Errorf("unexpected subdirectory in memory/: %s", name)
			}
			continue
		}
		return nil, fmt.Errorf("unexpected file in memory/: %s", name)
	}
	for _, dirName := range legacyMemoryDirs {
		dir := filepath.Join(memoryRoot, dirName)
		entries, err := os.ReadDir(dir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			if entry.IsDir() {
				return nil, fmt.Errorf("unexpected subdirectory in %s: %s", filepath.ToSlash(filepath.Join("memory", dirName)), entry.Name())
			}
			if entry.Name() == ".gitkeep" {
				continue
			}
			if filepath.Ext(entry.Name()) != ".json" {
				return nil, fmt.Errorf("unexpected non-JSON file in %s: %s", filepath.ToSlash(filepath.Join("memory", dirName)), entry.Name())
			}
			file, err := loadLegacyMemoryFile(path)
			if err != nil {
				return nil, err
			}
			out = append(out, file)
		}
	}
	return out, nil
}

func loadLegacyMemoryFile(path string) (legacyMemoryFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return legacyMemoryFile{}, err
	}
	var payload map[string]any
	if err := json.Unmarshal(b, &payload); err != nil {
		return legacyMemoryFile{}, err
	}
	payload = normalizeLegacyMemoryPayload(path, payload)
	if err := ValidateArtifact(path, payload); err != nil {
		return legacyMemoryFile{}, err
	}
	hash, err := CanonicalMemoryHash(payload)
	if err != nil {
		return legacyMemoryFile{}, err
	}
	raw, err := canonicalMemoryJSON(payload)
	if err != nil {
		return legacyMemoryFile{}, err
	}
	return legacyMemoryFile{Path: path, Payload: payload, Hash: hash, RawJSON: string(raw)}, nil
}

func normalizeLegacyMemoryPayload(path string, payload map[string]any) map[string]any {
	normalized := make(map[string]any, len(payload)+3)
	for k, v := range payload {
		normalized[k] = v
	}

	if _, ok := normalized["source_task"]; !ok {
		if taskRef, ok := nonEmptyString(normalized["task_ref"]); ok {
			normalized["source_task"] = taskRef
			delete(normalized, "task_ref")
		}
	}
	if _, ok := normalized["content"]; !ok {
		if resolution, ok := nonEmptyString(normalized["resolution"]); ok {
			normalized["content"] = resolution
			delete(normalized, "resolution")
		}
	}
	if _, ok := normalized["created_at"]; !ok {
		normalized["created_at"] = legacyMemoryDate(path)
	}
	if !memoryIDLooksCurrent(normalized["id"]) {
		if typ, ok := nonEmptyString(normalized["type"]); ok {
			normalized["id"] = legacyMemoryID(path, typ, normalized["id"], normalized["created_at"])
		}
	}

	return normalized
}

func nonEmptyString(value any) (string, bool) {
	s, ok := value.(string)
	s = strings.TrimSpace(s)
	return s, ok && s != ""
}

func memoryIDLooksCurrent(value any) bool {
	id, ok := nonEmptyString(value)
	if !ok {
		return false
	}
	return currentMemoryIDRegex.MatchString(id)
}

func legacyMemoryDate(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return time.Now().UTC().Format(time.DateOnly)
	}
	return info.ModTime().UTC().Format(time.DateOnly)
}

func legacyMemoryID(path, memoryType string, oldID any, createdAt any) string {
	prefix := map[string]string{
		"pattern":  "PAT",
		"decision": "DEC",
		"lesson":   "LES",
	}[memoryType]
	if prefix == "" {
		prefix = "LES"
	}
	date, ok := nonEmptyString(createdAt)
	if !ok {
		date = legacyMemoryDate(path)
	}
	seed := fmt.Sprintf("%s|%s|%v", memoryType, date, oldID)
	if id, ok := nonEmptyString(oldID); !ok || id == "" {
		seed = seed + "|" + filepath.ToSlash(path)
	}
	n := crc32.ChecksumIEEE([]byte(seed)) % 100000000
	if n == 0 {
		n = 1
	}
	return fmt.Sprintf("%s-%s-%d", prefix, date, n)
}

func beginInitMigration(tx *sql.Tx, projectID string, filesSeen int) (int64, error) {
	res, err := tx.Exec("INSERT INTO init_migrations (project_id, started_at, status, files_seen) VALUES (?, ?, ?, ?)", projectID, time.Now().UTC().Format(time.RFC3339), "started", filesSeen)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func completeInitMigration(tx *sql.Tx, id int64, status string, inserted, deleted int, message string) error {
	_, err := tx.Exec("UPDATE init_migrations SET completed_at = ?, status = ?, files_inserted = ?, files_deleted = ?, message = ? WHERE id = ?", time.Now().UTC().Format(time.RFC3339), status, inserted, deleted, message, id)
	return err
}

func insertLegacyMemory(tx *sql.Tx, projectID string, file legacyMemoryFile) (bool, error) {
	memoryID := memoryEntryID(file.Payload)
	var existingHash string
	err := tx.QueryRow("SELECT content_hash FROM memory_entries WHERE project_id = ? AND id = ?", projectID, memoryID).Scan(&existingHash)
	if err == nil {
		if existingHash == file.Hash {
			return false, nil
		}
		return false, fmt.Errorf("memory id %q already exists with different content hash", memoryID)
	}
	if err != sql.ErrNoRows {
		return false, err
	}

	memoryType, _ := file.Payload["type"].(string)
	sourceTask, _ := file.Payload["source_task"].(string)
	title, _ := file.Payload["title"].(string)
	context, _ := file.Payload["context"].(string)
	content, _ := file.Payload["content"].(string)
	createdAt, _ := file.Payload["created_at"].(string)
	supersedes, _ := file.Payload["supersedes"].(string)

	_, err = tx.Exec(`INSERT INTO memory_entries 
(project_id, id, type, source_task, title, context, content, created_at, supersedes, content_hash, raw_json, source_path)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		projectID, memoryID, memoryType, sourceTask, title, context, content, createdAt, nullableString(supersedes), file.Hash, file.RawJSON, filepath.ToSlash(file.Path))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			return false, fmt.Errorf("memory id %q already exists outside this project or conflicts with current schema", memoryID)
		}
		return false, err
	}

	// Insert related memory references
	if relatedRaw, ok := file.Payload["related"]; ok {
		if relatedList, ok := relatedRaw.([]any); ok {
			for _, r := range relatedList {
				if rStr, ok := r.(string); ok {
					if _, err := tx.Exec(`INSERT OR IGNORE INTO memory_related (project_id, memory_id, related_ref) VALUES (?, ?, ?)`, projectID, memoryID, rStr); err != nil {
						return false, fmt.Errorf("failed to insert related reference: %w", err)
					}
				}
			}
		} else if relatedStrList, ok := relatedRaw.([]string); ok {
			for _, r := range relatedStrList {
				if _, err := tx.Exec(`INSERT OR IGNORE INTO memory_related (project_id, memory_id, related_ref) VALUES (?, ?, ?)`, projectID, memoryID, r); err != nil {
					return false, fmt.Errorf("failed to insert related reference: %w", err)
				}
			}
		}
	}

	// Insert anti-patterns
	if antiPatternsRaw, ok := file.Payload["anti_patterns"]; ok {
		if antiPatternsList, ok := antiPatternsRaw.([]any); ok {
			for i, ap := range antiPatternsList {
				if apStr, ok := ap.(string); ok {
					if _, err := tx.Exec(`INSERT OR IGNORE INTO memory_anti_patterns (project_id, memory_id, ordinal, content) VALUES (?, ?, ?, ?)`, projectID, memoryID, i, apStr); err != nil {
						return false, fmt.Errorf("failed to insert anti-pattern: %w", err)
					}
				}
			}
		} else if antiPatternsStrList, ok := antiPatternsRaw.([]string); ok {
			for i, ap := range antiPatternsStrList {
				if _, err := tx.Exec(`INSERT OR IGNORE INTO memory_anti_patterns (project_id, memory_id, ordinal, content) VALUES (?, ?, ?, ?)`, projectID, memoryID, i, ap); err != nil {
					return false, fmt.Errorf("failed to insert anti-pattern: %w", err)
				}
			}
		}
	}

	// Insert supersession edge
	if supersedes != "" {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO memory_supersession_edges (project_id, from_id, to_id) VALUES (?, ?, ?)`, projectID, memoryID, supersedes); err != nil {
			return false, fmt.Errorf("failed to insert supersession edge: %w", err)
		}
	}

	return true, nil
}

func verifyMigratedMemory(db *sql.DB, projectID, memoryID, hash string) (bool, error) {
	var got string
	err := db.QueryRow("SELECT content_hash FROM memory_entries WHERE project_id = ? AND id = ?", projectID, memoryID).Scan(&got)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return got == hash, nil
}

func removeEmptyLegacyDirs(memoryRoot string) error {
	for _, dirName := range legacyMemoryDirs {
		_ = os.Remove(filepath.Join(memoryRoot, dirName))
	}
	entries, err := os.ReadDir(memoryRoot)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.Name() != ".gitkeep" {
			return nil
		}
		_ = os.Remove(filepath.Join(memoryRoot, entry.Name()))
	}
	return os.Remove(memoryRoot)
}
