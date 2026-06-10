package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/hsme/core/src/storage/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

type Mode string

const (
	ModeFull   Mode = "full"
	ModeDelta  Mode = "delta"
	ModeDryRun Mode = "dry-run"
)

type Config struct {
	Mode               Mode
	HSMEDBPath         string
	LegacyDBPath       string
	MigrationsDir      string
	UnmatchedThreshold float64
	SkipBackup         bool
	OllamaHost         string
	EmbeddingModel     string
	Quiet              bool
}

func main() {
	cfg := parseFlags()

	if err := validateConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Usage: %v\n", err)
		os.Exit(1)
	}

	report := &RunReport{
		RunID:        time.Now().Format("20060102T150405Z"),
		Mode:         cfg.Mode,
		Timestamp:    time.Now(),
		Status:       "running",
		HSMEDBPath:   cfg.HSMEDBPath,
		LegacyDBPath: cfg.LegacyDBPath,
	}

	if cfg.Mode == ModeDryRun {
		report.RunID += "-dryrun"
	}

	runDir := filepath.Join(cfg.MigrationsDir, report.RunID)
	
	err := runMigration(cfg, report)
	
	// Save report regardless of success
	if saveErr := report.Save(runDir); saveErr != nil {
		fmt.Fprintf(os.Stderr, "[ERROR] Failed to save report: %v\n", saveErr)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "[FATAL] Migration failed: %v\n", err)
		// Map error to exit code if possible
		os.Exit(4)
	}

	fmt.Printf("[done] report=%s\n", runDir)
}

func runMigration(cfg *Config, report *RunReport) error {
	// 0. Preflight
	preflightRes, err := runPreflight(cfg)
	report.Phases = append(report.Phases, *preflightRes)
	if err != nil {
		report.Status = "failed"
		return fmt.Errorf("preflight failed: %w", err)
	}
	fmt.Println("[preflight] ok")

	// 1. Backup
	backupRes, err := runBackup(cfg)
	report.Phases = append(report.Phases, *backupRes)
	if err != nil {
		report.Status = "failed"
		return fmt.Errorf("backup failed: %w", err)
	}
	fmt.Printf("[backup] %s\n", backupRes.Status)

	// Open Databases
	hsmeDB, err := sql.Open("sqlite3", cfg.HSMEDBPath)
	if err != nil {
		return fmt.Errorf("failed to open HSME DB: %w", err)
	}
	defer hsmeDB.Close()

	legacyDSN := fmt.Sprintf("file:%s?mode=ro&immutable=1", cfg.LegacyDBPath)
	legacyDB, err := sql.Open("sqlite3", legacyDSN)
	if err != nil {
		return fmt.Errorf("failed to open Legacy DB: %w", err)
	}
	defer legacyDB.Close()

	// 2. Schema
	schemaStart := time.Now()
	// We call our updated InitDB to ensure schema is correct
	// Note: in a real CLI we might want to just check or apply migrations.
	// Since InitDB is idempotent, it's safe.
	_, err = sqlite.InitDB(cfg.HSMEDBPath)
	schemaRes := PhaseResult{Name: "schema", Status: "ok", Duration: time.Since(schemaStart), Metadata: map[string]string{"applied": "true"}}
	if err != nil {
		schemaRes.Status = "failed"
		schemaRes.Error = err.Error()
		report.Phases = append(report.Phases, schemaRes)
		return fmt.Errorf("schema phase failed: %w", err)
	}
	report.Phases = append(report.Phases, schemaRes)
	fmt.Println("[schema] ok")

	// 3. Backfill Matched
	if cfg.Mode == ModeFull || cfg.Mode == ModeDryRun {
		restoreRes, err := runRestoreMatched(cfg, hsmeDB, legacyDB, report)
		report.Phases = append(report.Phases, *restoreRes)
		if err != nil {
			return fmt.Errorf("backfill_matched phase failed: %w", err)
		}
		fmt.Printf("[backfill_matched] ok matched=%s unmatched=%s\n", restoreRes.Metadata["matched"], restoreRes.Metadata["unmatched"])

		// 4. Retag Born in HSME
		retagRes, err := runRetagBornInHSME(cfg, hsmeDB, report)
		report.Phases = append(report.Phases, *retagRes)
		if err != nil {
			return fmt.Errorf("retag_born_in_hsme phase failed: %w", err)
		}
		fmt.Printf("[retag_born_in_hsme] ok retagged=%s\n", retagRes.Metadata["retagged"])

		// 5. Delete Garbage
		cleanupRes, err := runDeleteGarbage(cfg, hsmeDB, report)
		report.Phases = append(report.Phases, *cleanupRes)
		if err != nil {
			return fmt.Errorf("delete_garbage phase failed: %w", err)
		}
		fmt.Printf("[delete_garbage] ok deleted=%s\n", cleanupRes.Metadata["deleted"])
	}

	// 6. Snapshot Legacy
	// In Delta mode, we might need a prior baseline
	if cfg.Mode == ModeDelta {
		baselinePath, err := findLatestBaseline(cfg)
		if err == nil {
			fmt.Printf("Using baseline: %s\n", baselinePath)
			// Read baseline JSON to get max_created_at
			data, err := os.ReadFile(baselinePath)
			if err == nil {
				var baseline RunReport
				if err := json.Unmarshal(data, &baseline); err == nil {
					report.MaxCreatedAt = baseline.MaxCreatedAt
				}
			}
		}
	}

	snapshotRes, err := runSnapshotLegacy(legacyDB)
	report.Phases = append(report.Phases, *snapshotRes)
	if err != nil {
		return fmt.Errorf("snapshot_legacy phase failed: %w", err)
	}
	report.MaxCreatedAt = snapshotRes.Metadata["max_created_at"]
	fmt.Printf("[snapshot_legacy] ok rowcount=%s max_created_at='%s'\n", snapshotRes.Metadata["rowcount"], report.MaxCreatedAt)

	// 7. Ingest Orphans
	ingestRes, err := runIngestOrphans(cfg, hsmeDB, legacyDB, report)
	report.Phases = append(report.Phases, *ingestRes)
	if err != nil {
		return fmt.Errorf("ingest_orphans phase failed: %w", err)
	}
	fmt.Printf("[ingest_orphans] ok ingested=%s errored=%s\n", ingestRes.Metadata["ingested"], ingestRes.Metadata["errored"])


	report.Status = "completed"
	return nil
}



func parseFlags() *Config {
	cfg := &Config{}

	var modeStr string
	flag.StringVar(&modeStr, "mode", "full", "Execution mode (full, delta, dry-run)")
	flag.StringVar(&cfg.HSMEDBPath, "hsme-db", getEnv("HSME_DB_PATH", "/home/gary/dev/hsme/data/engram.db"), "HSME SQLite database (read-write)")
	flag.StringVar(&cfg.LegacyDBPath, "legacy-db", getEnv("LEGACY_DB_PATH", "/home/gary/.engram/engram.db"), "Legacy Engram SQLite database (read-only)")
	flag.StringVar(&cfg.MigrationsDir, "migrations-dir", "/home/gary/dev/hsme/data/migrations", "Where run reports are written")
	flag.Float64Var(&cfg.UnmatchedThreshold, "unmatched-threshold", 0.10, "Refuse phase 4 if unmatched ratio > threshold")
	flag.BoolVar(&cfg.SkipBackup, "skip-backup", false, "DANGEROUS — skip backup phase")
	flag.StringVar(&cfg.OllamaHost, "ollama-host", getEnv("OLLAMA_HOST", "http://localhost:11434"), "Ollama endpoint")
	flag.StringVar(&cfg.EmbeddingModel, "embedding-model", getEnv("EMBEDDING_MODEL", "nomic-embed-text"), "Embedding model name")
	flag.BoolVar(&cfg.Quiet, "quiet", false, "Suppress per-row progress output")

	flag.Parse()

	cfg.Mode = Mode(strings.ToLower(modeStr))
	return cfg
}

func validateConfig(cfg *Config) error {
	switch cfg.Mode {
	case ModeFull, ModeDelta, ModeDryRun:
		// valid
	default:
		return fmt.Errorf("invalid mode: %s", cfg.Mode)
	}
	return nil
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
