package main

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"time"
)

func runSnapshotLegacy(legacyDB *sql.DB) (*PhaseResult, error) {
	start := time.Now()
	res := &PhaseResult{Name: "snapshot_legacy", Status: "ok", Metadata: make(map[string]string)}

	var rowcount int
	var maxCreatedAt string
	err := legacyDB.QueryRow("SELECT count(*), max(created_at) FROM observations WHERE deleted_at IS NULL").Scan(&rowcount, &maxCreatedAt)
	if err != nil {
		return res, fmt.Errorf("failed to snapshot legacy: %w", err)
	}

	res.Metadata["rowcount"] = fmt.Sprint(rowcount)
	res.Metadata["max_created_at"] = maxCreatedAt
	res.Duration = time.Since(start)
	return res, nil
}

func findLatestBaseline(cfg *Config) (string, error) {
	// Look for the latest report.json in cfg.MigrationsDir
	files, err := ioutil.ReadDir(cfg.MigrationsDir)
	if err != nil {
		return "", err
	}

	var reports []string
	for _, f := range files {
		if f.IsDir() {
			reportPath := filepath.Join(cfg.MigrationsDir, f.Name(), "report.json")
			if _, err := os.Stat(reportPath); err == nil {
				reports = append(reports, reportPath)
			}
		}
	}

	if len(reports) == 0 {
		return "", fmt.Errorf("no prior baseline report found")
	}

	sort.Strings(reports)
	return reports[len(reports)-1], nil
}
