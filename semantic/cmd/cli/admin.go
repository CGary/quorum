//go:build sqlite_fts5 && sqlite_vec

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/hsme/core/src/bootstrap"
	"github.com/hsme/core/src/core/admin"
)

func runAdmin(args []string, cfg bootstrap.Config) {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "error: admin action required (retry-failed, backup, restore)")
		os.Exit(exitUsage)
	}

	action := args[0]
	subArgs := args[1:]

	switch action {
	case "retry-failed":
		runAdminRetryFailed(subArgs, cfg)
	case "backup":
		runAdminBackup(subArgs, cfg)
	case "restore":
		runAdminRestore(subArgs, cfg)
	default:
		fmt.Fprintf(os.Stderr, "unknown admin action: %s\n", action)
		os.Exit(exitUsage)
	}
}

func runAdminRetryFailed(args []string, cfg bootstrap.Config) {
	fs := flag.NewFlagSet("admin retry-failed", flag.ExitOnError)
	RegisterDBFlags(fs, &cfg)
	fs.Parse(args)

	db, err := bootstrap.OpenDB(cfg)
	if err != nil {
		WriteError(os.Stderr, fmt.Errorf("failed to open database: %w", err), exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}
	defer db.Close()

	affected, err := admin.RetryFailedTasks(context.Background(), db)
	if err != nil {
		WriteError(os.Stderr, err, exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}

	res := map[string]interface{}{
		"status":        "ok",
		"retried_tasks": affected,
	}
	if outputFormat == "json" {
		WriteResult(os.Stdout, res, outputFormat)
	} else {
		WriteResult(os.Stdout, FormatAdminRetryResult(res), outputFormat)
	}
}

func runAdminBackup(args []string, cfg bootstrap.Config) {
	fs := flag.NewFlagSet("admin backup", flag.ExitOnError)
	var dest string
	fs.StringVar(&dest, "dest", "", "Destination path for backup")
	RegisterDBFlags(fs, &cfg)
	fs.Parse(args)

	if dest == "" {
	        dest = filepath.Join("backups", fmt.Sprintf("engram-%s.db", time.Now().UTC().Format("20060102T150405Z")))
	        if err := os.MkdirAll("backups", 0755); err != nil {
	                WriteError(os.Stderr, fmt.Errorf("failed to create backups directory: %w", err), exitRuntime, outputFormat)
	                os.Exit(exitRuntime)
	        }
	}
	err := admin.Backup(context.Background(), cfg.DBPath, dest)
	if err != nil {
		WriteError(os.Stderr, err, exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}

	res := map[string]interface{}{
		"status": "ok",
		"backup": dest,
	}
	if outputFormat == "json" {
		WriteResult(os.Stdout, res, outputFormat)
	} else {
		WriteResult(os.Stdout, FormatAdminBackupResult(res), outputFormat)
	}
}

func runAdminRestore(args []string, cfg bootstrap.Config) {
	fs := flag.NewFlagSet("admin restore", flag.ExitOnError)
	var from string
	var latest bool
	fs.StringVar(&from, "from", "", "Source path for restore")
	fs.BoolVar(&latest, "latest", false, "Restore from most recent backup")
	RegisterDBFlags(fs, &cfg)
	fs.Parse(args)

	// Validation: exactly one of --from and --latest must be set
	if (from == "" && !latest) || (from != "" && latest) {
		WriteError(os.Stderr, fmt.Errorf("exactly one of --from or --latest must be set"), exitUsage, outputFormat)
		os.Exit(exitUsage)
	}

	src := from
	if latest {
		var err error
		src, err = findLatestBackup()
		if err != nil {
			WriteError(os.Stderr, err, exitRuntime, outputFormat)
			os.Exit(exitRuntime)
		}
	}

	err := admin.Restore(context.Background(), cfg.DBPath, src)
	if err != nil {
		WriteError(os.Stderr, err, exitRuntime, outputFormat)
		os.Exit(exitRuntime)
	}

	res := map[string]interface{}{
		"status":  "ok",
		"restore": src,
	}
	if outputFormat == "json" {
		WriteResult(os.Stdout, res, outputFormat)
	} else {
		WriteResult(os.Stdout, FormatAdminRestoreResult(res), outputFormat)
	}
}

func findLatestBackup() (string, error) {
	matches, err := filepath.Glob("backups/engram-*.db")
	if err != nil {
		return "", fmt.Errorf("failed to scan for backups: %w", err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no backups in backups/")
	}

	// Sort by mtime descending
	sort.Slice(matches, func(i, j int) bool {
		fi, errI := os.Stat(matches[i])
		fj, errJ := os.Stat(matches[j])
		if errI != nil || errJ != nil {
			return false
		}
		return fi.ModTime().After(fj.ModTime())
	})

	return matches[0], nil
}
