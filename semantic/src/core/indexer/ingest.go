package indexer

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/hsme/core/src/core/models"
)

// StoreContext ingests a new memory document.
func StoreContext(db *sql.DB, content string, sourceType string, project string, supersedesID *int64, forceReingest bool) (int64, error) {
	hash := ComputeHash(content)

	// Arrancamos la tx ANTES del dedup check. Con _txlock=immediate (ver db.go)
	// el BEGIN toma el write-lock; si otro caller está insertando el mismo hash
	// concurrentemente, nosotros bloqueamos hasta que commitee y después nuestro
	// SELECT ve su fila. Esto cierra la race entre SELECT y INSERT que existía
	// cuando el dedup vivía fuera de la tx.
	tx, err := db.Begin()
	if err != nil {
		return 0, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Dedup check — solo contra memorias ACTIVAS (el unique index es parcial
	// con WHERE status='active', así que filas superseded pueden compartir hash
	// y un SELECT sin filtro devolvería un ID superseded).
	var existingID int64
	err = tx.QueryRow("SELECT id FROM memories WHERE content_hash = ? AND status = 'active'", hash).Scan(&existingID)
	if err == nil {
		if !forceReingest {
			if cerr := tx.Commit(); cerr != nil {
				return 0, fmt.Errorf("failed to commit dedup tx: %w", cerr)
			}
			return existingID, nil
		}
		// forceReingest is true. Spec says:
		// "a new memory is created with the same content hash only if the caller
		// passes supersedes_memory_id pointing to the existing entry.
		// Otherwise the call is rejected with DUPLICATE_CONTENT."
		if supersedesID == nil || *supersedesID != existingID {
			return 0, fmt.Errorf("DUPLICATE_CONTENT: hash exists and supersedes_memory_id does not match")
		}
	} else if err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to check for existing content: %w", err)
	}

	// 4. Handle supersedence BEFORE insert to avoid UNIQUE constraint on active hash
	if supersedesID != nil {
		_, err = tx.Exec("UPDATE memories SET status = 'superseded', updated_at = ? WHERE id = ?", time.Now(), *supersedesID)
		if err != nil {
			return 0, fmt.Errorf("failed to update superseded memory: %w", err)
		}
	}

	// 5. Insert into memories
	res, err := tx.Exec(`
	        INSERT INTO memories (raw_content, content_hash, source_type, project, status, created_at, updated_at)
	        VALUES (?, ?, ?, ?, ?, ?, ?)
	`, content, hash, sourceType, project, "active", time.Now(), time.Now())
	if err != nil {
	        return 0, fmt.Errorf("failed to insert memory: %w", err)
	}
	memoryID, err := res.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}

	// 6. Link back if superseding
	if supersedesID != nil {
		_, err = tx.Exec("UPDATE memories SET superseded_by = ? WHERE id = ?", memoryID, *supersedesID)
		if err != nil {
			return 0, fmt.Errorf("failed to link superseded memory: %w", err)
		}
	}

	// 7. Split into chunks and insert
	chunks := Split(content, sourceType)
	for i, chunkText := range chunks {
		chunkRes, err := tx.Exec(`
			INSERT INTO memory_chunks (memory_id, chunk_index, chunk_text, token_estimate)
			VALUES (?, ?, ?, ?)
		`, memoryID, i, chunkText, estimateTokens(chunkText))
		if err != nil {
			return 0, fmt.Errorf("failed to insert chunk %d: %w", i, err)
		}

		if _, err := chunkRes.LastInsertId(); err != nil {
			return 0, fmt.Errorf("failed to get chunk last insert id: %w", err)
		}
		// FTS5 ya se sincroniza vía el trigger memory_chunks_ai en el schema.
	}

	// 8. Enqueue async tasks (T007)
	_, err = tx.Exec(`
		INSERT INTO async_tasks (memory_id, task_type, status)
		VALUES (?, ?, ?)
	`, memoryID, "embed", "pending")
	if err != nil {
		return 0, fmt.Errorf("failed to enqueue embed task: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO async_tasks (memory_id, task_type, status)
		VALUES (?, ?, ?)
	`, memoryID, "graph_extract", "pending")
	if err != nil {
		return 0, fmt.Errorf("failed to enqueue graph_extract task: %w", err)
	}

	// 9. Commit
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	return memoryID, nil
}
