package sqlite

import (
	"database/sql"
	"fmt"
	"strconv"
)

// EmbedderInfo describe los metadatos del embedder activo que hay que
// persistir y comparar contra system_config al arrancar.
type EmbedderInfo interface {
	ModelID() string
	Dimension() int
}

// ValidateEmbeddingConfig implementa §4.2 y §11.2 del Technical Specification:
// en el primer arranque escribe la baseline en system_config; en arranques
// posteriores compara y rechaza el startup si el modelo o la dimensión
// cambiaron. Sin esta validación, un cambio en EMBEDDING_MODEL silenciosamente
// rompe la tabla vec0 (dim fija) y cada chunk falla en el worker.
func ValidateEmbeddingConfig(db *sql.DB, e EmbedderInfo) error {
	want := map[string]string{
		"schema_version":  "1",
		"embedding_model": e.ModelID(),
		"embedding_dim":   strconv.Itoa(e.Dimension()),
	}

	rows, err := db.Query("SELECT key, value FROM system_config WHERE key IN ('schema_version','embedding_model','embedding_dim')")
	if err != nil {
		return fmt.Errorf("read system_config: %w", err)
	}
	got := make(map[string]string)
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			rows.Close()
			return err
		}
		got[k] = v
	}
	rows.Close()

	if len(got) == 0 {
		// Primer arranque: sembrar la baseline.
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		for k, v := range want {
			if _, err := tx.Exec("INSERT INTO system_config(key, value) VALUES(?, ?)", k, v); err != nil {
				return fmt.Errorf("seed system_config: %w", err)
			}
		}
		return tx.Commit()
	}

	// Arranques siguientes: comparar.
	for k, w := range want {
		if persisted, ok := got[k]; ok && persisted != w {
			return fmt.Errorf(
				"EMBEDDING_DIM_MISMATCH: system_config.%s = %q pero embedder activo = %q — reindex o crear nueva DB (ver Technical_Specification §11.3)",
				k, persisted, w,
			)
		}
	}
	return nil
}
