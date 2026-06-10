package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func runBackup(cfg *Config) (*PhaseResult, error) {
	start := time.Now()
	res := &PhaseResult{Name: "backup", Status: "ok", Metadata: make(map[string]string)}

	if cfg.Mode == ModeDryRun || cfg.SkipBackup {
		res.Status = "skipped"
		res.Metadata["reason"] = "dry-run or skip-backup set"
		res.Duration = time.Since(start)
		return res, nil
	}

	// Try to find the backup script
	backupScript := "/home/gary/dev/hsme/scripts/backup_hot.sh"
	// Check if we are in a worktree
	if cwd, err := os.Getwd(); err == nil {
		worktreeScript := filepath.Join(cwd, "scripts/backup_hot.sh")
		if _, err := os.Stat(worktreeScript); err == nil {
			backupScript = worktreeScript
		}
	}
	if _, err := os.Stat(backupScript); os.IsNotExist(err) {
		return res, fmt.Errorf("backup script not found at %s", backupScript)
	}

	// The script usually takes no arguments and uses environment variables
	// or defaults to backing up the production DB.
	// However, we want to backup cfg.HSMEDBPath.
	cmd := exec.Command(backupScript)
	cmd.Env = append(os.Environ(), fmt.Sprintf("SQLITE_DB_PATH=%s", cfg.HSMEDBPath))
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return res, fmt.Errorf("backup failed: %w\nOutput: %s", err, string(output))
	}

	// Typically the script prints the backup path.
	// We'll just record that it succeeded for now.
	res.Metadata["message"] = "hot backup completed successfully"
	res.Duration = time.Since(start)
	
	return res, nil
}
