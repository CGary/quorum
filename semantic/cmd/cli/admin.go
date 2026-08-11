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
	var agentFlags AgentFlags
	fs := flag.NewFlagSet("admin", flag.ContinueOnError)
	RegisterAgentFlags(fs, &agentFlags)
	RegisterDBFlags(fs, &cfg)
	_ = fs.Parse(args)
	ScanTrailingFlags(fs)

	if fs.NArg() < 1 {
		emitErrorAndExit("admin", errMissingRequired, "admin action required (retry-failed, backup, restore)", "action", "", true, "hsme-cli admin <action>", agentFlags.JSON)
	}

	action := fs.Arg(0)
	subArgs := fs.Args()[1:]

	switch action {
	case "retry-failed":
		runAdminRetryFailed(subArgs, cfg)
	case "backup":
		runAdminBackup(subArgs, cfg)
	case "restore":
		runAdminRestore(subArgs, cfg)
	default:
		emitErrorAndExit("admin", errInvalidEnum, fmt.Sprintf("unknown admin action: %s", action), "action", action, false, "hsme-cli admin retry-failed|backup|restore", agentFlags.JSON)
	}
}

func runAdminRetryFailed(args []string, cfg bootstrap.Config) {
	fs := flag.NewFlagSet("admin retry-failed", flag.ExitOnError)
	var agentFlags AgentFlags

	RegisterAgentFlags(fs, &agentFlags)
	RegisterDBFlags(fs, &cfg)

	fs.Parse(args)
	ScanTrailingFlags(fs)

	// 1. --schema check first
	if agentFlags.Schema {
		WriteSchemaEnvelope(os.Stdout, adminRetryFailedSchema())
		os.Exit(0)
	}

	// 2. Open DB
	db, err := bootstrap.OpenDB(cfg)
	if err != nil {
		emitErrorAndExit("admin.retry-failed", errInternal, fmt.Sprintf("failed to open database: %v", err), "", "", false, "", agentFlags.JSON)
	}
	defer db.Close()

	// 3. Retry failed tasks
	affected, err := admin.RetryFailedTasks(context.Background(), db)
	if err != nil {
		emitErrorAndExit("admin.retry-failed", errInternal, err.Error(), "", "", false, "", agentFlags.JSON)
	}

	res := map[string]interface{}{
		"status":        "ok",
		"retried_tasks": affected,
	}

	if agentFlags.Output != "" {
		if err := writeOutputFile(agentFlags.Output, res); err != nil {
			emitErrorAndExit("admin.retry-failed", errInternal, fmt.Sprintf("cannot write --output %s: %v", agentFlags.Output, err), "output", agentFlags.Output, false, "", agentFlags.JSON)
		}
		if agentFlags.JSON {
			WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
				OK:          true,
				Command:     "admin.retry-failed",
				Summary:     "result written to " + agentFlags.Output,
				Data:        map[string]any{"result_file": agentFlags.Output},
				NextActions: []NextAction{},
			})
		} else {
			fmt.Printf("result written to %s\n", agentFlags.Output)
		}
		return
	}

	if agentFlags.JSON {
		WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
			OK:          true,
			Command:     "admin.retry-failed",
			Summary:     fmt.Sprintf("Retry complete. Retried tasks: %d", affected),
			Data:        res,
			NextActions: []NextAction{},
		})
	} else {
		fmt.Println(FormatAdminRetryResult(res))
	}
}

func runAdminBackup(args []string, cfg bootstrap.Config) {
	fs := flag.NewFlagSet("admin backup", flag.ExitOnError)
	var dest string
	var agentFlags AgentFlags

	fs.StringVar(&dest, "dest", "", "Destination path for backup")
	RegisterAgentFlags(fs, &agentFlags)
	RegisterDBFlags(fs, &cfg)

	fs.Parse(args)
	ScanTrailingFlags(fs)

	// 1. --schema check first
	if agentFlags.Schema {
		WriteSchemaEnvelope(os.Stdout, adminBackupSchema())
		os.Exit(0)
	}

	if dest == "" {
		dest = filepath.Join("backups", fmt.Sprintf("engram-%s.db", time.Now().UTC().Format("20060102T150405Z")))
		if err := os.MkdirAll("backups", 0755); err != nil {
			code := errInternal
			if os.IsPermission(err) {
				code = errPermissionDenied
			}
			emitErrorAndExit("admin.backup", code, fmt.Sprintf("failed to create backups directory: %v", err), "dest", dest, false, "", agentFlags.JSON)
		}
	}

	err := admin.Backup(context.Background(), cfg.DBPath, dest)
	if err != nil {
		emitErrorAndExit("admin.backup", errInternal, err.Error(), "", "", false, "", agentFlags.JSON)
	}

	res := map[string]interface{}{
		"status": "ok",
		"backup": dest,
	}

	if agentFlags.Output != "" {
		if err := writeOutputFile(agentFlags.Output, res); err != nil {
			emitErrorAndExit("admin.backup", errInternal, fmt.Sprintf("cannot write --output %s: %v", agentFlags.Output, err), "output", agentFlags.Output, false, "", agentFlags.JSON)
		}
		if agentFlags.JSON {
			WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
				OK:          true,
				Command:     "admin.backup",
				Summary:     "result written to " + agentFlags.Output,
				Data:        map[string]any{"result_file": agentFlags.Output},
				NextActions: []NextAction{},
			})
		} else {
			fmt.Printf("result written to %s\n", agentFlags.Output)
		}
		return
	}

	if agentFlags.JSON {
		WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
			OK:          true,
			Command:     "admin.backup",
			Summary:     fmt.Sprintf("Backup created successfully: %s", dest),
			Data:        res,
			NextActions: []NextAction{},
		})
	} else {
		fmt.Println(FormatAdminBackupResult(res))
	}
}

func runAdminRestore(args []string, cfg bootstrap.Config) {
	fs := flag.NewFlagSet("admin restore", flag.ExitOnError)
	var from string
	var latest bool
	var dryRun bool
	var agentFlags AgentFlags

	fs.StringVar(&from, "from", "", "Source path for restore")
	fs.BoolVar(&latest, "latest", false, "Restore from most recent backup")
	fs.BoolVar(&dryRun, "dry-run", false, "simulate restore without modifying database")

	RegisterAgentFlags(fs, &agentFlags)
	RegisterDBFlags(fs, &cfg)

	fs.Parse(args)
	ScanTrailingFlags(fs)

	// 1. --schema check first
	if agentFlags.Schema {
		WriteSchemaEnvelope(os.Stdout, adminRestoreSchema())
		os.Exit(0)
	}

	// 2. Validation: exactly one of --from and --latest must be set
	if (from == "" && !latest) || (from != "" && latest) {
		emitErrorAndExit("admin.restore", errValidationFailed, "exactly one of --from or --latest must be set", "from/latest", "", true, "hsme-cli admin restore --from <path> OR --latest", agentFlags.JSON)
	}

	src := from
	if latest {
		var err error
		src, err = findLatestBackup()
		if err != nil {
			emitErrorAndExit("admin.restore", errFileNotFound, err.Error(), "latest", "", false, "", agentFlags.JSON)
		}
	} else {
		if _, err := os.Stat(src); err != nil {
			emitErrorAndExit("admin.restore", errFileNotFound, fmt.Sprintf("backup file does not exist: %s", src), "from", src, false, "", agentFlags.JSON)
		}
	}

	// 3. --dry-run check
	if dryRun {
		data := map[string]any{
			"dry_run":      true,
			"restore_from": src,
		}
		if agentFlags.JSON {
			WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
				OK:          true,
				Command:     "admin.restore",
				Summary:     fmt.Sprintf("dry-run: would restore from %s", src),
				Data:        data,
				NextActions: []NextAction{},
			})
		} else {
			fmt.Printf("dry-run: would restore from %s\n", src)
		}
		os.Exit(0)
	}

	err := admin.Restore(context.Background(), cfg.DBPath, src)
	if err != nil {
		emitErrorAndExit("admin.restore", errInternal, err.Error(), "", "", false, "", agentFlags.JSON)
	}

	res := map[string]interface{}{
		"status":  "ok",
		"restore": src,
	}

	if agentFlags.Output != "" {
		if err := writeOutputFile(agentFlags.Output, res); err != nil {
			emitErrorAndExit("admin.restore", errInternal, fmt.Sprintf("cannot write --output %s: %v", agentFlags.Output, err), "output", agentFlags.Output, false, "", agentFlags.JSON)
		}
		if agentFlags.JSON {
			WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
				OK:          true,
				Command:     "admin.restore",
				Summary:     "result written to " + agentFlags.Output,
				Data:        map[string]any{"result_file": agentFlags.Output},
				NextActions: []NextAction{},
			})
		} else {
			fmt.Printf("result written to %s\n", agentFlags.Output)
		}
		return
	}

	if agentFlags.JSON {
		WriteSuccessEnvelope(os.Stdout, SuccessEnvelope{
			OK:          true,
			Command:     "admin.restore",
			Summary:     fmt.Sprintf("Database restored successfully from: %s", src),
			Data:        res,
			NextActions: []NextAction{},
		})
	} else {
		fmt.Println(FormatAdminRestoreResult(res))
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

func adminRetryFailedSchema() map[string]any {
	return map[string]any{
		"command":     "admin.retry-failed",
		"description": "Re-queue all tasks in 'failed' state.",
		"input": map[string]any{
			"properties": map[string]any{
				"output": map[string]any{"type": "string", "description": "write result to this file"},
			},
		},
		"output": map[string]any{"type": "object", "required": []string{"ok", "command", "summary", "data"}},
		"errors": []string{errInternal},
	}
}

func adminBackupSchema() map[string]any {
	return map[string]any{
		"command":     "admin.backup",
		"description": "Create a backup of the current database.",
		"input": map[string]any{
			"properties": map[string]any{
				"dest":   map[string]any{"type": "string", "description": "destination path for backup"},
				"output": map[string]any{"type": "string", "description": "write result to this file"},
			},
		},
		"output": map[string]any{"type": "object", "required": []string{"ok", "command", "summary", "data"}},
		"errors": []string{errPermissionDenied, errInternal},
	}
}

func adminRestoreSchema() map[string]any {
	return map[string]any{
		"command":     "admin.restore",
		"description": "Restore the database from a backup.",
		"input": map[string]any{
			"properties": map[string]any{
				"from":    map[string]any{"type": "string", "description": "source path for restore"},
				"latest":  map[string]any{"type": "boolean", "description": "restore from most recent backup"},
				"dry-run": map[string]any{"type": "boolean", "description": "simulate restore without modifying database"},
				"output":  map[string]any{"type": "string", "description": "write result to this file"},
			},
		},
		"output": map[string]any{"type": "object", "required": []string{"ok", "command", "summary", "data"}},
		"errors": []string{errValidationFailed, errFileNotFound, errInternal},
	}
}
