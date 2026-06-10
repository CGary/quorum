package main

import (
	"database/sql"
	"fmt"
	"os"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func runPreflight(cfg *Config) (*PhaseResult, error) {
	start := time.Now()
	res := &PhaseResult{Name: "preflight", Status: "ok", Metadata: make(map[string]string)}

	// 1. Check migrations dir
	if err := os.MkdirAll(cfg.MigrationsDir, 0755); err != nil {
		return res, fmt.Errorf("failed to create migrations directory: %w", err)
	}

	// 2. HSME DB reachability
	db, err := sql.Open("sqlite3", cfg.HSMEDBPath)
	if err != nil {
		return res, fmt.Errorf("failed to open HSME DB: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return res, fmt.Errorf("HSME DB unreachable: %w", err)
	}

	// 3. Legacy DB reachability (read-only)
	legacyDSN := fmt.Sprintf("file:%s?mode=ro&immutable=1", cfg.LegacyDBPath)
	legacyDB, err := sql.Open("sqlite3", legacyDSN)
	if err != nil {
		return res, fmt.Errorf("failed to open Legacy DB: %w", err)
	}
	defer legacyDB.Close()

	if err := legacyDB.Ping(); err != nil {
		return res, fmt.Errorf("Legacy DB unreachable: %w", err)
	}

	res.Duration = time.Since(start)
	return res, nil
}
